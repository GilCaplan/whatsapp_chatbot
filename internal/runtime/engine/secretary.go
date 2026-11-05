package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"whatsappdoppel/internal/core/events"
	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/core/store"
	"whatsappdoppel/internal/logic/behavior"
	"whatsappdoppel/internal/logic/calendar"
	"whatsappdoppel/internal/runtime/prompt"
)

// Personal secretary (model.SecretaryConfig on the persona): its job goes
// into the prompt (prompt.SecretarySection), a meeting request starts a Find
// a time task (task.go: it only ever sees the times it may offer), and a
// background job writes down what people want from the owner (notes.json,
// "Messages for you"); urgent ones and VIPs notify the owner at once.

const (
	calendarWait    = 20 * time.Second
	noteDelay       = 45 * time.Second // after the last message: one call per burst
	noteDelayUrgent = 5 * time.Second
	noteTimeout     = 60 * time.Second
	noteContext     = 6 // earlier messages read along for context
)

// urgentWords make the note job run at once (English + Hebrew).
var urgentWords = []string{"urgent", "asap", "emergency", "right now", "immediately", "as soon as possible", "דחוף", "מיד", "חירום", "בהקדם"}

func noteJob(key string) string { return "secnote:" + key }

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
	switch {
	case s.FlagUrgent && (hasUrgentWord(text) || isVIP(s, in.PushName, in.SenderJID)):
		d = noteDelayUrgent
	case e.paramsOf(c).Background == model.BackgroundDigest:
		return // the digest takes the notes (digest.go); urgent ones still go at once
	}
	key := c.Key
	e.scheduleYield(noteJob(key), d, func(ctx context.Context) {
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
	nb, err := e.notesBatch(c)
	if err != nil || nb.done {
		return err
	}
	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err != nil {
		return err
	}
	req := prompt.Notes(prompt.NotesInput{Owner: s.OwnerName, Assistant: p.Name, ChatName: c.Name, IsGroup: isGroupChat(c),
		Messages: nb.read, Open: nb.open, Zone: time.Local, Now: e.clock.Now()})
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
	return e.saveNotes(c, p, s, nb, taken)
}

// noteBatch is what the next note-taking reads.
type noteBatch struct {
	fresh []model.Message // since the note cursor
	read  []model.Message // fresh plus a little context
	open  []string        // notes already taken in this chat
	last  time.Time
	done  bool // nothing from them: the cursor already moved on
}

func (e *Engine) notesBatch(c model.ChatAssignment) (noteBatch, error) {
	var nb noteBatch
	all, err := e.store.History(c.Key, 0)
	if err != nil {
		return nb, err
	}
	cursor := e.store.RunnerState(c.Key).NoteCursor
	start := len(all)
	for i, m := range all {
		if m.TS.After(cursor) {
			start = i
			break
		}
	}
	nb.fresh = all[start:]
	if len(nb.fresh) == 0 {
		nb.done = true
		return nb, nil
	}
	nb.last = nb.fresh[len(nb.fresh)-1].TS
	if !slices.ContainsFunc(nb.fresh, func(m model.Message) bool { return m.Speaker == "them" }) {
		e.updateState(c.Key, func(st *store.RunnerState) { st.NoteCursor = nb.last })
		nb.done = true
		return nb, nil
	}
	nb.read = all[max(0, start-noteContext):]
	for _, n := range e.store.SecretaryNotes() {
		if n.ChatKey == c.Key && !n.Done && len(nb.open) < 10 {
			nb.open = append(nb.open, n.From+": "+n.Text)
		}
	}
	return nb, nil
}

// saveNotes stores the notes taken from a batch, moves the cursor and
// tells the owner (also the digest's part, digest.go).
func (e *Engine) saveNotes(c model.ChatAssignment, p model.Persona, s *model.SecretaryConfig, nb noteBatch, taken []prompt.TakenNote) error {
	key := c.Key
	e.updateState(key, func(st *store.RunnerState) { st.NoteCursor = nb.last })
	if len(taken) == 0 {
		return nil
	}
	senders := map[string]string{} // display name → JID, for VIPs
	for _, m := range nb.fresh {
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
