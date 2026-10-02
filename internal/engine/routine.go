package engine

import (
	"fmt"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
	"whatsappdoppel/internal/store"
	"whatsappdoppel/internal/world"
)

// Daily life (wave 3, Engineer A; WAVE3_PLAN §3.2–3.3): the persona lives
// in its own time zone and follows its routine. During an "unreachable"
// block (gym, sleep) new messages wait until the block ends; during a
// "slow" block (work) it notices messages later; a reply written long after
// the message gets a late-reply note in the prompt so the persona can say
// "sorry, was at the gym" when it fits.

const (
	slowNoticeFactor = 3                // "slow" routine blocks: notice delay ×3
	slowNoticeCap    = time.Hour        // …but never more than this
	lateAfterRoutine = 10 * time.Minute // a routine block in between: note from 10 min
	lateGeneric      = 45 * time.Minute // no routine reason: note from 45 min
)

// personaWorld is the chat persona's World (zero when unknown).
func (e *Engine) personaWorld(c model.ChatAssignment) model.World {
	if p, ok := e.store.Persona(c.PersonaID); ok {
		return p.World
	}
	return model.World{}
}

// withPersonaZone resolves the "persona" active-hours zone marker.
func (e *Engine) withPersonaZone(eff behavior.Effective, c model.ChatAssignment) behavior.Effective {
	if !behavior.FollowsPersona(eff.Profile.Availability) {
		return eff
	}
	return behavior.WithZone(eff, e.personaWorld(c).Timezone)
}

// unreachableLocked returns the routine block that keeps the persona off its
// phone at now, if any.
func (r *Runner) unreachableLocked(now time.Time) (world.Active, bool) {
	return world.Unreachable(r.e.personaWorld(r.cfg), now)
}

// routineGateLocked parks a new message (idle/queued chat) while the
// persona is in an unreachable routine block. It reports whether it did.
func (r *Runner) routineGateLocked(fx *effects, eff behavior.Effective, pm pendingMsg, now time.Time) bool {
	a, ok := r.unreachableLocked(now)
	if !ok {
		return false
	}
	if r.phase == phaseQueued {
		r.cyc.pending = append(r.cyc.pending, pm)
		return true
	}
	r.startCycleLocked([]pendingMsg{pm}, false, false)
	r.routineParkLocked(fx, eff, now, a)
	return true
}

// routineParkLocked queues the current cycle until the block ends (plus a
// catch-up delay, like active hours).
func (r *Runner) routineParkLocked(fx *effects, eff behavior.Effective, now time.Time, a world.Active) {
	wake := a.End.Add(r.e.rng.Between(0, eff.Profile.Availability.CatchUpMaxMin*60))
	r.phase = phaseQueued
	r.armLocked(wake.Sub(now), r.wakeLocked)
	key := r.key
	fx.add(func() { r.e.updateState(key, func(st *store.RunnerState) { w := wake; st.QueuedWakeAt = &w }) })
	loc := world.Location(r.e.personaWorld(r.cfg))
	text := fmt.Sprintf("Busy (%s) until %s — will reply after", a.Block.Label, fmtClock(a.End, now, loc))
	r.actFx(fx, model.ActDeferred, text, map[string]any{"reason": "routine", "block": a.Block.Label, "resumeAt": wake})
}

// slowNotice stretches a notice delay during a "slow" routine block and
// says so in the activity feed (label "" = not slowed).
func (r *Runner) slowNoticeLocked(d time.Duration, now time.Time) (time.Duration, string) {
	a, ok := world.RoutineAt(r.e.personaWorld(r.cfg), now)
	if !ok || a.Block.Reach != model.ReachSlow {
		return d, ""
	}
	if d <= 0 {
		d = r.e.rng.Between(30, 120)
	}
	return min(d*slowNoticeFactor, max(d, slowNoticeCap)), a.Block.Label
}

// lateNote is the late-reply note for a reply written at now: how long
// after the last message from them, and the routine block that kept the
// persona away (nil = not late).
func (e *Engine) lateNote(p model.Persona, hist []model.Message, now time.Time) *prompt.LateNote {
	var at time.Time
	for i := len(hist) - 1; i >= 0; i-- {
		if hist[i].Speaker == "them" {
			at = hist[i].TS
			break
		}
		if hist[i].Speaker == "me" && hist[i].FromBot {
			return nil // already answered since
		}
	}
	if at.IsZero() || now.Before(at) {
		return nil
	}
	late := now.Sub(at)
	if late < lateAfterRoutine {
		return nil
	}
	mins := int(late / time.Minute)
	loc := world.Location(p.World)
	var busy *world.Active
	for _, a := range world.RoutineBetween(p.World, at, now) {
		if a.Block.Reach == model.ReachNormal {
			continue
		}
		if busy == nil || a.End.After(busy.End) {
			x := a
			busy = &x
		}
	}
	if busy != nil {
		return &prompt.LateNote{Minutes: mins, Busy: busy.Block.Label + " until " + busy.End.In(loc).Format("15:04")}
	}
	if late < lateGeneric {
		return nil
	}
	return &prompt.LateNote{Minutes: mins}
}
