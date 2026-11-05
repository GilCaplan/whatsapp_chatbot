package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/core/store"
	"whatsappdoppel/internal/logic/behavior"
	"whatsappdoppel/internal/logic/mention"
	"whatsappdoppel/internal/runtime/llm"
	"whatsappdoppel/internal/runtime/prompt"
)

// The reply cycle of one chat:
//
//	idle ─msg→ noticing ─notice→ waiting(debounce) ─→ thinking(+LLM) ─→ typing(bubbles) ─→ idle
//	idle ─msg outside hours (queue)→ queued ─wake→ noticing …
//
// New messages: noticing → join; waiting → debounce reset (burst cap);
// thinking → dirty (a draft that came quickly is discarded and regenerated, a
// slow one is sent: see keepDraftLocked); typing → answered by the next cycle
// right after. Your own reply cancels the cycle (pauseWhenYouReply).
type phase int

const (
	phaseIdle phase = iota
	phaseNoticing
	phaseWaiting
	phaseThinking
	phaseTyping
	phaseQueued
)

func (p phase) String() string {
	return [...]string{"idle", "noticing", "waiting", "thinking", "typing", "queued"}[p]
}

// pendingMsg is an incoming message the current cycle answers.
type pendingMsg struct {
	ID, SenderJID, ChatJID, Name, Text string
	read                               bool
}

// cycle is one attempt to answer (or, proactively, to start) a conversation.
type cycle struct {
	id     uint64
	ctx    context.Context
	cancel context.CancelFunc

	pending        []pendingMsg
	later          []pendingMsg // arrived while typing: answered by the next cycle
	laterImmediate bool
	immediate      bool // trigger prefix
	proactive      bool
	scheduled      bool          // proactive: a scheduled message (schedule.go), not a check-in
	opener         prompt.Opener // proactive: manual tap / hint (opener.go)
	approval       bool
	copilot        bool // co-pilot mode: three drafts for you to pick from (drafts.go)

	burstStart time.Time
	resets     int

	thinkDone   bool
	thinkEnd    time.Time
	typingShown bool
	genDone     bool
	draft       *genResult
	genErr      error
	dirty       bool
	drafted     int       // pending messages the running generation can see
	restarts    int       // drafts thrown away for newer messages
	genStart    time.Time // when the running generation started
	persona     model.Persona
	hist        []model.Message
	dir         *mention.Directory // group members for encoding tags (nil in DMs)
	mentions    []string           // display names the draft tags
	task        *taskUse           // a running task's turn: saved once sent, dropped otherwise (task.go)
	sent        atomic.Int32       // bubbles delivered so far (send goroutine)
}

func (r *Runner) startCycleLocked(pending []pendingMsg, immediate, proactive bool) *cycle {
	r.cycleSeq++
	ctx, cancel := context.WithCancel(r.ctx)
	cyc := &cycle{id: r.cycleSeq, ctx: ctx, cancel: cancel, pending: pending, immediate: immediate, proactive: proactive}
	r.cyc = cyc
	return cyc
}

// endCycleLocked returns to idle: timer stopped, in-flight work cancelled, typing cleared.
func (r *Runner) endCycleLocked(fx *effects) {
	r.stopTimerLocked()
	if r.cyc != nil {
		r.taskFxLocked(fx, r.cyc, r.cyc.sent.Load() > 0) // cut short: it counts if a bubble went out
		r.cyc.cancel()
		r.cyc = nil
	}
	r.phase = phaseIdle
	fx.add(r.typingFxLocked(false))
}

// ─── timers ──────────────────────────────────────────────────

// armLocked (re)arms the single phase timer; fn runs under mu.
func (r *Runner) armLocked(d time.Duration, fn func(fx *effects)) {
	r.stopTimerLocked()
	gen := r.timerGen
	r.timer = r.e.clock.AfterFunc(max(d, 0), func() {
		var fx effects
		r.mu.Lock()
		if r.stopped || gen != r.timerGen {
			r.mu.Unlock()
			return
		}
		r.timer = nil
		fn(&fx)
		r.mu.Unlock()
		fx.run()
	})
}

func (r *Runner) stopTimerLocked() {
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	r.timerGen++
}

// afterLocked runs fn (under mu) after d on an auxiliary timer (reactions,
// read receipts of skipped messages). d <= 0 runs it now.
func (r *Runner) afterLocked(fx *effects, d time.Duration, fn func(fx *effects)) {
	if d <= 0 {
		fn(fx)
		return
	}
	r.auxSeq++
	id := r.auxSeq
	r.aux[id] = r.e.clock.AfterFunc(d, func() {
		var fx effects
		r.mu.Lock()
		if r.stopped {
			r.mu.Unlock()
			return
		}
		delete(r.aux, id)
		fn(&fx)
		r.mu.Unlock()
		fx.run()
	})
}

// ─── side-effect helpers ─────────────────────────────────────

// typingFxLocked records a typing on/off decision and returns the call to make
// after unlocking. Calls are applied in decision order (late ones are dropped).
func (r *Runner) typingFxLocked(on bool) func() {
	if !on && !r.typOn {
		return func() {}
	}
	if on {
		r.typJID = r.sendJIDOrDefaultLocked()
	}
	r.typOn = on
	r.typSeq++
	seq, jid := r.typSeq, r.typJID
	return func() {
		r.typMu.Lock()
		defer r.typMu.Unlock()
		if seq <= r.typApplied {
			return
		}
		r.typApplied = seq
		r.e.typing(jid, on)
	}
}

// setTyping switches typing for a delivery; nothing happens for a cancelled cycle (on).
func (r *Runner) setTyping(cyc *cycle, on bool) {
	r.mu.Lock()
	if on && (cyc == nil || r.cyc != cyc) {
		r.mu.Unlock()
		return
	}
	fn := r.typingFxLocked(on)
	r.mu.Unlock()
	fn()
}

// setTypingDirect switches typing outside a cycle (approved replies).
func (r *Runner) setTypingDirect(on bool) {
	r.mu.Lock()
	fn := r.typingFxLocked(on)
	r.mu.Unlock()
	fn()
}

// markReadFxLocked queues read receipts for msgs (grouped per sender, as
// WhatsApp requires) and flags them read.
func (r *Runner) markReadFxLocked(fx *effects, msgs []*pendingMsg) {
	type key struct{ chat, sender string }
	groups := map[key][]string{}
	var order []key
	for _, m := range msgs {
		m.read = true
		if m.ID == "" {
			continue
		}
		chat := m.ChatJID
		if chat == "" {
			chat = r.sendJIDOrDefaultLocked()
		}
		k := key{chat, m.SenderJID}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], m.ID)
	}
	wa := r.e.wa
	for _, k := range order {
		ids := groups[k]
		fx.add(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = wa.MarkRead(ctx, k.chat, k.sender, ids)
		})
	}
}

func (r *Runner) actFx(fx *effects, typ, text string, meta map[string]any) {
	c, e := r.cfg, r.e
	fx.add(func() { e.act(typ, c, "", text, meta) })
}

// ─── intake gates ────────────────────────────────────────────

// admitLocked applies availability, rate limits and reply chance to a new
// message (in that order) and joins or starts a reply cycle.
// forced (a priority person or a trigger word) skips the reply-chance roll.
func (r *Runner) admitLocked(fx *effects, c model.ChatAssignment, eff behavior.Effective, p model.Persona, pm pendingMsg, immediate, forced bool, now time.Time) {
	bp := eff.Profile
	if immediate {
		// Trigger prefix: skip notice/wait/think, availability and limits.
		switch r.phase {
		case phaseIdle:
			r.startCycleLocked([]pendingMsg{pm}, true, false)
			r.thinkLocked(fx)
		case phaseNoticing, phaseWaiting, phaseQueued:
			if r.phase == phaseQueued {
				fx.add(func() { r.e.updateState(c.Key, func(st *store.RunnerState) { st.QueuedWakeAt = nil }) })
			}
			r.cyc.pending = append(r.cyc.pending, pm)
			r.cyc.immediate = true
			r.thinkLocked(fx)
		case phaseThinking:
			r.cyc.pending = append(r.cyc.pending, pm)
			r.cyc.immediate, r.cyc.dirty = true, true
		case phaseTyping:
			r.cyc.later = append(r.cyc.later, pm)
			r.cyc.laterImmediate = true
		}
		return
	}

	switch r.phase {
	case phaseNoticing:
		r.cyc.pending = append(r.cyc.pending, pm)
		return
	case phaseWaiting:
		r.cyc.pending = append(r.cyc.pending, pm)
		if bp.MarkRead {
			r.markReadFxLocked(fx, []*pendingMsg{&r.cyc.pending[len(r.cyc.pending)-1]})
		}
		r.moreLocked(fx, now)
		return
	case phaseThinking:
		r.cyc.pending = append(r.cyc.pending, pm)
		if bp.MarkRead {
			r.markReadFxLocked(fx, []*pendingMsg{&r.cyc.pending[len(r.cyc.pending)-1]})
		}
		r.cyc.dirty = true
		return
	case phaseTyping:
		r.cyc.later = append(r.cyc.later, pm)
		if bp.MarkRead {
			r.markReadFxLocked(fx, []*pendingMsg{&r.cyc.later[len(r.cyc.later)-1]})
		}
		return
	}

	// idle or queued
	open, next := eff.Available(now)
	if !open {
		snoozed := eff.Snoozed(now)
		if bp.Availability.OutsideHours == "silent" || next.IsZero() {
			reason, text := "outside_hours", "Outside active hours — not answering"
			if snoozed {
				reason, text = "snoozed", "Away — not answering"
			}
			r.actFx(fx, model.ActDecisionSkip, text, withMsg(map[string]any{"reason": reason}, pm.ID))
			return
		}
		if r.phase == phaseQueued {
			r.cyc.pending = append(r.cyc.pending, pm)
			return
		}
		r.startCycleLocked([]pendingMsg{pm}, false, false)
		r.queueLocked(fx, eff, now, next)
		return
	}
	if r.routineGateLocked(fx, eff, pm, now) { // off the phone (gym, sleep…): routine.go
		return
	}
	if r.phase == phaseQueued {
		r.cyc.pending = append(r.cyc.pending, pm) // answered when the catch-up delay ends
		return
	}
	if limited, meta, text := r.rateLimited(bp, now); limited {
		r.actFx(fx, model.ActDecisionSkip, text, withMsg(meta, pm.ID))
		return
	}
	if !forced && !r.e.rng.Hit(bp.ReplyPercent) {
		r.skipReplyLocked(fx, c, eff, p, pm)
		return
	}
	r.startCycleLocked([]pendingMsg{pm}, false, false)
	r.beginNoticeLocked(fx)
}

// rateLimited checks maxRepliesPerHour / maxRepliesPerDay (rolling windows).
func (r *Runner) rateLimited(bp model.BehaviorProfile, now time.Time) (bool, map[string]any, string) {
	if bp.MaxRepliesPerHour <= 0 && bp.MaxRepliesPerDay <= 0 {
		return false, nil, ""
	}
	st := r.e.store.RunnerState(r.key)
	if bp.MaxRepliesPerHour > 0 && st.RepliesSince(now.Add(-time.Hour)) >= bp.MaxRepliesPerHour {
		return true, map[string]any{"reason": "rate_limit", "limit": bp.MaxRepliesPerHour, "window": "hour"},
			fmt.Sprintf("Reply limit reached (%d per hour) — not answering", bp.MaxRepliesPerHour)
	}
	if bp.MaxRepliesPerDay > 0 && st.RepliesSince(now.Add(-24*time.Hour)) >= bp.MaxRepliesPerDay {
		return true, map[string]any{"reason": "rate_limit", "limit": bp.MaxRepliesPerDay, "window": "day"},
			fmt.Sprintf("Reply limit reached (%d per day) — not answering", bp.MaxRepliesPerDay)
	}
	return false, nil, ""
}

// skipReplyLocked handles a missed reply-chance roll: react, or leave on read.
func (r *Runner) skipReplyLocked(fx *effects, c model.ChatAssignment, eff behavior.Effective, p model.Persona, pm pendingMsg) {
	bp := eff.Profile
	meta := withMsg(map[string]any{"reason": "reply_chance", "percent": bp.ReplyPercent}, pm.ID)
	if pm.ID != "" && r.e.rng.Hit(bp.ReactPercent) {
		meta["react"] = true
		r.actFx(fx, model.ActDecisionSkip, "Decided to react instead of answering", meta)
		r.reactLaterLocked(fx, c, eff, p, pm)
		return
	}
	text := "Decided not to answer this one"
	if bp.MarkRead {
		text += " — left on read"
		r.afterLocked(fx, behavior.PlanNotice(bp, r.e.rng), func(fx *effects) {
			m := pm
			r.markReadFxLocked(fx, []*pendingMsg{&m})
		})
	}
	r.actFx(fx, model.ActDecisionSkip, text, meta)
}

// reactLaterLocked looks at the chat after the notice delay, then reacts to pm.
func (r *Runner) reactLaterLocked(fx *effects, c model.ChatAssignment, eff behavior.Effective, p model.Persona, pm pendingMsg) {
	if pm.ID == "" {
		return
	}
	bp := eff.Profile
	emoji, text, tone := r.e.reactionFor(p, pm.Name, pm.Text) // by meaning: expression.go
	r.afterLocked(fx, behavior.PlanNotice(bp, r.e.rng), func(fx *effects) {
		m := pm
		if m.ChatJID == "" {
			m.ChatJID = r.sendJIDOrDefaultLocked()
		}
		if bp.MarkRead {
			r.markReadFxLocked(fx, []*pendingMsg{&m})
		}
		e := r.e
		fx.add(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := e.wa.React(ctx, m.ChatJID, m.SenderJID, m.ID, emoji); err != nil {
				e.act(model.ActError, c, p.Name, "Couldn't react: "+err.Error(), nil)
				return
			}
			e.act(model.ActReacted, c, p.Name, text,
				map[string]any{"emoji": emoji, "messageId": m.ID, "tone": string(tone)})
		})
	})
}

// queueLocked parks the current cycle until the persona is available again.
func (r *Runner) queueLocked(fx *effects, eff behavior.Effective, now, next time.Time) {
	bp := eff.Profile
	wake := next.Add(r.e.rng.Between(0, bp.Availability.CatchUpMaxMin*60))
	r.phase = phaseQueued
	r.armLocked(wake.Sub(now), r.wakeLocked)
	key := r.key
	fx.add(func() { r.e.updateState(key, func(st *store.RunnerState) { w := wake; st.QueuedWakeAt = &w }) })
	reason, text := "outside_hours", "Outside active hours — will reply around "+fmtClock(wake, now, behavior.Location(bp.Availability))
	if eff.Snoozed(now) {
		reason = "snoozed"
		text = "Away until " + fmtClock(*eff.SnoozedUntil, now, time.Local) + " — will reply then"
	}
	r.actFx(fx, model.ActDeferred, text, map[string]any{"resumeAt": wake, "reason": reason})
}

// wakeLocked ends a queued wait: start noticing, or wait longer if still closed.
func (r *Runner) wakeLocked(fx *effects) {
	if r.cyc == nil || r.phase != phaseQueued {
		return
	}
	now := r.e.clock.Now()
	key := r.key
	fx.add(func() { r.e.updateState(key, func(st *store.RunnerState) { st.QueuedWakeAt = nil }) })
	eff := r.effLocked()
	bp := eff.Profile
	if open, next := eff.Available(now); !open {
		if next.IsZero() || bp.Availability.OutsideHours == "silent" {
			r.endCycleLocked(fx)
			return
		}
		r.queueLocked(fx, eff, now, next)
		return
	}
	if a, ok := r.unreachableLocked(now); ok { // still busy (routine.go)
		r.routineParkLocked(fx, eff, now, a)
		return
	}
	if limited, meta, text := r.rateLimited(bp, now); limited {
		r.actFx(fx, model.ActDecisionSkip, text, withMsg(meta, r.cyc.msgIDs()...))
		r.endCycleLocked(fx)
		return
	}
	if len(r.cyc.pending) == 0 && (len(r.history) == 0 || r.history[len(r.history)-1].Speaker != "them") {
		r.endCycleLocked(fx) // restored after a restart with nothing left to answer
		return
	}
	r.beginNoticeLocked(fx)
}

// restoreQueue re-arms a queued wake-up persisted in runtime.json.
func (r *Runner) restoreQueue() {
	st := r.e.store.RunnerState(r.key)
	if st.QueuedWakeAt == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.e.clock.Now()
	d := st.QueuedWakeAt.Sub(now)
	if d <= 0 {
		d = r.e.rng.Between(0, r.effLocked().Profile.Availability.CatchUpMaxMin*60)
	}
	r.startCycleLocked(nil, false, false)
	r.phase = phaseQueued
	r.armLocked(d, r.wakeLocked)
}

// cancelCycleLocked stands the persona down (you answered yourself).
func (r *Runner) cancelCycleLocked(fx *effects, c model.ChatAssignment) {
	if r.phase == phaseIdle || r.cyc == nil {
		return
	}
	wasQueued, ids := r.phase == phaseQueued, r.cyc.msgIDs()
	r.endCycleLocked(fx)
	r.actFx(fx, model.ActDecisionSkip, "You replied yourself — standing down", withMsg(map[string]any{"reason": "you_replied"}, ids...))
	if wasQueued {
		fx.add(func() { r.e.updateState(c.Key, func(st *store.RunnerState) { st.QueuedWakeAt = nil }) })
	}
}

// youReplied is called when you send a message from the app.
func (r *Runner) youReplied() {
	var fx effects
	r.mu.Lock()
	if r.effLocked().Profile.PauseWhenYouReply {
		r.cancelCycleLocked(&fx, r.cfg)
	}
	r.mu.Unlock()
	fx.run()
}

// ─── phases ──────────────────────────────────────────────────

// beginNoticeLocked: the persona hasn't looked at the chat yet.
func (r *Runner) beginNoticeLocked(fx *effects) {
	bp := r.effLocked().Profile
	r.phase = phaseNoticing
	d := behavior.PlanNotice(bp, r.e.rng)
	d, busy := r.slowNoticeLocked(d, r.e.clock.Now()) // slow routine block (routine.go)
	if d <= 0 {
		r.noticedLocked(fx)
		return
	}
	r.armLocked(d, r.noticedLocked)
	if busy != "" {
		r.actFx(fx, model.ActNoticing, "Busy ("+busy+") — slow to notice, will see it in "+fmtDur(d),
			map[string]any{"delaySeconds": d.Seconds(), "reason": "routine", "block": busy})
		return
	}
	r.actFx(fx, model.ActNoticing, "Hasn't looked at the chat yet — will see it in "+fmtDur(d),
		map[string]any{"delaySeconds": d.Seconds()})
}

// cooldownWait extends wait so the next reply respects cooldownSec.
func (r *Runner) cooldownWait(bp model.BehaviorProfile, now time.Time, wait time.Duration) (time.Duration, bool) {
	if bp.CooldownSec <= 0 {
		return wait, false
	}
	st := r.e.store.RunnerState(r.key)
	if st.LastReplyAt.IsZero() {
		return wait, false
	}
	if left := st.LastReplyAt.Add(time.Duration(bp.CooldownSec) * time.Second).Sub(now); left > wait {
		return left, true
	}
	return wait, false
}

// noticedLocked: the chat is opened (read receipts), then wait for more messages.
func (r *Runner) noticedLocked(fx *effects) {
	cyc := r.cyc
	if cyc == nil {
		return
	}
	bp := r.effLocked().Profile
	now := r.e.clock.Now()
	r.phase = phaseWaiting
	cyc.burstStart, cyc.resets = now, 0
	var unread []*pendingMsg
	for i := range cyc.pending {
		if !cyc.pending[i].read {
			unread = append(unread, &cyc.pending[i])
		}
	}
	if bp.MarkRead && !cyc.proactive {
		r.markReadFxLocked(fx, unread)
	}
	if n := len(cyc.pending); n > 0 {
		noun := "message"
		if n > 1 {
			noun = "messages"
		}
		if bp.MarkRead {
			r.actFx(fx, model.ActSeen, fmt.Sprintf("Seen (%d %s)", n, noun), map[string]any{"count": n, "readReceipts": true})
		} else {
			r.actFx(fx, model.ActSeen, fmt.Sprintf("Looked at the chat (%d %s) — read receipts off", n, noun), map[string]any{"count": n, "readReceipts": false})
		}
	}
	if cyc.immediate {
		r.thinkLocked(fx)
		return
	}
	wait, cooling := r.cooldownWait(bp, now, time.Duration(bp.WaitForMoreSec)*time.Second)
	if wait <= 0 {
		r.thinkLocked(fx)
		return
	}
	r.armLocked(wait, r.thinkLocked)
	if cooling {
		r.actFx(fx, model.ActWaiting, "Replied recently — waiting "+fmtDur(wait)+" before answering",
			map[string]any{"waitSeconds": wait.Seconds(), "resetCount": 0, "reason": "cooldown"})
		return
	}
	r.actFx(fx, model.ActWaiting, fmt.Sprintf("Waiting %s for more messages", fmtDur(wait)),
		map[string]any{"waitSeconds": wait.Seconds(), "resetCount": 0})
}

// moreLocked: another message while waiting → reset the debounce (burst cap).
func (r *Runner) moreLocked(fx *effects, now time.Time) {
	cyc := r.cyc
	bp := r.effLocked().Profile
	wait := time.Duration(bp.WaitForMoreSec) * time.Second
	if burstCap := time.Duration(bp.BurstCapSec) * time.Second; burstCap > 0 {
		left := burstCap - now.Sub(cyc.burstStart)
		if left <= 0 {
			r.actFx(fx, model.ActWaiting, "Burst cap reached — replying now", map[string]any{"reason": "burst_cap", "resetCount": cyc.resets})
			r.thinkLocked(fx)
			return
		}
		wait = min(wait, left)
	}
	wait, _ = r.cooldownWait(bp, now, wait)
	cyc.resets++
	if wait <= 0 {
		r.thinkLocked(fx)
		return
	}
	r.armLocked(wait, r.thinkLocked)
	r.actFx(fx, model.ActWaiting, fmt.Sprintf("More messages — waiting %s", fmtDur(wait)),
		map[string]any{"waitSeconds": wait.Seconds(), "resetCount": cyc.resets})
}

// thinkLocked starts generation and the think timer in parallel (the LLM's
// latency is absorbed by the think time).
func (r *Runner) thinkLocked(fx *effects) {
	cyc := r.cyc
	if cyc == nil {
		return
	}
	r.stopTimerLocked()
	c := r.cfg
	eff := r.effLocked()
	bp := eff.Profile
	r.phase = phaseThinking
	cyc.approval = c.ApprovalMode
	cyc.copilot = c.Mode == model.ChatModeCopilot
	cyc.thinkDone, cyc.genDone, cyc.draft, cyc.genErr, cyc.dirty = false, false, nil, nil, false
	cyc.drafted, cyc.genStart = len(cyc.pending), r.e.clock.Now()
	var think, distracted time.Duration
	if !cyc.immediate && !cyc.approval {
		think, distracted = behavior.PlanThink(bp, r.e.rng)
	}
	hist := pendingLast(r.historyCopyLocked(), cyc.pending)
	var answered pendingMsg
	if n := len(cyc.pending); n > 0 {
		answered = cyc.pending[n-1]
	}
	r.e.goBusy(func() { r.generate(cyc, c, eff, hist, answered) })
	if total := think + distracted; total > 0 {
		r.armLocked(total, r.thinkDoneLocked)
		r.actFx(fx, model.ActThinking, "Thinking for "+fmtDur(think), map[string]any{"delaySeconds": think.Seconds()})
		if distracted > 0 {
			r.actFx(fx, model.ActThinking, "Got distracted — back in "+fmtAbout(distracted),
				map[string]any{"delaySeconds": distracted.Seconds(), "distracted": true})
		}
		return
	}
	text := "Writing a reply…"
	if cyc.approval {
		text = "Writing a reply for your approval…"
		if cyc.copilot && !cyc.proactive {
			text = "Writing three ideas for you to pick from…"
		}
	}
	r.actFx(fx, model.ActGenerating, text, map[string]any{"messages": len(hist)})
	r.thinkDoneLocked(fx)
}

func (r *Runner) thinkDoneLocked(fx *effects) {
	cyc := r.cyc
	if cyc == nil {
		return
	}
	cyc.thinkDone = true
	cyc.thinkEnd = r.e.clock.Now()
	if !cyc.approval && !cyc.genDone && r.effLocked().Profile.TypingIndicator {
		// The text isn't ready yet: start "typing…" now so a slow model never
		// leaves a silent gap.
		cyc.typingShown = true
		fx.add(r.typingFxLocked(true))
	}
	r.maybeDeliverLocked(fx)
}

// generate runs the LLM for a cycle (own goroutine).
func (r *Runner) generate(cyc *cycle, c model.ChatAssignment, eff behavior.Effective, hist []model.Message, answered pendingMsg) {
	e := r.e
	var res genResult
	var dir *mention.Directory
	var tagged []mention.Entry
	var tk *taskUse
	p, ok := e.store.Persona(c.PersonaID)
	var err error
	if !ok {
		err = fmt.Errorf("persona %q not found", c.PersonaID)
	} else {
		ctx, cancel := context.WithTimeout(e.withProgress(cyc.ctx, c, p.Name), generateLimit)
		progress(ctx, fmt.Sprintf("Reading the chat (%d recent)", len(hist)), map[string]any{"stage": "read", "messages": len(hist)})
		isGroup := c.Kind == "group"
		dir = e.directory(ctx, c, hist)
		cp := e.paramsOf(c) // the chat's context system (ctxsys.go)
		opts, cross := e.promptOptions(c, eff.Profile, dir, hist)
		ph := e.shapePrompt(&opts, cp, p, hist)
		if cross.active() {
			e.act(model.ActThinking, c, p.Name, crossActivityText(cross), map[string]any{"stage": "cross", "crossContext": cross.meta()})
		}
		if cyc.proactive {
			opts.Opener = e.openerFor(cyc.opener, hist) // opener.go
		} else {
			opts.Late = e.lateNote(p, hist, e.clock.Now()) // "sorry, was at the gym" (routine.go)
		}
		tk = e.taskTurn(ctx, c, p, hist, cyc.proactive, e.replyLanguage(c, hist)) // a running task is the goal (task.go)
		copilot := cyc.copilot && tk == nil                                       // a task writes one exact message
		if tk != nil {
			opts.Task = tk.prompt
		}
		gcfg, gturn := e.chatGoalTurn(ctx, c, p, hist, isGroup, cyc.proactive)
		build := func(t prompt.GoalTurn, x prompt.CrossTurn) llm.Request {
			opts.Goal, opts.CrossTurn = t, x
			if cyc.proactive {
				return prompt.Initiate(p, c, ph, isGroup, opts)
			}
			return prompt.Compose(p, c, ph, isGroup, opts)
		}
		res, err = e.replyFor(ctx, copilot, p, eff.Profile, ph, cyc.proactive, gcfg, gturn, cross, build)
		if err == nil && !copilot && (cp.ReplyCheck != model.CheckOff || opts.LanguageFixed) { // replycheck.go
			o := opts
			o.Goal = gturn
			ck := replyCheck{cp: cp, persona: p, plan: gturn.Plan, lang: opts.ReplyLanguage, langFixed: opts.LanguageFixed, hist: hist,
				rules: checkRules(prompt.ComposeSections(p, c, ph, isGroup, o), p)}
			res = e.checkedReply(ctx, ck, res, e.noteRetry(ctx, p, eff.Profile, ph, cyc.proactive, gcfg, gturn, cross, build))
		}
		if err == nil && tk != nil { // the task's leak check (task.go)
			res = e.taskGuard(c, tk, res, e.noteRetry(ctx, p, eff.Profile, ph, cyc.proactive, gcfg, gturn, cross, build))
		}
		cancel()
		res.Opener = cyc.proactive
		if err == nil {
			tagged = e.applyMentionsAll(&res, dir, eff.Profile, answered, hist, cyc.proactive) // every co-pilot draft too
		}
	}
	var fx effects
	r.mu.Lock()
	if r.stopped || r.cyc != cyc {
		r.mu.Unlock()
		e.taskDropped(tk) // the cycle is gone: its task turn never goes out
		return
	}
	cyc.genDone, cyc.persona, cyc.hist, cyc.task = true, p, hist, tk
	cyc.dir, cyc.mentions = dir, mention.Names(tagged)
	if err != nil {
		cyc.genErr = err
	} else {
		cyc.draft = &res
	}
	r.maybeDeliverLocked(&fx)
	r.mu.Unlock()
	fx.run()
}

// taskFxLocked hands a cycle's task turn on (task.go): delivered = sent or
// waiting in Approvals (its effects apply), else it is undone.
func (r *Runner) taskFxLocked(fx *effects, cyc *cycle, delivered bool) {
	u := cyc.task
	if u == nil {
		return
	}
	cyc.task = nil
	e, c := r.e, r.cfg
	if delivered {
		fx.add(func() { e.taskDelivered(c, u) })
	} else {
		fx.add(func() { e.taskDropped(u) })
	}
}

// pendingLast moves the messages being answered after a reply that went out
// while they waited: they were sent before it landed, but they are the turn
// the persona answers now.
func pendingLast(hist []model.Message, pending []pendingMsg) []model.Message {
	n := len(hist)
	if n == 0 || hist[n-1].Speaker != "me" || !hist[n-1].FromBot {
		return hist
	}
	waiting := func(m model.Message) bool {
		return m.ID != "" && slices.ContainsFunc(pending, func(p pendingMsg) bool { return p.ID == m.ID })
	}
	if !slices.ContainsFunc(hist, waiting) {
		return hist
	}
	out := make([]model.Message, 0, n)
	var moved []model.Message
	for _, m := range hist {
		if waiting(m) {
			moved = append(moved, m)
		} else {
			out = append(out, m)
		}
	}
	return append(out, moved...)
}

// A chat that keeps talking must not starve a reply: on a slow model every
// finished draft would meet a newer message and be thrown away, forever. A
// draft is only redone when redoing it is cheap, and only so many times.
const (
	draftRedoWindow = 20 * time.Second
	maxDraftRedos   = 2
)

// keepDraftLocked reports whether a draft that newer messages arrived behind
// is sent anyway. Approvals, failed and quiet drafts are redone: those paths
// end the cycle, so nothing would answer the newer messages.
func (r *Runner) keepDraftLocked(cyc *cycle) bool {
	if cyc.approval || cyc.genErr != nil || cyc.draft == nil || cyc.draft.Quiet {
		return false
	}
	return cyc.restarts >= maxDraftRedos || r.e.clock.Now().Sub(cyc.genStart) >= draftRedoWindow
}

// maybeDeliverLocked acts once both the think time and the generation are done.
func (r *Runner) maybeDeliverLocked(fx *effects) {
	cyc := r.cyc
	if cyc == nil || !cyc.genDone || !cyc.thinkDone {
		return
	}
	c := r.cfg
	eff := r.effLocked()
	bp := eff.Profile
	if cyc.dirty && r.keepDraftLocked(cyc) {
		// Too slow to redo: send it and answer the newer messages next.
		cyc.later = append(cyc.later, cyc.pending[cyc.drafted:]...)
		cyc.laterImmediate = cyc.immediate
		cyc.pending, cyc.dirty = cyc.pending[:cyc.drafted], false
		r.actFx(fx, model.ActProgress, "New messages came in. Sending this reply first, then answering them", map[string]any{"stage": "keep"})
	}
	if cyc.dirty {
		// New messages arrived while thinking: drop the draft and think again.
		cyc.restarts++
		r.taskFxLocked(fx, cyc, false)
		cyc.typingShown = false
		fx.add(r.typingFxLocked(false))
		r.phase = phaseWaiting
		wait := min(time.Duration(bp.WaitForMoreSec)*time.Second, 3*time.Second)
		if cyc.immediate {
			wait = 0
		}
		r.actFx(fx, model.ActWaiting, "New message while thinking — starting over",
			map[string]any{"reason": "new_messages", "waitSeconds": wait.Seconds()})
		if wait <= 0 {
			r.thinkLocked(fx)
		} else {
			r.armLocked(wait, r.thinkLocked)
		}
		return
	}
	if cyc.genErr != nil {
		if cyc.ctx.Err() == nil {
			r.actFx(fx, model.ActError, "Couldn't generate a reply: "+cyc.genErr.Error(), withMsg(nil, cyc.msgIDs()...))
		}
		r.endCycleLocked(fx)
		return
	}
	if !c.Enabled {
		r.endCycleLocked(fx)
		return
	}
	if d := cyc.draft; d.Quiet && !cyc.approval {
		r.actFx(fx, model.ActDecisionSkip, "Staying quiet — the reply didn't pass the check: "+strings.Join(slices.Compact(slices.Clone(d.Checks)), "; "),
			withMsg(map[string]any{"reason": "reply_check", "checks": d.Checks}, cyc.msgIDs()...))
		r.endCycleLocked(fx)
		return
	}
	res, p, hist := *cyc.draft, cyc.persona, cyc.hist
	if cyc.approval {
		r.taskFxLocked(fx, cyc, true) // it waits in Approvals: the task moves on
		e := r.e
		mentions, ids, now := cyc.mentions, cyc.msgIDs(), r.e.clock.Now()
		fx.add(func() {
			e.queueApproval(r, c, p, res, hist, mentions)
			if len(ids) > 0 {
				e.logDecided(c.Key, ids, model.DecisionApproval, "approval", "Reply written — waiting for your approval", now)
			}
		})
		r.endCycleLocked(fx)
		return
	}
	var cand *model.QuoteRef
	if n := len(cyc.pending); n > 0 && !cyc.proactive {
		if last := cyc.pending[n-1]; last.ID != "" {
			cand = &model.QuoteRef{MessageID: last.ID, SenderJID: last.SenderJID, Text: last.Text}
		}
	}
	quote := behavior.PickQuote(eff, r.e.rng, behavior.PlanInput{Group: eff.Kind == behavior.KindGroup, Immediate: cyc.immediate, Quote: cand})
	bubbles := behavior.PlanBubbles(bp, r.e.rng, res.Text, quote, cyc.immediate)
	var already time.Duration
	if cyc.typingShown {
		already = r.e.clock.Now().Sub(cyc.thinkEnd)
	}
	r.phase = phaseTyping
	r.stopTimerLocked()
	targets := r.targetsLocked()
	typingOn := bp.TypingIndicator
	dir := cyc.dir
	r.e.goBusy(func() { r.send(cyc, c, p, res, bubbles, already, targets, typingOn, dir) })
}

// send delivers the bubbles of a reply: typing, message, gap, … (own goroutine).
// dir encodes "@Name" tags (nil = none).
func (r *Runner) send(cyc *cycle, c model.ChatAssignment, p model.Persona, res genResult, bubbles []behavior.Bubble, already time.Duration, targets []string, typingOn bool, dir *mention.Directory) {
	e := r.e
	n, sent := len(bubbles), 0
	typoAt, slip := r.planTypo(cyc, c, bubbles) // typo.go
	for i, b := range bubbles {
		if i > 0 && !e.sleep(cyc.ctx, b.GapBefore) {
			r.sendAborted(cyc, nil, sent)
			return
		}
		wait := b.Typing
		if i == 0 {
			wait = max(0, wait-already)
		}
		if typingOn {
			r.setTyping(cyc, true)
		}
		if b.Typing > 0 {
			e.act(model.ActTyping, c, p.Name, typingText(b, i, n), map[string]any{
				"typingSeconds": b.Typing.Seconds(), "chars": utf8.RuneCountInString(b.Text), "part": i + 1, "parts": n,
			})
		}
		if !r.typeFor(cyc, wait, typingOn) {
			r.sendAborted(cyc, nil, sent)
			return
		}
		text := b.Text
		if i == typoAt {
			text = slip.Typed
		}
		jid, wire, sres, err := e.sendTextResult(cyc.ctx, targets, text, b.Quote, dir)
		if typingOn {
			r.setTyping(nil, false)
		}
		if err != nil {
			r.sendAborted(cyc, err, sent)
			return
		}
		tags := bubbleMentions(b.Text, dir)
		msgID := e.recordSentBubble(r, c, text, b.Text, wire, tags, sres.ID, "", res.crossUsed())
		m := mentionMeta(res.meta(), tags)
		if i == typoAt {
			m["typo"] = true
		}
		m["jid"], m["part"], m["parts"], m["typingSeconds"] = jid, i+1, n, b.Typing.Seconds()
		if b.Quote != nil {
			m["quoted"] = true
		}
		if cyc.proactive {
			m["proactive"] = true
		}
		e.act(model.ActSent, c, p.Name, text, withMsg(m, cyc.msgIDs()...))
		sent++
		cyc.sent.Add(1)
		if i == typoAt && !r.fixTypo(cyc, c, p, slip, jid, sres.ID, msgID, b.Text, dir, typingOn) {
			r.sendAborted(cyc, nil, sent)
			return
		}
	}
	r.finishSend(cyc, sent)
}

func typingText(b behavior.Bubble, i, n int) string {
	chars := utf8.RuneCountInString(b.Text)
	if n == 1 {
		return fmt.Sprintf("Typing for %s (%d characters)", fmtDur(b.Typing), chars)
	}
	return fmt.Sprintf("Typing for %s (%d characters, part %d of %d)", fmtDur(b.Typing), chars, i+1, n)
}

// typeFor waits d while typing, refreshing the indicator every 8 s.
func (r *Runner) typeFor(cyc *cycle, d time.Duration, typingOn bool) bool {
	const refresh = 8 * time.Second
	for d > 0 {
		step := min(d, refresh)
		if !r.e.sleep(cyc.ctx, step) {
			return false
		}
		d -= step
		if d > 0 && typingOn {
			r.setTyping(cyc, true)
		}
	}
	return cyc.ctx.Err() == nil
}

func (r *Runner) sendAborted(cyc *cycle, err error, sent int) {
	var fx effects
	r.mu.Lock()
	if r.cyc == cyc && !r.stopped {
		r.taskFxLocked(&fx, cyc, sent > 0) // part of it went out: the task moves on
		if err != nil && cyc.ctx.Err() == nil {
			r.actFx(&fx, model.ActError, "Send failed: "+err.Error(), withMsg(nil, cyc.msgIDs()...))
		}
		r.endCycleLocked(&fx)
	}
	r.mu.Unlock()
	fx.run()
	if sent > 0 {
		r.recordReply(cyc.proactive, cyc.proactive && !cyc.scheduled)
	}
}

// finishSend ends the cycle; messages that arrived while typing start the next one.
func (r *Runner) finishSend(cyc *cycle, sent int) {
	if sent > 0 {
		r.recordReply(cyc.proactive, cyc.proactive && !cyc.scheduled)
	}
	var fx effects
	r.mu.Lock()
	if r.cyc == cyc && !r.stopped {
		r.taskFxLocked(&fx, cyc, sent > 0)
		later, imm := cyc.later, cyc.laterImmediate
		r.endCycleLocked(&fx)
		if len(later) > 0 {
			// They wrote while we were typing: we're in the chat, no notice delay.
			r.startCycleLocked(later, imm, false)
			r.noticedLocked(&fx)
		}
	}
	r.mu.Unlock()
	fx.run()
}

// recordReply counts a delivered reply for rate limits, cooldown and silence
// tracking; checkIn also counts it against the check-in limit.
func (r *Runner) recordReply(proactive, checkIn bool) {
	now := r.e.clock.Now()
	if !proactive {
		r.mu.Lock()
		r.noteReplyLocked(now)
		r.mu.Unlock()
	}
	r.e.updateState(r.key, func(st *store.RunnerState) {
		st.LastReplyAt = now
		st.Replies = append(st.Replies, now)
		if checkIn {
			st.Proactive = append(st.Proactive, now)
		}
	})
}
