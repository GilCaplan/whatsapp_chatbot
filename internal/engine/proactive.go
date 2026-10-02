package engine

import (
	"fmt"
	"time"

	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

// Proactive check-ins: while any enabled chat has proactive.enabled, an
// engine-level timer scans idle chats every proactiveEvery. A chat qualifies
// when it is available, has at least one message from them, has been quiet
// for afterHours, is under maxPerDay check-ins and its reply limits. The first
// time it qualifies a due time is drawn within spreadMinutes (persisted in
// runtime.json); once due, the persona thinks, types and sends an opener
// (or queues it for approval).

// syncProactive arms or stops the scan timer depending on whether any chat needs it.
func (e *Engine) syncProactive() {
	want := false
	for _, r := range e.allRunners() {
		if r.proactiveEnabled() {
			want = true
			break
		}
	}
	e.proMu.Lock()
	defer e.proMu.Unlock()
	if e.ctx.Err() != nil {
		return
	}
	switch {
	case want && e.proTimer == nil:
		e.proTimer = e.clock.AfterFunc(e.proactiveEvery, e.proactiveTick)
	case !want && e.proTimer != nil:
		e.proTimer.Stop()
		e.proTimer = nil
	}
}

func (e *Engine) proactiveTick() {
	e.proMu.Lock()
	e.proTimer = nil
	e.proMu.Unlock()
	if e.ctx.Err() != nil {
		return
	}
	now := e.clock.Now()
	for _, r := range e.allRunners() {
		r.proactiveCheck(now)
	}
	e.syncProactive()
}

func (r *Runner) proactiveEnabled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.stopped && r.cfg.Enabled && r.effLocked().Profile.Proactive.Enabled
}

// proactiveCheck schedules or starts a check-in for an idle chat.
func (r *Runner) proactiveCheck(now time.Time) {
	e := r.e
	var fx effects
	defer fx.run()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped || !r.cfg.Enabled || r.phase != phaseIdle || r.cfg.Handoff != nil { // paused for you (handoff.go): no check-ins
		return
	}
	eff := r.effLocked()
	bp := eff.Profile
	pr := bp.Proactive
	st := e.store.RunnerState(r.key)
	clearDue := func() {
		if st.ProactiveDueAt != nil {
			fx.add(func() { e.updateState(r.key, func(s *store.RunnerState) { s.ProactiveDueAt = nil }) })
		}
	}
	if !pr.Enabled {
		clearDue()
		return
	}
	if open, _ := eff.Available(now); !open {
		return
	}
	if _, busy := r.unreachableLocked(now); busy { // gym, sleep… (routine.go)
		return
	}
	var last time.Time
	hasThem := false
	for _, m := range r.history {
		if m.Speaker == "them" {
			hasThem = true
		}
		if m.TS.After(last) {
			last = m.TS
		}
	}
	if !hasThem {
		return
	}
	for _, t := range []time.Time{st.LastIncomingAt, st.LastReplyAt, st.LastYouRepliedAt} {
		if t.After(last) {
			last = t
		}
	}
	silence := now.Sub(last)
	if silence < time.Duration(pr.AfterHours)*time.Hour {
		clearDue()
		return
	}
	if st.ProactiveSince(now.Add(-24*time.Hour)) >= pr.MaxPerDay {
		return
	}
	if limited, _, _ := r.rateLimited(bp, now); limited {
		return
	}
	silenceHours := int(silence / time.Hour)
	due := st.ProactiveDueAt
	if due == nil {
		d := now.Add(e.rng.Between(0, pr.SpreadMinutes*60))
		due = &d
		if d.After(now) {
			fx.add(func() { e.updateState(r.key, func(s *store.RunnerState) { s.ProactiveDueAt = &d }) })
			r.actFx(&fx, model.ActProactive,
				fmt.Sprintf("Will check in within %s — quiet for %s", fmtDur(d.Sub(now).Round(time.Minute)), fmtDur(silence)),
				map[string]any{"stage": "scheduled", "silenceHours": silenceHours, "dueAt": d})
			return
		}
	}
	if now.Before(*due) {
		return
	}
	fx.add(func() { e.updateState(r.key, func(s *store.RunnerState) { s.ProactiveDueAt = nil }) })
	r.actFx(&fx, model.ActProactive, "Checking in after "+fmtDur(silence)+" of silence",
		map[string]any{"stage": "starting", "silenceHours": silenceHours})
	r.startCycleLocked(nil, false, true)
	r.thinkLocked(&fx)
}
