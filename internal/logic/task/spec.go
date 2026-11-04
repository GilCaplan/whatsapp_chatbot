package task

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/core/model"
)

// Limits of a spec's text.
const (
	MaxTitle     = 80
	MaxAbout     = 200
	MaxPlace     = 120
	MaxCoverLine = 200
	MaxCover     = 10
	MaxSlots     = 6
)

// FindTime is the Find a time template's spec.
func FindTime() model.TaskSpec {
	return model.TaskSpec{
		Template: model.TaskFindTime, Title: "Find a time", Voice: model.VoiceAsYou,
		Meeting: model.TaskMeeting{Minutes: 60, Days: []string{"mon", "tue", "wed", "thu", "fri"}, From: "09:00", To: "18:00",
			HorizonDays: 7, Calendars: true},
		Slots: []model.TaskSlot{{Key: "when", Label: "Time", Kind: model.SlotWhen, Required: true}},
		Rules: []model.TaskRule{},
	}
}

// Templates are the ready-made tasks.
func Templates() []model.TaskTemplate {
	return []model.TaskTemplate{
		{ID: model.TaskFindTime, Title: "Find a time",
			Blurb: "Agree on a meeting time with someone. It only shows the times it offers, never your calendar, and asks you before confirming.",
			Spec:  FindTime()},
	}
}

// Template returns a template's spec.
func Template(id string) (model.TaskSpec, bool) {
	for _, t := range Templates() {
		if t.ID == id {
			return t.Spec, true
		}
	}
	return model.TaskSpec{}, false
}

// Rules are the spec's rules, or its template's.
func Rules(spec model.TaskSpec) []model.TaskRule {
	if len(spec.Rules) > 0 {
		return spec.Rules
	}
	return FindTimeRules()
}

func validClock(s string) bool {
	_, err := time.Parse("15:04", s)
	return err == nil
}

// Normalize trims and checks a spec (unsaved or saved).
func Normalize(s *model.TaskSpec) error {
	if s.Template == "" {
		s.Template = model.TaskFindTime
	}
	if _, ok := Template(s.Template); !ok {
		return fmt.Errorf("unknown template %q", s.Template)
	}
	s.Title, s.About, s.Place = strings.TrimSpace(s.Title), strings.TrimSpace(s.About), strings.TrimSpace(s.Place)
	if s.Title == "" {
		s.Title = "Find a time"
	}
	switch {
	case utf8.RuneCountInString(s.Title) > MaxTitle:
		return fmt.Errorf("a title has at most %d characters", MaxTitle)
	case utf8.RuneCountInString(s.About) > MaxAbout:
		return fmt.Errorf("what it is about has at most %d characters", MaxAbout)
	case utf8.RuneCountInString(s.Place) > MaxPlace:
		return fmt.Errorf("a place has at most %d characters", MaxPlace)
	}
	if s.Voice == "" {
		s.Voice = model.VoiceAsYou
	}
	if s.Voice != model.VoiceAsYou && s.Voice != model.VoiceAssistant {
		return errors.New("voice must be as-you or assistant")
	}
	m := &s.Meeting
	if m.Minutes == 0 {
		m.Minutes = 60
	}
	if m.Minutes < 15 || m.Minutes > 480 {
		return errors.New("a meeting is 15 to 480 minutes")
	}
	if m.From == "" {
		m.From = "09:00"
	}
	if m.To == "" {
		m.To = "18:00"
	}
	if !validClock(m.From) || !validClock(m.To) || m.To <= m.From {
		return errors.New("meeting hours must be HH:MM, from before to")
	}
	if m.HorizonDays == 0 {
		m.HorizonDays = 7
	}
	if m.HorizonDays < 1 || m.HorizonDays > 30 {
		return errors.New("look 1 to 30 days ahead")
	}
	var days []string
	for _, d := range m.Days {
		d = strings.ToLower(strings.TrimSpace(d))
		if !slices.Contains([]string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}, d) {
			return fmt.Errorf("unknown day %q", d)
		}
		if !slices.Contains(days, d) {
			days = append(days, d)
		}
	}
	m.Days = days
	if m.Days == nil {
		m.Days = []string{}
	}
	if m.Prefer != "" && !slices.Contains([]string{"morning", "afternoon", "evening"}, m.Prefer) {
		return errors.New("prefer is morning, afternoon or evening")
	}
	if err := normalizeSlots(s); err != nil {
		return err
	}
	if s.Rules == nil {
		s.Rules = []model.TaskRule{}
	}
	if err := ValidateRules(*s, s.Rules); err != nil {
		return err
	}
	if s.Privacy != nil {
		if err := checkPrivacy(s.Privacy); err != nil {
			return err
		}
	}
	if l := s.Limits; l != nil && (l.MaxMessages < 4 || l.MaxMessages > 100 || l.MaxDays < 1 || l.MaxDays > 30 ||
		l.MaxNudges < 0 || l.MaxNudges > 5 || l.NudgeAfterHours < 1 || l.NudgeAfterHours > 168) {
		return errors.New("limits: 4–100 messages, 1–30 days, 0–5 nudges, nudge after 1–168 hours")
	}
	if c := s.Commit; c != nil {
		if c.Mode != model.CommitApprove && c.Mode != model.CommitAutoFree {
			return errors.New("confirming must be approve or auto-if-free")
		}
		if !slices.Contains([]string{model.BookOff, model.BookICS, model.BookMACalendar}, c.Book) {
			return errors.New("booking must be off, ics or macalendar")
		}
	}
	return nil
}

func normalizeSlots(s *model.TaskSpec) error {
	if len(s.Slots) == 0 {
		s.Slots = FindTime().Slots
	}
	if len(s.Slots) > MaxSlots {
		return fmt.Errorf("at most %d things to agree on", MaxSlots)
	}
	keys := map[string]bool{}
	when := 0
	for i := range s.Slots {
		sl := &s.Slots[i]
		sl.Key, sl.Label = strings.ToLower(strings.TrimSpace(sl.Key)), strings.TrimSpace(sl.Label)
		if sl.Key == "" || keys[sl.Key] || strings.ContainsAny(sl.Key, " :") || utf8.RuneCountInString(sl.Key) > 30 {
			return fmt.Errorf("slot %d needs its own short key", i+1)
		}
		keys[sl.Key] = true
		if sl.Label == "" {
			sl.Label = sl.Key
		}
		switch sl.Kind {
		case model.SlotWhen:
			when++
		case model.SlotText, model.SlotPlace, model.SlotYesNo:
		case model.SlotChoice:
			if len(sl.Choices) < 2 {
				return fmt.Errorf("%s: a choice needs at least two options", sl.Label)
			}
		default:
			return fmt.Errorf("%s: unknown kind %q", sl.Label, sl.Kind)
		}
	}
	if when != 1 {
		return errors.New("a task has exactly one meeting time")
	}
	return nil
}

func checkPrivacy(p *model.TaskPrivacy) error {
	switch {
	case !slices.Contains([]string{model.StrategyTheyFirst, model.StrategyOffers, model.StrategyWindows}, p.Strategy):
		return errors.New("strategy must be they-first, offers or windows")
	case p.Offers < 1 || p.Offers > 3:
		return errors.New("offer 1 to 3 times at once")
	case p.ProbeLimit < 1 || p.ProbeLimit > 10:
		return errors.New("the probe limit is 1 to 10")
	case !slices.Contains([]string{model.ProbeOffer, model.ProbeAsk, model.ProbeHandoff}, p.OnProbeLimit):
		return errors.New("at the probe limit: offer, ask or handoff")
	case p.Cover != model.CoverDeflect && p.Cover != model.CoverLines:
		return errors.New("cover must be deflect or lines")
	case p.BufferMin < 0 || p.BufferMin > 60:
		return errors.New("the buffer is 0 to 60 minutes")
	}
	if len(p.CoverLines) > MaxCover {
		return fmt.Errorf("at most %d cover lines", MaxCover)
	}
	var lines []string
	for _, l := range p.CoverLines {
		if l = strings.TrimSpace(l); l == "" {
			continue
		}
		if utf8.RuneCountInString(l) > MaxCoverLine {
			return fmt.Errorf("a cover line has at most %d characters", MaxCoverLine)
		}
		lines = append(lines, l)
	}
	p.CoverLines = lines
	if p.CoverLines == nil {
		p.CoverLines = []string{}
	}
	return nil
}

// Settings are the defaults a task falls back to (settings.tasks).
type Settings struct {
	Privacy model.TaskPrivacy
	Limits  model.TaskLimits
	Commit  model.TaskCommit
}

// Resolve fills a spec's unset blocks from the defaults, so a run keeps
// what it was started with.
func Resolve(s model.TaskSpec, d Settings) model.TaskSpec {
	if s.Privacy == nil {
		p := d.Privacy
		p.CoverLines = slices.Clone(p.CoverLines)
		s.Privacy = &p
	}
	if s.Limits == nil {
		l := d.Limits
		s.Limits = &l
	}
	if s.Commit == nil {
		c := d.Commit
		s.Commit = &c
	}
	if len(s.Rules) == 0 {
		s.Rules = Rules(s)
	}
	return s
}

// Errors of running tasks (the engine returns them, the server maps them).
var (
	ErrRunning = errors.New("this chat already has a task running")
	ErrInvalid = errors.New("invalid task")
	ErrState   = errors.New("the task isn't waiting for that")
)
