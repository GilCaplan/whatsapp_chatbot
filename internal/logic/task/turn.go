package task

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"whatsappdoppel/internal/core/model"
)

// What triggers a turn.
const (
	EvStarted = "started" // the task starts and we write first
	EvMessage = "message" // they wrote
	EvNoReply = "no_reply"
)

// What the next message does (Turn.Say); the prompt turns it into words.
const (
	SayOffer      = "offer"       // Times
	SayWindows    = "windows"     // Windows
	SayAskTimes   = "ask_times"   // ask when suits them
	SayAnswerNo   = "answer_no"   // Their (or TheirDay) doesn't work; Times = alternatives (may be empty)
	SayAskConfirm = "ask_confirm" // Times[0]
	SayAskWhich   = "ask_which"   // Times = the offered ones
	SayAskSlot    = "ask_slot"    // Slot
	SayCover      = "cover"       // Cover ("" = deflect)
	SayChat       = "chat"        // answer in character, steer back
	SayNudge      = "nudge"       // Times = still open offers (may be empty)
	SayWait       = "wait"        // "no rush"
	SayConfirmed  = "confirmed"   // Times[0] is agreed (engine sets it after confirming)
	SayHold       = "hold"        // "let me confirm and get back to you" (waiting for your approval)
	SayHandoff    = "handoff"     // "let me get back to you on that"
	SayGoodbye    = "goodbye"     // ending without a time
	SayCheck      = "check"       // "let me check and get back to you" (the calendar can't be read right now)
)

// Input is one turn's trigger.
type Input struct {
	Event   string
	Reading model.TaskReading
	Their   []string // the texts of their new messages (evidence)
	MsgID   string   // their newest message
	Now     time.Time
}

// Turn is what code decided for the next message.
type Turn struct {
	Say       string
	Times     []time.Time
	Windows   []Window
	Their     *time.Time
	TheirDay  *time.Time
	TheirPart string
	Slot      *model.TaskSlot
	Cover     string
	Effects   []string // handoff, finish:ok, finish:fail, notify
	Commit    bool     // the engine confirms run.Proposed (approval or auto-confirm) first
	Rule      int      // -1 = no rule matched
	Facts     []string
}

// Has reports whether the turn has an effect.
func (t Turn) Has(effect string) bool { return slices.Contains(t.Effects, effect) }

// normText lowercases and keeps letters, digits and single spaces.
func normText(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// Found reports whether quote is (word for word) in one of texts; very
// short messages count as their own evidence.
func Found(quote string, texts []string) bool {
	q := normText(quote)
	all := normText(strings.Join(texts, " \n "))
	if q == "" {
		return len([]rune(all)) > 0 && len([]rune(all)) <= 40
	}
	return strings.Contains(all, q)
}

func (a Availability) parse(p model.TaskProposal) (day time.Time, at *time.Time, ok bool) {
	d, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(p.Date), a.loc())
	if err != nil {
		return time.Time{}, nil, false
	}
	if c, err := time.Parse("15:04", strings.TrimSpace(p.Time)); err == nil {
		t := time.Date(d.Year(), d.Month(), d.Day(), c.Hour(), c.Minute(), 0, 0, a.loc())
		return d, &t, true
	}
	return d, nil, true
}

// works is Availability.Works, kept consistent with what we already told
// them: a time we said doesn't work stays that way.
func works(run *model.TaskRun, a Availability, t time.Time) bool {
	for _, ans := range run.Answers {
		if ans.Time.Equal(t) && !ans.Works {
			return false
		}
	}
	return a.Works(t)
}

func missing(run *model.TaskRun) []model.TaskSlot {
	var out []model.TaskSlot
	for _, s := range run.Spec.Slots {
		if s.Kind == model.SlotWhen || !s.Required {
			continue
		}
		if v, ok := run.Values[s.Key]; !ok || strings.TrimSpace(v.Value) == "" {
			out = append(out, s)
		}
	}
	return out
}

// mergeValues keeps the slot values they gave whose words are found.
func mergeValues(run *model.TaskRun, in Input) []string {
	var got []string
	for _, f := range in.Reading.Values {
		i := slices.IndexFunc(run.Spec.Slots, func(s model.TaskSlot) bool { return s.Key == strings.ToLower(f.Key) })
		if i < 0 || run.Spec.Slots[i].Kind == model.SlotWhen {
			continue
		}
		sl := run.Spec.Slots[i]
		v := strings.TrimSpace(f.Value)
		if v == "" || len([]rune(v)) > 200 || !Found(f.Quote, in.Their) {
			continue
		}
		switch sl.Kind {
		case model.SlotChoice:
			j := slices.IndexFunc(sl.Choices, func(c string) bool { return strings.EqualFold(c, v) })
			if j < 0 {
				continue
			}
			v = sl.Choices[j]
		case model.SlotYesNo:
			switch strings.ToLower(v) {
			case "yes", "true", "כן":
				v = "yes"
			case "no", "false", "לא":
				v = "no"
			default:
				continue
			}
		}
		if run.Values == nil {
			run.Values = map[string]model.TaskValue{}
		}
		run.Values[sl.Key] = model.TaskValue{Value: v, Quote: f.Quote, MsgID: in.MsgID, At: in.Now}
		got = append(got, sl.Label+": "+v)
	}
	return got
}

// Decide reads a turn's input, updates run (values, answers, offers, step,
// counters, log) and returns what the next message must do. It never
// confirms: Commit asks the engine to.
func Decide(run *model.TaskRun, in Input, a Availability, pick Picker) Turn {
	spec := run.Spec
	priv := derefPrivacy(spec.Privacy)
	lim := derefLimits(spec.Limits)
	var facts []string
	var theirAt *time.Time
	var theirDay *time.Time
	theirPart := ""
	logf := func(kind, text string) {
		run.Log = append(run.Log, model.TaskEvent{At: in.Now, Kind: kind, Text: text, MsgID: in.MsgID})
	}
	switch in.Event {
	case EvStarted:
		facts = append(facts, CondStarted)
	case EvNoReply:
		if run.Nudges >= lim.MaxNudges {
			facts = append(facts, CondOutOfNudges)
		} else {
			facts = append(facts, CondNoReply)
		}
	case EvMessage:
		r := in.Reading
		intent := r.Intent
		if !slices.Contains(Intents, intent) {
			intent = "other"
		}
		if slices.Contains([]string{"accept", "reject", "propose"}, intent) && !Found(r.Quote, in.Their) {
			logf("read", "Their words for \""+intent+"\" weren't found in the message — read as other")
			intent, r.Times = "other", nil
		}
		if got := mergeValues(run, in); len(got) > 0 {
			logf("read", "Noted "+strings.Join(got, ", "))
		}
		if run.Sent >= lim.MaxMessages {
			facts = append(facts, CondLimit)
		}
		facts = append(facts, "intent:"+intent)
		// Times they gave.
		var exact []time.Time
		for _, p := range r.Times {
			d, at, ok := a.parse(p)
			if !ok {
				continue
			}
			if at != nil {
				exact = append(exact, *at)
			} else if theirDay == nil {
				dd := d
				theirDay, theirPart = &dd, p.Part
			}
		}
		if intent == "accept" && len(exact) == 0 && theirDay == nil && run.Step != model.StepConfirming {
			switch {
			case run.Proposed != nil:
				exact = []time.Time{*run.Proposed}
			case len(run.LastOffer) == 1:
				exact = []time.Time{run.LastOffer[0]}
			case len(run.LastOffer) > 1:
				facts = append(facts, CondUnclear)
			}
		}
		if run.Step == model.StepConfirming && run.Proposed != nil && len(exact) > 0 && !slices.ContainsFunc(exact, run.Proposed.Equal) {
			// They answered "shall we?" with another time: that's a new proposal.
			facts = slices.DeleteFunc(facts, func(f string) bool { return f == "intent:accept" })
			run.Step, run.Proposed = model.StepNegotiate, nil
		}
		if len(exact) > 0 {
			ok := -1
			for i, t := range exact {
				if works(run, a, t) {
					ok = i
					break
				}
			}
			if ok >= 0 {
				t := exact[ok]
				theirAt = &t
				facts = append(facts, CondTimeWorks)
			} else {
				t := exact[0]
				theirAt = &t
				if run.Probes >= priv.ProbeLimit {
					facts = append(facts, CondProbeLimit)
				}
				facts = append(facts, CondTimeNo)
			}
			for _, t := range exact {
				w := works(run, a, t)
				if !slices.ContainsFunc(run.Answers, func(x model.TaskAnswer) bool { return x.Time.Equal(t) }) {
					run.Answers = append(run.Answers, model.TaskAnswer{Time: t, Works: w})
				}
			}
		} else if theirDay != nil {
			facts = append(facts, CondTimeDay)
		}
		if run.Proposed != nil && run.Step == model.StepNegotiate && len(missing(run)) == 0 && theirAt == nil {
			facts = append(facts, CondReady)
		}
	}
	for _, s := range missing(run) {
		facts = append(facts, CondMissing+s.Key)
	}
	rules := Rules(spec)
	idx := Pick(rules, facts, run.Step, run.Fired)
	turn := Turn{Rule: idx, Facts: facts, Their: theirAt, TheirDay: theirDay, TheirPart: theirPart}
	do := []string{DoChat}
	if idx >= 0 {
		if run.Fired == nil {
			run.Fired = map[string]int{}
		}
		run.Fired[fmt.Sprint(idx)]++
		do = rules[idx].Do
		logf("rule", "Rule "+fmt.Sprint(idx+1)+": "+RuleText(rules[idx]))
	} else if in.Event == EvNoReply {
		do = nil
	}
	for _, act := range do {
		if IsEffect(act) {
			turn.Effects = append(turn.Effects, act)
			continue
		}
		apply(run, &turn, act, a, pick, priv)
	}
	if turn.Say == "" && in.Event != EvNoReply {
		switch {
		case turn.Has(DoHandoff):
			turn.Say = SayHandoff
		case turn.Has(DoFinishFail):
			turn.Say = SayGoodbye
		}
	}
	return turn
}

// apply turns one action into the turn (and the run's bookkeeping).
func apply(run *model.TaskRun, t *Turn, act string, a Availability, pick Picker, priv model.TaskPrivacy) {
	avoid := append(slices.Clone(run.Offered), declined(run)...)
	offer := func(n int, o OfferOpts) {
		o.Avoid, o.Prefer = avoid, run.Spec.Meeting.Prefer
		ts := a.Offer(n, o, pick)
		if len(ts) == 0 && len(avoid) > 0 {
			o.Avoid = declined(run) // offered before is better than nothing
			ts = a.Offer(n, o, pick)
		}
		t.Times = ts
		run.LastOffer = slices.Clone(ts)
		for _, x := range ts {
			if !slices.ContainsFunc(run.Offered, x.Equal) {
				run.Offered = append(run.Offered, x)
			}
		}
		if len(ts) == 0 {
			t.Say = SayAskTimes
		}
	}
	if act == DoStrategy {
		act = map[string]string{model.StrategyTheyFirst: DoAskTimes, model.StrategyOffers: DoOffer, model.StrategyWindows: DoWindows}[priv.Strategy]
	}
	if act == DoOnProbeLimit {
		switch priv.OnProbeLimit {
		case model.ProbeOffer:
			act = DoOffer
		case model.ProbeAsk:
			act = DoAskTimes
		default:
			t.Effects = append(t.Effects, DoHandoff)
			t.Say = SayHandoff
			return
		}
	}
	if run.Step == model.StepOpen {
		run.Step = model.StepNegotiate
	}
	switch act {
	case DoOffer, DoWindows, DoOfferOnDay, DoAnswerNo, DoAskTimes:
		if run.Step == model.StepConfirming { // looking again: the time we asked about is off the table
			run.Step, run.Proposed = model.StepNegotiate, nil
		}
	}
	switch {
	case act == DoOffer:
		t.Say = SayOffer
		offer(priv.Offers, OfferOpts{})
	case act == DoWindows:
		t.Say = SayWindows
		t.Windows = a.Windows(priv.Offers, avoid)
		if len(t.Windows) == 0 {
			t.Say = SayAskTimes
		}
	case act == DoOfferOnDay:
		n := priv.Offers
		if priv.Strategy == model.StrategyTheyFirst {
			n = 1
		}
		t.Say = SayOffer
		if t.TheirDay != nil {
			offer(n, OfferOpts{Day: t.TheirDay, Part: t.TheirPart})
			if len(t.Times) == 0 { // nothing that day: say so, offer other days
				t.Say = SayAnswerNo
				run.Probes++
				offer(n, OfferOpts{})
			}
		} else {
			offer(n, OfferOpts{})
		}
	case act == DoAnswerNo:
		t.Say = SayAnswerNo
		run.Probes++
		if t.Their != nil {
			if alt, ok := a.Alternative(*t.Their, avoid, pick); ok {
				t.Times = []time.Time{alt}
				run.LastOffer = []time.Time{alt}
				if !slices.ContainsFunc(run.Offered, alt.Equal) {
					run.Offered = append(run.Offered, alt)
				}
			}
		}
	case act == DoAskTimes:
		t.Say = SayAskTimes
	case act == DoAskConfirm:
		at := t.Their
		if at == nil {
			at = run.Proposed
		}
		if at == nil {
			t.Say = SayAskTimes
			return
		}
		p := *at
		run.Proposed = &p
		if m := missing(run); len(m) > 0 {
			t.Say, t.Slot = SayAskSlot, &m[0]
			return
		}
		run.Step = model.StepConfirming
		t.Say, t.Times = SayAskConfirm, []time.Time{p}
	case act == DoAskWhich:
		t.Say, t.Times = SayAskWhich, slices.Clone(run.LastOffer)
	case act == DoCommit:
		if run.Proposed == nil {
			t.Say = SayAskTimes
			return
		}
		if m := missing(run); len(m) > 0 {
			t.Say, t.Slot = SayAskSlot, &m[0]
			return
		}
		run.Step = model.StepCommitting
		t.Commit, t.Times = true, []time.Time{*run.Proposed}
	case strings.HasPrefix(act, DoAsk):
		key := strings.TrimPrefix(act, DoAsk)
		if i := slices.IndexFunc(run.Spec.Slots, func(s model.TaskSlot) bool { return s.Key == key }); i >= 0 {
			sl := run.Spec.Slots[i]
			t.Say, t.Slot = SayAskSlot, &sl
		} else {
			t.Say = SayChat
		}
	case act == DoCover:
		t.Say = SayCover
		if priv.Cover == model.CoverLines && len(priv.CoverLines) > 0 {
			t.Cover = priv.CoverLines[index(pick, len(priv.CoverLines))]
		}
	case act == DoNudge:
		t.Say = SayNudge
		run.Nudges++
		for _, x := range run.LastOffer {
			if a.Works(x) {
				t.Times = append(t.Times, x)
			}
		}
	case act == DoWait:
		t.Say = SayWait
	default:
		t.Say = SayChat
	}
}

// declined are the times we told them don't work.
func declined(run *model.TaskRun) []time.Time {
	var out []time.Time
	for _, a := range run.Answers {
		if !a.Works {
			out = append(out, a.Time)
		}
	}
	return out
}

// AllowedDays are the days a draft may name without a time: the parts of
// days it offers and the day they named.
func AllowedDays(t Turn) []time.Time {
	var out []time.Time
	for _, w := range t.Windows {
		out = append(out, w.Day)
	}
	if t.TheirDay != nil {
		out = append(out, *t.TheirDay)
	}
	return out
}

// Allowed are the times a draft may mention in this turn: what we offer now,
// what we offered before, theirs, and the agreed one.
func Allowed(run *model.TaskRun, t Turn) []time.Time {
	out := slices.Clone(t.Times)
	out = append(out, run.Offered...)
	for _, a := range run.Answers {
		out = append(out, a.Time)
	}
	if t.Their != nil {
		out = append(out, *t.Their)
	}
	if run.Proposed != nil {
		out = append(out, *run.Proposed)
	}
	return out
}

func derefPrivacy(p *model.TaskPrivacy) model.TaskPrivacy {
	if p == nil {
		return model.TaskPrivacy{Strategy: model.StrategyOffers, Offers: 2, ProbeLimit: 3, OnProbeLimit: model.ProbeHandoff, Cover: model.CoverDeflect}
	}
	return *p
}

func derefLimits(l *model.TaskLimits) model.TaskLimits {
	if l == nil {
		return model.TaskLimits{MaxMessages: 20, MaxDays: 7, MaxNudges: 2, NudgeAfterHours: 24}
	}
	return *l
}

// Summary is a short plain-English account of a run for you (hand-off,
// end): what was offered, what they asked about, where it stands.
func Summary(run *model.TaskRun, tone model.TaskTone, loc *time.Location, why string) string {
	if loc == nil {
		loc = time.Local
	}
	var parts []string
	if why != "" {
		parts = append(parts, why+".")
	}
	if len(run.Offered) > 0 {
		var ts []string
		for _, t := range run.Offered {
			ts = append(ts, When(t.In(loc), tone, ""))
		}
		parts = append(parts, "Offered: "+strings.Join(ts, ", ")+".")
	}
	if len(run.Answers) > 0 {
		var ts []string
		for _, a := range run.Answers {
			w := "works"
			if !a.Works {
				w = "doesn't work"
			}
			ts = append(ts, When(a.Time.In(loc), tone, "")+" ("+w+")")
		}
		parts = append(parts, "They asked about: "+strings.Join(ts, ", ")+".")
	}
	if run.Proposed != nil {
		parts = append(parts, "Last proposed: "+When(run.Proposed.In(loc), tone, "")+".")
	}
	var vals []string
	for _, s := range run.Spec.Slots {
		if v, ok := run.Values[s.Key]; ok && s.Kind != model.SlotWhen {
			vals = append(vals, s.Label+": "+v.Value)
		}
	}
	if len(vals) > 0 {
		parts = append(parts, "Noted: "+strings.Join(vals, ", ")+".")
	}
	parts = append(parts, fmt.Sprintf("%d messages sent, %d nudges.", run.Sent, run.Nudges))
	return strings.Join(parts, " ")
}
