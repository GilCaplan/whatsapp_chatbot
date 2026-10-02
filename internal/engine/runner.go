package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/guard"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

const (
	inboxSize     = 64
	generateLimit = 120 * time.Second
	echoWindow    = 2 * time.Minute
	contextSize   = 6 // messages shown with a pending approval
)

// Runner owns one chat: its history and the reply cycle state machine
// (pipeline.go). Incoming messages are processed in order by a single worker
// goroutine; generation and delivery run in their own goroutines so new
// messages keep flowing. Phase transitions happen under mu; side effects
// (network, activity) are collected and run after unlocking.
type Runner struct {
	e      *Engine
	key    string
	inbox  chan model.Incoming
	queued atomic.Int64

	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	cfg       model.ChatAssignment
	history   []model.Message
	sendJID   string // address the last incoming message used
	pendingID string
	echoes    []echo // recently sent texts, to ignore our own messages echoed back
	stopped   bool
	streak    streakState // replies in a row to one person (people.go)
	// handoffNoted: when the "paused, waiting for you" skip was last logged (handoff.go).
	handoffNoted time.Time

	phase    phase
	cyc      *cycle
	cycleSeq uint64
	timer    Timer
	timerGen uint64 // invalidates stale timer callbacks
	aux      map[uint64]Timer
	auxSeq   uint64

	typSeq     uint64 // order of typing on/off decisions
	typMu      sync.Mutex
	typApplied uint64
	typJID     string
	typOn      bool
}

type echo struct {
	text string
	at   time.Time
}

func newRunner(e *Engine, c model.ChatAssignment) *Runner {
	r := &Runner{e: e, key: c.Key, cfg: c, inbox: make(chan model.Incoming, inboxSize), aux: map[uint64]Timer{}}
	r.ctx, r.cancel = context.WithCancel(e.ctx)
	rp := e.effective(c).Profile
	if h, err := e.store.History(c.Key, rp.HistoryMessages); err == nil {
		r.history = trimHistory(h, rp.HistoryMessages, rp.HistoryChars)
	}
	var latest model.PendingReply
	for _, p := range e.store.Approvals() {
		if p.ChatKey == c.Key && (latest.ID == "" || p.CreatedAt.After(latest.CreatedAt)) {
			latest = p
		}
	}
	r.pendingID = latest.ID
	r.restoreQueue()
	go r.loop()
	return r
}

func (r *Runner) loop() {
	for {
		select {
		case <-r.ctx.Done():
			return
		case in := <-r.inbox:
			r.process(in)
			r.queued.Add(-1)
		}
	}
}

func (r *Runner) enqueue(in model.Incoming) {
	if r.ctx.Err() != nil {
		return
	}
	r.queued.Add(1)
	select {
	case r.inbox <- in:
	default:
		r.queued.Add(-1)
		r.e.act(model.ActError, r.config(), "", "Too many queued messages; dropped one", nil)
	}
}

// stop ends the runner: every timer is stopped and in-flight work is cancelled.
func (r *Runner) stop() {
	r.mu.Lock()
	r.stopped = true
	r.stopTimerLocked()
	for id, t := range r.aux {
		t.Stop()
		delete(r.aux, id)
	}
	if r.cyc != nil {
		r.cyc.cancel()
		r.cyc = nil
	}
	r.phase = phaseIdle
	off := r.typingFxLocked(false)
	r.mu.Unlock()
	r.cancel()
	off()
}

func (r *Runner) setConfig(c model.ChatAssignment) {
	r.mu.Lock()
	r.cfg = c
	r.mu.Unlock()
}

func (r *Runner) config() model.ChatAssignment {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg
}

func (r *Runner) preferredJID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sendJID
}

func (r *Runner) historySnapshot() []model.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.history)
}

func (r *Runner) setPending(id string) {
	r.mu.Lock()
	r.pendingID = id
	r.mu.Unlock()
}

func (r *Runner) clearPending(id string) {
	r.mu.Lock()
	if r.pendingID == id {
		r.pendingID = ""
	}
	r.mu.Unlock()
}

func (r *Runner) active() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.stopped && r.cfg.Enabled
}

// effLocked resolves the chat's current behaviour.
func (r *Runner) effLocked() behavior.Effective { return r.e.effective(r.cfg) }

// process handles one incoming message (worker goroutine).
func (r *Runner) process(in model.Incoming) {
	e := r.e
	settings := e.cfg.Get()
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return
	}
	now := e.clock.Now()
	var fx effects
	defer fx.run()

	// Group members (cached by the wa layer): tag names, the size rule and
	// who is picked. Incoming "@<number>" tags become "@Name" for history,
	// activity and the LLM; decisions use the raw text.
	var roster []model.Participant
	if in.IsGroup {
		roster = e.roster(r.ctx, r.config())
	}
	raw := text
	display, mentionNames := text, []string(nil)
	if in.IsGroup && len(in.MentionedJIDs) > 0 {
		selfName := ""
		if sp, ok := e.store.Persona(r.config().PersonaID); ok {
			selfName = sp.Name
		}
		display, mentionNames = e.humanizeIncoming(roster, r.historySnapshot(), in, text, selfName)
	}

	r.mu.Lock()
	if r.stopped || !r.cfg.Enabled {
		r.mu.Unlock()
		return
	}
	c := r.cfg
	eff := e.withPersonaZone(behavior.Resolve(settings.Behavior.For(behavior.KindOf(c)), c), c) // "persona" zone: routine.go
	bp := eff.Profile
	if in.ChatJID != "" {
		r.sendJID = in.ChatJID
	}
	speaker, immediate := "them", false
	if in.IsFromMe {
		if r.isEchoLocked(raw) {
			r.mu.Unlock()
			return
		}
		r.noteSpeakerLocked("", now) // you spoke: no bot back-and-forth
		body, ok := stripTrigger(display, settings.Behavior.TriggerPrefix)
		if !ok {
			// Your own normal message: keep it as context, don't reply, and
			// let the persona stand down if it was about to answer.
			me := newMessage(in, "me", display, now)
			me.Mentions = mentionNames
			msg := r.appendLocked(me, bp)
			if bp.PauseWhenYouReply {
				r.cancelCycleLocked(&fx, c)
			}
			r.mu.Unlock()
			r.persist(msg, bp)
			e.updateState(c.Key, func(st *store.RunnerState) { st.LastYouRepliedAt = now })
			return
		}
		if body == "" {
			r.mu.Unlock()
			return
		}
		text, speaker, immediate = body, "me", true
	} else {
		text = display
		r.noteSpeakerLocked(streakKey(c, in), now)
	}
	r.mu.Unlock()

	// Messages delivered late (e.g. backlog after being offline) become context only:
	// replying to something sent long ago looks robotic.
	staleAfter := time.Duration(bp.StaleAfterMin) * time.Minute
	if !in.Timestamp.IsZero() && e.clock.Since(in.Timestamp) > staleAfter {
		if speaker == "them" {
			r.keepAsContext(in, text, mentionNames, bp, now)
		}
		e.act(model.ActDecisionSkip, c, "", "Old message from while offline — kept as context, not answered",
			map[string]any{"reason": "stale", "sentAt": in.Timestamp})
		return
	}

	isGroup := c.Kind == "group" || in.IsGroup
	p, ok := e.store.Persona(c.PersonaID)
	if !ok {
		e.act(model.ActError, c, "", fmt.Sprintf("Persona %q not found — assign another persona", c.PersonaID), nil)
		return
	}
	meta := map[string]any{"fromMe": in.IsFromMe, "group": isGroup}
	if in.PushName != "" && !in.IsFromMe {
		meta["sender"] = in.PushName
	}
	if immediate {
		meta["trigger"] = true
	}
	meta = mentionMeta(meta, mentionNames)
	e.act(model.ActIncoming, c, p.Name, text, meta)
	if speaker == "them" {
		e.goalIncoming(c, p, isGroup, senderName(in, c, isGroup), text, in.Media) // "say the word" goals (goal.go)
		// Hand-off (handoff.go): a paused chat keeps messages as context; a
		// sensitive message pauses it — even when the persona would stay quiet.
		if r.handoffGate(c, p, in, text, mentionNames, bp, isGroup, now) {
			return
		}
	}

	if bp.IgnoreLinks && hasLink(text) {
		e.act(model.ActDecisionSkip, c, p.Name, "Ignored a message with a link", map[string]any{"reason": "link"})
		return
	}

	pm := pendingMsg{ID: in.MessageID, SenderJID: in.SenderJID, ChatJID: in.ChatJID, Name: in.PushName, Text: text}
	forced := false
	if speaker == "them" {
		// Who it answers (people.go): mute words, picked people, priority
		// people, trigger words, the streak guard.
		switch gt := r.peopleGate(c, bp, p, in, raw, isGroup, roster, now); gt.action {
		case gateSkip:
			e.act(model.ActDecisionSkip, c, p.Name, gt.text, gt.meta)
			r.keepAsContext(in, text, mentionNames, bp, now)
			return
		case gateForce:
			e.act(model.ActDecisionReply, c, p.Name, gt.text, gt.meta)
			forced = true
		}
	}
	if isGroup && speaker == "them" && !forced {
		d := e.decide(r.ctx, p, bp, raw, in.MentionedJIDs)
		if !d.Reply {
			e.act(model.ActDecisionSkip, c, p.Name, decisionText(d.Reason, p.Name), d.Meta)
			if open, _ := eff.Available(now); open && e.rng.Hit(bp.ReactPercent) {
				r.mu.Lock()
				if !r.stopped {
					r.reactLaterLocked(&fx, c, eff, p, pm)
				}
				r.mu.Unlock()
			}
			return
		}
		e.act(model.ActDecisionReply, c, p.Name, decisionText(d.Reason, p.Name), d.Meta)
	}

	g := guard.Inspect(text, bp.InjectionFilter)
	if g.Blocked {
		e.act(model.ActBlockedInjection, c, p.Name, "Ignored a likely prompt-injection attempt: "+guard.Truncate(text, 200),
			map[string]any{"score": g.Score, "matches": g.Matches, "level": bp.InjectionFilter})
		return
	}
	if g.Text == "" {
		return
	}
	pm.Text = g.Text

	msg := newMessage(in, speaker, g.Text, now)
	msg.Mentions = mentionNames
	r.mu.Lock()
	if r.stopped || !r.cfg.Enabled {
		r.mu.Unlock()
		return
	}
	if pm.ChatJID == "" {
		pm.ChatJID = r.sendJIDOrDefaultLocked()
	}
	msg = r.appendLocked(msg, bp)
	stale := r.pendingID
	r.admitLocked(&fx, c, eff, p, pm, immediate, forced, now)
	r.mu.Unlock()

	r.persist(msg, bp)
	e.store.TouchChat(c.Key, time.Now())
	if speaker == "them" {
		e.updateState(c.Key, func(st *store.RunnerState) { st.LastIncomingAt = now })
		e.memoryTick(c) // learn about people every few messages (memory.go)
	}
	if stale != "" {
		e.markStale(stale)
	}
}

func (r *Runner) sendJIDOrDefaultLocked() string {
	if r.sendJID != "" {
		return r.sendJID
	}
	if r.cfg.JID != "" {
		return r.cfg.JID
	}
	return r.cfg.AltJID
}

// targetsLocked lists send addresses, the last-used one first.
func (r *Runner) targetsLocked() []string {
	var out []string
	for _, j := range []string{r.sendJID, r.cfg.JID, r.cfg.AltJID} {
		if j != "" && !containsStr(out, j) {
			out = append(out, j)
		}
	}
	return out
}

// recordSent appends a persona message to history and remembers the text as
// sent (wire, "" = msg.Text) to recognise its echo.
func (r *Runner) recordSent(msg model.Message, wire string) {
	if wire == "" {
		wire = msg.Text
	}
	r.mu.Lock()
	rp := r.effLocked().Profile
	msg = r.appendLocked(msg, rp)
	r.echoes = append(r.echoes, echo{text: strings.TrimSpace(wire), at: time.Now()})
	r.mu.Unlock()
	r.persist(msg, rp)
}

func (r *Runner) appendLocked(m model.Message, rp model.BehaviorProfile) model.Message {
	if m.ID == "" {
		m.ID = store.NewID()
	}
	r.history = trimHistory(append(r.history, m), rp.HistoryMessages, rp.HistoryChars)
	return m
}

func (r *Runner) persist(m model.Message, rp model.BehaviorProfile) {
	if err := r.e.store.AppendHistory(r.key, m, rp.HistoryMessages); err != nil {
		r.e.act(model.ActError, r.config(), "", "Couldn't save history: "+err.Error(), nil)
	}
}

// isEchoLocked reports (and forgets) a recently sent text coming back as "from me".
func (r *Runner) isEchoLocked(text string) bool {
	now := time.Now()
	r.echoes = slices.DeleteFunc(r.echoes, func(x echo) bool { return now.Sub(x.at) > echoWindow })
	for i, x := range r.echoes {
		if x.text == text {
			r.echoes = slices.Delete(r.echoes, i, i+1)
			return true
		}
	}
	return false
}

// ─── helpers ─────────────────────────────────────────────────

func newMessage(in model.Incoming, speaker, text string, now time.Time) model.Message {
	m := model.Message{ID: in.MessageID, TS: in.Timestamp, Speaker: speaker, Text: text}
	if m.TS.IsZero() {
		m.TS = now
	}
	if speaker == "them" {
		m.Name = in.PushName
		m.SenderJID = in.SenderJID
	}
	return m
}

// trimHistory drops the oldest messages beyond maxMsgs or maxChars (keeps at least one).
func trimHistory(h []model.Message, maxMsgs, maxChars int) []model.Message {
	if maxMsgs > 0 && len(h) > maxMsgs {
		h = h[len(h)-maxMsgs:]
	}
	if maxChars > 0 {
		total := 0
		for i := len(h) - 1; i >= 0; i-- {
			total += utf8.RuneCountInString(h[i].Text)
			if total > maxChars && i < len(h)-1 {
				h = h[i+1:]
				break
			}
		}
	}
	return slices.Clip(h)
}

// stripTrigger returns the text after the trigger prefix when text starts with
// prefix followed by whitespace (or is exactly the prefix).
func stripTrigger(text, prefix string) (string, bool) {
	if prefix == "" || !strings.HasPrefix(text, prefix) {
		return "", false
	}
	rest := text[len(prefix):]
	if rest == "" {
		return "", true
	}
	if r, _ := utf8.DecodeRuneInString(rest); !unicode.IsSpace(r) {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

func hasLink(text string) bool {
	l := strings.ToLower(text)
	return strings.Contains(l, "http://") || strings.Contains(l, "https://") || strings.Contains(l, "www.") || strings.Contains(l, "wa.me/")
}

// fmtDur renders a duration for activity texts: "8s", "2.5s", "3 minutes", "2 hours".
func fmtDur(d time.Duration) string {
	switch {
	case d < time.Minute:
		if d%time.Second == 0 {
			return fmt.Sprintf("%ds", int(d/time.Second))
		}
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < 2*time.Minute:
		return "1 minute"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Round(time.Minute)/time.Minute))
	case d < 2*time.Hour:
		return "1 hour"
	case d < 48*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Round(time.Hour)/time.Hour))
	}
	return fmt.Sprintf("%d days", int(d/(24*time.Hour)))
}

// fmtAbout is fmtDur for rough estimates ("about 2 minutes").
func fmtAbout(d time.Duration) string {
	if d < time.Minute {
		return fmtDur(d.Round(time.Second))
	}
	return "about " + fmtDur(d)
}

// fmtClock renders t as "08:30" when it is within a day of now, else "Mon 08:30".
func fmtClock(t, now time.Time, loc *time.Location) string {
	t = t.In(loc)
	if t.Sub(now) < 20*time.Hour {
		return t.Format("15:04")
	}
	return t.Format("Mon 15:04")
}

// ─── deferred side effects ───────────────────────────────────

// effects are side effects collected under Runner.mu and run after unlocking,
// in order, on the same goroutine.
type effects []func()

func (f *effects) add(fn func()) { *f = append(*f, fn) }

func (f *effects) run() {
	for _, fn := range *f {
		fn()
	}
	*f = nil
}
