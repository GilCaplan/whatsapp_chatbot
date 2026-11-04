package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/calendar"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/persona"
	"whatsappdoppel/internal/prompt"
	"whatsappdoppel/internal/store"
)

// Personal secretary (model.SecretaryConfig on the persona): its job goes
// into the prompt (prompt.SecretarySection), meetings get the owner's free
// times from the read-only calendars (busy times only, never titles), and a
// background job writes down what people want from the owner (notes.json,
// "Messages for you"); urgent ones and VIPs notify the owner at once.

const (
	freeDays        = 7
	calendarWait    = 20 * time.Second
	noteDelay       = 45 * time.Second // after the last message: one call per burst
	noteDelayUrgent = 5 * time.Second
	noteTimeout     = 60 * time.Second
	noteContext     = 6 // earlier messages read along for context
)

// urgentWords make the note job run at once (English + Hebrew).
var urgentWords = []string{"urgent", "asap", "emergency", "right now", "immediately", "as soon as possible", "דחוף", "מיד", "חירום", "בהקדם"}

func noteJob(key string) string { return "secnote:" + key }

// freeTimes lists the owner's free meeting times for the next week, one
// line per day ("Wed 4 Nov: 09:00–10:00, 12:00–18:00"); known=false when
// every calendar failed.
func (e *Engine) freeTimes(s *model.SecretaryConfig) ([]string, bool) {
	now := e.clock.Now()
	from := now.Truncate(15 * time.Minute).Add(15 * time.Minute)
	to := from.AddDate(0, 0, freeDays)
	var busy []calendar.Interval
	if s.UseCalendars && e.cal != nil {
		srcs := e.store.Calendars()
		ctx, cancel := context.WithTimeout(e.ctx, calendarWait)
		b, sts := e.cal.Busy(ctx, srcs, from, to, true)
		cancel()
		if len(sts) > 0 && !anyOK(sts) {
			return nil, false
		}
		busy = b
	}
	slots := calendar.FreeSlots(busy, from, to, persona.Weekdays(s.MeetDays), s.MeetFrom, s.MeetTo,
		time.Duration(s.MeetingMinutes)*time.Minute, time.Local)
	var lines []string
	var day string
	for _, sl := range slots {
		d := sl.Start.In(time.Local).Format("Mon 2 Jan")
		span := sl.Start.In(time.Local).Format("15:04") + "–" + sl.End.In(time.Local).Format("15:04")
		if d != day {
			lines = append(lines, d+": "+span)
			day = d
		} else {
			lines[len(lines)-1] += ", " + span
		}
	}
	return lines, true
}

func anyOK(sts []calendar.Status) bool {
	for _, s := range sts {
		if s.OK {
			return true
		}
	}
	return false
}

// CalendarCheck reads one calendar now (Settings › Calendars test).
func (e *Engine) CalendarCheck(ctx context.Context, src model.CalendarSource) model.CalendarCheck {
	if e.cal == nil {
		return model.CalendarCheck{Error: "calendars are not available", Upcoming: []model.CalendarItem{}}
	}
	evs, st := e.cal.Events(ctx, src, true)
	out := model.CalendarCheck{OK: st.OK, Error: st.Error, Upcoming: []model.CalendarItem{}}
	now := e.clock.Now()
	for _, ev := range evs {
		if !ev.End.After(now) {
			continue
		}
		out.Events++
		if len(out.Upcoming) < 5 {
			out.Upcoming = append(out.Upcoming, model.CalendarItem{Start: ev.Start, End: ev.End, AllDay: ev.AllDay, Title: ev.Title})
		}
	}
	return out
}

// secretaryOf is the chat's secretary block (nil = not a secretary).
func (e *Engine) secretaryOf(c model.ChatAssignment) (model.Persona, *model.SecretaryConfig) {
	p, ok := e.store.Persona(c.PersonaID)
	if !ok || p.Secretary == nil {
		return p, nil
	}
	return p, p.Secretary
}

// secretaryTick (re)arms the note job after a message from them: soon for
// urgent words or a VIP, else after a quiet spell.
func (e *Engine) secretaryTick(c model.ChatAssignment, in model.Incoming, text string) {
	_, s := e.secretaryOf(c)
	if s == nil || (!s.TakeMessages && !s.FlagUrgent) {
		return
	}
	d := noteDelay
	if s.FlagUrgent && (hasUrgentWord(text) || isVIP(s, in.PushName, in.SenderJID)) {
		d = noteDelayUrgent
	}
	key := c.Key
	e.schedule(noteJob(key), d, func(ctx context.Context) {
		if err := e.takeNotes(ctx, key); err != nil && ctx.Err() == nil {
			if cc, ok := e.store.Chat(key); ok {
				e.act(model.ActError, cc, "", "Couldn't take the message down: "+err.Error(), nil)
			}
		}
	})
}

func hasUrgentWord(text string) bool {
	low := strings.ToLower(text)
	for _, w := range urgentWords {
		if strings.Contains(low, w) {
			return true
		}
	}
	return false
}

// isVIP matches a sender's name or number against the secretary's VIPs.
func isVIP(s *model.SecretaryConfig, name, jid string) bool {
	user := behavior.UserOf(jid)
	for _, v := range s.VIPs {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(name)) && name != "" {
			return true
		}
		if digits := onlyDigits(v); len(digits) >= 6 && user != "" && strings.HasSuffix(user, digits) {
			return true
		}
	}
	return false
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// takeNotes reads the chat's messages since the note cursor and writes down
// what people want from the owner.
func (e *Engine) takeNotes(ctx context.Context, key string) error {
	c, ok := e.store.Chat(key)
	if !ok {
		return nil
	}
	p, s := e.secretaryOf(c)
	if s == nil {
		return nil
	}
	all, err := e.store.History(key, 0)
	if err != nil {
		return err
	}
	cursor := e.store.RunnerState(key).NoteCursor
	start := len(all)
	for i, m := range all {
		if m.TS.After(cursor) {
			start = i
			break
		}
	}
	fresh := all[start:]
	theirs := false
	for _, m := range fresh {
		theirs = theirs || m.Speaker == "them"
	}
	if len(fresh) == 0 {
		return nil
	}
	last := fresh[len(fresh)-1].TS
	if !theirs {
		e.updateState(key, func(st *store.RunnerState) { st.NoteCursor = last })
		return nil
	}
	var open []string
	for _, n := range e.store.SecretaryNotes() {
		if n.ChatKey == key && !n.Done && len(open) < 10 {
			open = append(open, n.From+": "+n.Text)
		}
	}
	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err != nil {
		return err
	}
	req := prompt.Notes(prompt.NotesInput{Owner: s.OwnerName, Assistant: p.Name, ChatName: c.Name, IsGroup: isGroupChat(c),
		Messages: all[max(0, start-noteContext):], Open: open, Zone: time.Local, Now: e.clock.Now()})
	req.Model = mdl
	cctx, cancel := context.WithTimeout(ctx, noteTimeout)
	resp, err := prov.Chat(cctx, req)
	cancel()
	if err != nil {
		return err
	}
	taken, err := prompt.ParseNotes(resp.Text)
	if err != nil {
		return errors.New("the model didn't write readable notes")
	}
	e.updateState(key, func(st *store.RunnerState) { st.NoteCursor = last })
	if len(taken) == 0 {
		return nil
	}
	senders := map[string]string{} // display name → JID, for VIPs
	for _, m := range fresh {
		if m.Speaker == "them" && m.Name != "" {
			senders[strings.ToLower(m.Name)] = m.SenderJID
		}
	}
	now := e.clock.Now()
	var added []model.SecretaryNote
	for _, t := range taken {
		from := t.From
		if from == "" && !isGroupChat(c) {
			from = c.Name
		}
		jid := senders[strings.ToLower(from)]
		if jid == "" && !isGroupChat(c) {
			jid = c.JID
		}
		n := model.SecretaryNote{ID: store.NewID(), ChatKey: key, ChatName: c.Name, From: from, FromJID: jid, Text: t.Text,
			Urgent: s.FlagUrgent && (t.Urgent || isVIP(s, from, jid)), Callback: t.Callback, Meeting: t.Meeting, At: now}
		added = append(added, n)
	}
	if err := e.store.UpdateSecretaryNotes(func(all []model.SecretaryNote) []model.SecretaryNote {
		// A new note from the same person in the same chat replaces their open one.
		for _, n := range added {
			for i := range all {
				if all[i].ChatKey == n.ChatKey && !all[i].Done && strings.EqualFold(all[i].From, n.From) {
					all[i].Done = true
				}
			}
		}
		return append(all, added...)
	}); err != nil {
		return err
	}
	owner := s.OwnerName
	if owner == "" {
		owner = "you"
	}
	for _, n := range added {
		text := fmt.Sprintf("Message for %s from %s: %s", owner, orName(n.From), n.Text)
		if n.Urgent {
			text = "Urgent — " + text
		}
		e.act(model.ActSecretaryNote, c, p.Name, text, map[string]any{"urgent": n.Urgent, "noteId": n.ID, "from": n.From})
	}
	if e.hub != nil {
		e.hub.Publish(events.TypeNotesChanged, struct{}{})
	}
	return nil
}
