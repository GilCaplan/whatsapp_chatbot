package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/goals"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
	"whatsappdoppel/internal/store"
)

// Daily recap (wave 3). At settings recap.time (this Mac's zone) every
// enabled chat with at least recapMinMessages new messages gets a short
// private recap (one JSON call each, one chat at a time, never while a reply
// is being written). It can also run on demand. See WAVE3_PLAN §3.9.

const (
	recapMinMessages = 3
	recapMaxChats    = 20
	recapRetry       = 2 * time.Minute // a reply was being written: try again
	recapTimeout     = 60 * time.Second
	recapFallbackMsg = 30 // on demand, a quiet chat recaps its latest messages
)

// recapSched is the recap timer (Engine.recap).
type recapSched struct {
	mu    sync.Mutex
	timer Timer
	next  time.Time
}

// nextRecapAt is the next time hh:mm comes round after now in loc ("" or a
// malformed time = 21:00).
func nextRecapAt(now time.Time, hhmm string, loc *time.Location) time.Time {
	h, m := 21, 0
	if t, err := time.Parse("15:04", strings.TrimSpace(hhmm)); err == nil {
		h, m = t.Hour(), t.Minute()
	}
	l := now.In(loc)
	at := time.Date(l.Year(), l.Month(), l.Day(), h, m, 0, 0, loc)
	if !at.After(now) {
		at = time.Date(l.Year(), l.Month(), l.Day()+1, h, m, 0, 0, loc)
	}
	return at
}

// scheduleRecap (re)arms the daily recap from the current settings. It runs
// at start and after every settings change.
func (e *Engine) scheduleRecap() {
	e.armRecap(0)
}

// armRecap arms the recap timer: after d (> 0, a retry) or at the next
// recap.time. Disabled recaps (or a stopped engine) leave it off.
func (e *Engine) armRecap(d time.Duration) {
	s := e.cfg.Get().Recap
	e.recap.mu.Lock()
	defer e.recap.mu.Unlock()
	if e.recap.timer != nil {
		e.recap.timer.Stop()
		e.recap.timer = nil
	}
	e.recap.next = time.Time{}
	if !s.Enabled || e.ctx.Err() != nil {
		return
	}
	now := e.clock.Now()
	at := nextRecapAt(now, s.Time, time.Local)
	if d > 0 {
		at = now.Add(d)
	}
	e.recap.next = at
	e.recap.timer = e.clock.AfterFunc(at.Sub(now), func() {
		e.goBusy(e.recapTick)
	})
}

func (e *Engine) stopRecap() {
	e.recap.mu.Lock()
	if e.recap.timer != nil {
		e.recap.timer.Stop()
		e.recap.timer = nil
	}
	e.recap.mu.Unlock()
}

// recapTick runs the scheduled recap (own goroutine).
func (e *Engine) recapTick() {
	if e.ctx.Err() != nil {
		return
	}
	if e.anyGenerating() {
		e.armRecap(recapRetry)
		return
	}
	ctx, cancel := context.WithTimeout(e.ctx, recapMaxChats*recapTimeout)
	// Share the background slot with memory extraction so a local model never
	// runs two background calls at once.
	e.withJobSlot(ctx, func() { _, _ = e.runRecaps(ctx, "", false) })
	cancel()
	e.armRecap(0)
}

// anyGenerating reports whether some chat is writing a reply right now
// (background work yields to replies, so local models never run two calls).
func (e *Engine) anyGenerating() bool {
	for _, r := range e.allRunners() {
		r.mu.Lock()
		busy := r.cyc != nil && r.phase == phaseThinking && !r.cyc.genDone
		r.mu.Unlock()
		if busy {
			return true
		}
	}
	return false
}

// GenerateRecap writes recaps now: for one chat, or for every eligible chat
// when chatKey is "". Chats with nothing to recap are skipped (an empty
// list, not an error).
func (e *Engine) GenerateRecap(ctx context.Context, chatKey string) ([]model.Recap, error) {
	if chatKey != "" {
		if _, ok := e.store.Chat(chatKey); !ok {
			return nil, fmt.Errorf("chat %q: %w", chatKey, ErrNotFound)
		}
	}
	return e.runRecaps(ctx, chatKey, true)
}

// runRecaps writes the recaps of one run, one chat at a time.
func (e *Engine) runRecaps(ctx context.Context, chatKey string, onDemand bool) ([]model.Recap, error) {
	var chats []model.ChatAssignment
	for _, c := range e.store.Chats() {
		if chatKey != "" && c.Key == chatKey || chatKey == "" && c.Enabled {
			chats = append(chats, c)
		}
	}
	if len(chats) > recapMaxChats {
		chats = chats[:recapMaxChats]
	}
	var out []model.Recap
	var firstErr error
	for _, c := range chats {
		if ctx.Err() != nil {
			break
		}
		rec, ok, err := e.recapChat(ctx, c, onDemand, chatKey != "")
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			e.act(model.ActError, c, "", "Couldn't write the daily recap: "+err.Error(), map[string]any{"recap": true})
			continue
		}
		if ok {
			out = append(out, rec)
		}
	}
	if len(out) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return []model.Recap{}, ctx.Err()
	}
	names := make([]string, 0, len(out))
	for _, r := range out {
		names = append(names, orName(r.ChatName))
	}
	text := "Daily recap ready for " + joinNames(names)
	ev := model.ActivityEvent{Type: model.ActRecap, Text: text, Meta: map[string]any{"chats": len(out), "onDemand": onDemand, "headline": out[0].Headline}}
	if len(out) == 1 {
		ev.ChatKey, ev.ChatName, ev.PersonaName = out[0].ChatKey, out[0].ChatName, out[0].PersonaName
	}
	if e.hub != nil {
		e.hub.Activity(ev)
		e.hub.Publish(events.TypeRecapsChanged, struct{}{})
	}
	return out, nil
}

// recapChat writes one chat's recap. ok is false when there is nothing to
// recap. single: the chat was asked for by itself (on demand), so a quiet
// chat recaps its latest messages instead of being skipped.
func (e *Engine) recapChat(ctx context.Context, c model.ChatAssignment, onDemand, single bool) (model.Recap, bool, error) {
	now := e.clock.Now()
	st := e.store.RunnerState(c.Key)
	from := now.Add(-24 * time.Hour)
	if !onDemand && st.LastRecapAt.After(from) {
		from = st.LastRecapAt
	}
	all, err := e.store.History(c.Key, 0)
	if err != nil {
		return model.Recap{}, false, err
	}
	var msgs []model.Message
	for _, m := range all {
		if m.Kind == model.MsgKindFix {
			continue
		}
		if !m.TS.Before(from) {
			msgs = append(msgs, m)
		}
	}
	need := recapMinMessages
	if onDemand {
		need = 1
	}
	if len(msgs) < need {
		if !single || len(all) == 0 {
			return model.Recap{}, false, nil
		}
		msgs = all[max(0, len(all)-recapFallbackMsg):]
	}
	if len(msgs) > prompt.MaxRecapMessages {
		msgs = msgs[len(msgs)-prompt.MaxRecapMessages:]
	}
	p, ok := e.store.Persona(c.PersonaID)
	if !ok {
		p = model.Persona{Name: "The persona"}
	}
	in := prompt.RecapInput{
		ChatName: c.Name, PersonaName: p.Name, IsGroup: c.Kind == "group",
		Messages: msgs, Handoff: c.Handoff, Zone: time.Local,
	}
	if cfg := goals.Resolve(p, c); strings.TrimSpace(cfg.Text) != "" && (c.GoalOverride != "" || p.Goal != "") {
		gs := e.goalStatus(c.Key, cfg.Text)
		in.Goal, in.GoalReached, in.GoalPlan = cfg.Text, gs.Reached, gs.LastPlan
	}
	mems, _ := e.store.Memories(c.Key)
	for _, m := range mems {
		if !m.CreatedAt.Before(from) {
			who := m.Person
			if who != "" {
				who += ": "
			}
			in.Memories = append(in.Memories, who+m.Text)
		}
	}
	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err != nil {
		return model.Recap{}, false, err
	}
	req := prompt.Recap(in)
	req.Model = mdl
	cctx, cancel := context.WithTimeout(ctx, recapTimeout)
	resp, err := prov.Chat(cctx, req)
	cancel()
	if err != nil {
		return model.Recap{}, false, err
	}
	rc, err := prompt.ParseRecap(resp.Text)
	if err != nil {
		return model.Recap{}, false, errors.New("the model didn't write a readable recap")
	}
	rec := model.Recap{
		ID: store.NewID(), ChatKey: c.Key, ChatName: c.Name, PersonaName: p.Name,
		Date: now.In(time.Local).Format("2006-01-02"), From: msgs[0].TS, To: msgs[len(msgs)-1].TS, MessageCount: len(msgs),
		Headline: rc.Headline, Topics: rc.Topics, GoalProgress: rc.GoalProgress, ToKnow: rc.ToKnow, Mood: rc.Mood,
		GeneratedAt: now, OnDemand: onDemand,
	}
	if rec.Topics == nil {
		rec.Topics = []string{}
	}
	if rec.ToKnow == nil {
		rec.ToKnow = []string{}
	}
	if err := e.store.AppendRecap(rec, e.cfg.Get().Recap.KeepDays); err != nil {
		return model.Recap{}, false, err
	}
	if !onDemand {
		e.updateState(c.Key, func(s *store.RunnerState) { s.LastRecapAt = now })
	}
	return rec, true, nil
}

func orName(s string) string {
	if strings.TrimSpace(s) == "" {
		return "a chat"
	}
	return s
}

// joinNames renders "Dana", "Dana and Josh", "Dana, Josh and 2 more".
func joinNames(names []string) string {
	switch n := len(names); {
	case n == 0:
		return "no chats"
	case n == 1:
		return names[0]
	case n == 2:
		return names[0] + " and " + names[1]
	case n == 3:
		return names[0] + ", " + names[1] + " and " + names[2]
	default:
		return fmt.Sprintf("%s, %s and %d more", names[0], names[1], n-2)
	}
}

// settingsChanged runs after any settings change (config.Manager.OnChange):
// the recap timer follows recap.enabled / recap.time. Everything else reads
// the settings when it needs them (behaviour, hand-off, notifications).
func (e *Engine) settingsChanged() {
	if e.ctx.Err() != nil {
		return
	}
	e.scheduleRecap()
}
