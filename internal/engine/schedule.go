package engine

import (
	"fmt"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

// Scheduled messages (proactive.schedule, behavior/schedule.go). Besides
// silence check-ins (proactive.go) a chat can plan openers: one when the
// persona starts in the chat (replies turned on, or you join the group), one
// at each fixed moment, and repeats with a fixed or random gap from the start
// on. Each runner keeps one timer for its next moment.
//
// When a moment comes the chat must be able to take a message right then:
// WhatsApp connected, still in the group, not paused or away, inside active
// hours, idle and under the daily cap. Otherwise the moment is skipped (the
// activity feed says why) and the next one is planned. A missed moment is
// never sent late — not after a disconnect, not after the app was closed —
// so messages never pile up and go out in one go.

// scheduleGrace is how late a moment may still fire (timer slack).
const scheduleGrace = 2 * time.Minute

// Triggers of a scheduled message (activity meta "trigger").
const (
	trigStart  = "start"  // replies turned on in the chat
	trigJoined = "joined" // you joined (or were added back to) the group
	trigTime   = "time"   // a fixed moment
	trigRepeat = "repeat"
)

// syncSchedule re-plans the chat's next scheduled moment (settings changed).
func (r *Runner) syncSchedule() {
	var fx effects
	r.mu.Lock()
	r.syncScheduleLocked(&fx, r.e.clock.Now())
	r.mu.Unlock()
	fx.run()
}

// scheduleStart runs when the persona starts in the chat: the start message
// (onStart) and, unless a fixed moment is still ahead, repeats count from now.
func (r *Runner) scheduleStart(trigger string) {
	e := r.e
	var fx effects
	r.mu.Lock()
	defer func() {
		r.mu.Unlock()
		fx.run()
	}()
	if r.stopped || !r.cfg.Enabled {
		return
	}
	now := e.clock.Now()
	sc := r.effLocked().Profile.Proactive.Schedule
	st := e.store.RunnerState(r.key)
	if behavior.Repeats(sc) && behavior.NextTime(sc, st.ScheduleDoneTo, now).IsZero() {
		e.updateState(r.key, func(s *store.RunnerState) { s.ScheduleNextAt, s.ScheduleRepeat = nil, "" })
	}
	if sc.OnStart {
		r.scheduledLocked(&fx, now, trigger, time.Time{})
	}
	r.syncScheduleLocked(&fx, now)
}

func (r *Runner) stopScheduleLocked() {
	if r.sched != nil {
		r.sched.Stop()
		r.sched = nil
	}
	r.schedGen++
}

// syncScheduleLocked skips moments that already passed, draws the next
// repeat when needed and arms the timer for the earliest moment ahead.
func (r *Runner) syncScheduleLocked(fx *effects, now time.Time) {
	e := r.e
	r.stopScheduleLocked()
	if r.stopped || !r.cfg.Enabled {
		return
	}
	sc := r.effLocked().Profile.Proactive.Schedule
	st := e.store.RunnerState(r.key)
	next := st
	loc := behavior.Location(r.effLocked().Profile.Availability)

	// Fixed moments that passed unhandled: the app was closed or the chat off.
	var missed []time.Time
	for _, t := range sc.Times {
		if t.After(next.ScheduleDoneTo) && t.Before(now.Add(-scheduleGrace)) {
			missed = append(missed, t)
		}
	}
	if n := len(missed); n > 0 {
		next.ScheduleDoneTo = missed[n-1]
		text := "Skipped the message planned for " + fmtWhen(missed[0], now, loc)
		if n > 1 {
			text = fmt.Sprintf("Skipped %d planned messages", n)
		}
		r.actFx(fx, model.ActProactive, text+" — the chat was off or the app wasn't running then",
			map[string]any{"stage": "skipped", "reason": "missed", "trigger": trigTime, "count": n})
	}

	// Repeats: draw the next one when there is none, the settings changed
	// or it was missed.
	key := behavior.RepeatKey(sc)
	switch {
	case key == "":
		next.ScheduleNextAt, next.ScheduleRepeat = nil, ""
	case next.ScheduleNextAt == nil || next.ScheduleRepeat != key:
		anchor := now
		if first := behavior.NextTime(sc, next.ScheduleDoneTo, now); !first.IsZero() {
			anchor = first // repeats start at the first fixed moment
		}
		t := anchor.Add(behavior.ScheduleGap(e.rng, sc))
		next.ScheduleNextAt, next.ScheduleRepeat = &t, key
	case next.ScheduleNextAt.Before(now.Add(-scheduleGrace)):
		r.actFx(fx, model.ActProactive, "Skipped the repeating message planned for "+fmtWhen(*next.ScheduleNextAt, now, loc)+
			" — the chat was off or the app wasn't running then",
			map[string]any{"stage": "skipped", "reason": "missed", "trigger": trigRepeat})
		t := now.Add(behavior.ScheduleGap(e.rng, sc))
		next.ScheduleNextAt = &t
	}

	at := behavior.NextTime(sc, next.ScheduleDoneTo, now.Add(-scheduleGrace))
	if n := next.ScheduleNextAt; n != nil && (at.IsZero() || n.Before(at)) {
		at = *n
	}
	next.ScheduleAt = nil
	if !at.IsZero() {
		next.ScheduleAt = &at
	}
	if !next.ScheduleDoneTo.Equal(st.ScheduleDoneTo) || !timePtrEqual(next.ScheduleNextAt, st.ScheduleNextAt) ||
		next.ScheduleRepeat != st.ScheduleRepeat || !timePtrEqual(next.ScheduleAt, st.ScheduleAt) {
		e.updateState(r.key, func(s *store.RunnerState) {
			s.ScheduleDoneTo, s.ScheduleNextAt, s.ScheduleRepeat, s.ScheduleAt = next.ScheduleDoneTo, next.ScheduleNextAt, next.ScheduleRepeat, next.ScheduleAt
		})
	}
	if at.IsZero() {
		return
	}
	gen := r.schedGen
	r.sched = e.clock.AfterFunc(max(at.Sub(now), 0), func() {
		var fx effects
		r.mu.Lock()
		if r.stopped || gen != r.schedGen {
			r.mu.Unlock()
			return
		}
		r.sched = nil
		r.scheduleDueLocked(&fx, e.clock.Now())
		r.mu.Unlock()
		fx.run()
	})
}

// scheduleDueLocked handles the moments that are due now. Several due at
// once (a fixed moment and a repeat) make a single message.
func (r *Runner) scheduleDueLocked(fx *effects, now time.Time) {
	e := r.e
	sc := r.effLocked().Profile.Proactive.Schedule
	st := e.store.RunnerState(r.key)
	doneTo, planned, trigger := st.ScheduleDoneTo, time.Time{}, ""
	for _, t := range sc.Times {
		if t.After(doneTo) && !t.After(now) {
			doneTo, planned, trigger = t, t, trigTime
		}
	}
	var nextAt *time.Time
	if n := st.ScheduleNextAt; n != nil && !now.Before(*n) && st.ScheduleRepeat == behavior.RepeatKey(sc) {
		if trigger == "" {
			planned, trigger = *n, trigRepeat
		}
		t := n.Add(behavior.ScheduleGap(e.rng, sc))
		if !t.After(now) {
			t = now.Add(behavior.ScheduleGap(e.rng, sc))
		}
		nextAt = &t
	}
	if trigger != "" {
		e.updateState(r.key, func(s *store.RunnerState) {
			s.ScheduleDoneTo = doneTo
			if nextAt != nil {
				s.ScheduleNextAt = nextAt
			}
		})
		r.scheduledLocked(fx, now, trigger, planned)
	}
	r.syncScheduleLocked(fx, now)
}

// scheduledLocked starts a scheduled opener now, or skips it with the reason.
func (r *Runner) scheduledLocked(fx *effects, now time.Time, trigger string, planned time.Time) {
	e := r.e
	meta := map[string]any{"trigger": trigger}
	if !planned.IsZero() {
		meta["plannedAt"] = planned
	}
	if reason, why := r.scheduleBlockedLocked(now); reason != "" {
		meta["stage"], meta["reason"] = "skipped", reason
		r.actFx(fx, model.ActProactive, "Skipped a scheduled message — "+why, meta)
		return
	}
	e.updateState(r.key, func(s *store.RunnerState) { s.Scheduled = append(s.Scheduled, now) })
	text := "Sending a scheduled message"
	switch trigger {
	case trigStart:
		text = "Starting the conversation — replies were just turned on"
	case trigJoined:
		text = "Starting the conversation — you just joined the group"
	case trigTime:
		text = "Sending the message planned for " + fmtWhen(planned, now, behavior.Location(r.effLocked().Profile.Availability))
	}
	if r.cfg.ApprovalMode {
		text += " (it will wait for your approval)"
	}
	meta["stage"] = "starting"
	meta["scheduled"] = true
	r.actFx(fx, model.ActProactive, text, meta)
	cyc := r.startCycleLocked(nil, false, true)
	cyc.scheduled = true
	r.thinkLocked(fx)
}

// scheduleBlockedLocked reports why the chat can't take a scheduled message
// right now (reason code, plain-English why), or "" when it can.
func (r *Runner) scheduleBlockedLocked(now time.Time) (string, string) {
	e := r.e
	c := r.cfg
	eff := r.effLocked()
	switch {
	case e.wa.Status().State != model.WAConnected:
		return "disconnected", "WhatsApp was disconnected, so it wasn't sent late either"
	case c.Kind == "group" && !e.stillInGroup(c):
		return "left_group", "you're no longer in this group"
	case c.Handoff != nil:
		return "handoff", "the chat is paused until you reply yourself"
	case eff.Snoozed(now):
		return "away", "the chat is set to Away"
	case r.phase != phaseIdle:
		return "busy", "the persona was in the middle of a reply"
	}
	if open, _ := eff.Available(now); !open {
		return "closed", "it was outside active hours"
	}
	if _, busy := r.unreachableLocked(now); busy {
		return "routine", "the persona's routine had it away from the phone"
	}
	sc := eff.Profile.Proactive.Schedule
	if n := behavior.ScheduleCap(sc); e.store.RunnerState(r.key).ScheduledSince(now.Add(-24*time.Hour)) >= n {
		return "cap", fmt.Sprintf("it already sent %d scheduled messages in the last 24 hours", n)
	}
	return "", ""
}

// stillInGroup reports whether you are still in the chat's group (true when unknown).
func (e *Engine) stillInGroup(c model.ChatAssignment) bool {
	for _, jid := range []string{c.JID, c.AltJID} {
		if jid == "" {
			continue
		}
		if member, known := e.wa.InGroup(jid); known {
			return member
		}
	}
	return true
}

// HandleGroupChange reacts to groups you join or leave (wa layer): joining
// an assigned group starts the persona there (start message, repeats).
func (e *Engine) HandleGroupChange(gc model.GroupChange) {
	if !gc.Joined {
		return // nothing is sent there any more: scheduled moments are skipped
	}
	r := e.runner(gc.Key)
	if r == nil {
		if c, ok := e.store.ChatByJID(gc.JID); ok {
			r = e.runner(c.Key)
		}
	}
	if r != nil {
		r.scheduleStart(trigJoined)
	}
}

func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// fmtWhen renders a planned moment: "14:00" today, else "Mon 14:00", else a date.
func fmtWhen(t, now time.Time, loc *time.Location) string {
	t = t.In(loc)
	switch {
	case sameDay(t, now.In(loc)):
		return t.Format("15:04")
	case absDur(t.Sub(now)) < 6*24*time.Hour:
		return t.Format("Mon 15:04")
	}
	return t.Format("2 Jan 15:04")
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
