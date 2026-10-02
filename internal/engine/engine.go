// Package engine runs every assigned chat concurrently: it routes incoming
// WhatsApp messages to per-chat runners, decides whether to reply, generates
// replies with the persona's LLM and delivers them like a person would
// (notice → seen → wait → think → typing → bubbles), or queues them for approval.
package engine

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/contract"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/guard"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/mention"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
	"whatsappdoppel/internal/store"
)

var (
	// ErrNotFound is store.ErrNotFound (chats, personas, approvals, sessions).
	ErrNotFound = store.ErrNotFound
	// ErrChatDisabled means the chat exists but has no running runner.
	ErrChatDisabled = errors.New("chat is disabled")
	// ErrBusy means the approval is already being sent.
	ErrBusy = errors.New("this reply is already being sent")
	// ErrNoSuchDraft means SendApproved was given a draft index the pending
	// reply doesn't have.
	ErrNoSuchDraft = errors.New("no such draft")
	// ErrNotImplemented is contract.ErrNotImplemented (wave 3 stubs).
	ErrNotImplemented = contract.ErrNotImplemented
)

var _ contract.Engine = (*Engine)(nil)

// defaultProactiveEvery is how often idle chats are checked for check-ins.
const defaultProactiveEvery = 5 * time.Minute

// Deps are the engine's collaborators. Clock nil means the real clock.
type Deps struct {
	Config *config.Manager
	Store  *store.Store
	Hub    *events.Hub
	WA     contract.WhatsApp
	LLM    *llm.Registry
	Clock  Clock
	// Rand seeds all behaviour randomness (nil = time-seeded). Sampler, when
	// set, replaces it entirely (tests use behavior.Fixed).
	Rand    rand.Source
	Sampler behavior.Sampler
	// ProactiveEvery is the check-in scan interval (0 = 5 minutes).
	ProactiveEvery time.Duration
}

type Engine struct {
	cfg   *config.Manager
	store *store.Store
	hub   *events.Hub
	wa    contract.WhatsApp
	llm   *llm.Registry
	clock Clock
	rng   behavior.Sampler

	ctx    context.Context
	cancel context.CancelFunc

	// busy counts goroutines doing work (generation, delivery) that are not
	// parked on a clock timer; tests wait for it to reach zero.
	busy atomic.Int64

	mu      sync.RWMutex
	runners map[string]*Runner
	stopped bool

	proMu          sync.Mutex
	proTimer       Timer
	proactiveEvery time.Duration

	apMu       sync.Mutex
	autoTimers map[string]Timer
	sending    map[string]bool

	pg *playground

	recap recapSched // daily recap timer (recap.go)

	jobs jobRunner // background jobs: memory extraction (jobs.go)
}

// New builds the engine, starts runners for enabled chats and re-arms
// auto-send timers of persisted approvals.
func New(d Deps) *Engine {
	clk := d.Clock
	if clk == nil {
		clk = realClock{}
	}
	rng := d.Sampler
	if rng == nil {
		rng = behavior.NewSampler(d.Rand)
	}
	e := &Engine{
		cfg:            d.Config,
		store:          d.Store,
		hub:            d.Hub,
		wa:             d.WA,
		llm:            d.LLM,
		clock:          clk,
		rng:            rng,
		runners:        map[string]*Runner{},
		autoTimers:     map[string]Timer{},
		sending:        map[string]bool{},
		proactiveEvery: d.ProactiveEvery,
	}
	if e.proactiveEvery <= 0 {
		e.proactiveEvery = defaultProactiveEvery
	}
	e.ctx, e.cancel = context.WithCancel(context.Background())
	e.pg = newPlayground(e)
	e.Reload()
	for _, p := range e.store.Approvals() {
		e.armAutoSend(p)
	}
	e.scheduleRecap()
	if e.cfg != nil {
		e.cfg.OnChange(e.settingsChanged) // v4 settings (recap time) apply without a restart
	}
	return e
}

// Playground returns the in-memory persona playground.
func (e *Engine) Playground() contract.Playground { return e.pg }

// Builder returns the AI persona builder.
func (e *Engine) Builder() contract.Builder { return builder{e} }

// Reload diffs chats.json against the running runners: removed/disabled chats
// stop, new ones start, changed ones get the new config (history is kept).
// In-flight reply cycles keep their plan; new messages use the new behaviour.
func (e *Engine) Reload() {
	want := map[string]model.ChatAssignment{}
	for _, c := range e.store.Chats() {
		if c.Enabled {
			want[c.Key] = c
		}
	}
	var stop []*Runner
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		return
	}
	for key, r := range e.runners {
		if c, ok := want[key]; ok {
			r.setConfig(c)
			delete(want, key)
		} else {
			stop = append(stop, r)
			delete(e.runners, key)
		}
	}
	for key, c := range want {
		e.runners[key] = newRunner(e, c)
	}
	e.mu.Unlock()
	for _, r := range stop {
		r.stop()
	}
	e.syncProactive()
}

// Stop cancels all runners, in-flight generations and every timer.
func (e *Engine) Stop() {
	e.mu.Lock()
	e.stopped = true
	runners := e.runners
	e.runners = map[string]*Runner{}
	e.mu.Unlock()
	for _, r := range runners {
		r.stop()
	}
	e.cancel()
	e.proMu.Lock()
	if e.proTimer != nil {
		e.proTimer.Stop()
		e.proTimer = nil
	}
	e.proMu.Unlock()
	e.apMu.Lock()
	for id, t := range e.autoTimers {
		t.Stop()
		delete(e.autoTimers, id)
	}
	e.apMu.Unlock()
	e.stopRecap()
	e.stopJobs()
}

func (e *Engine) runner(key string) *Runner {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.runners[key]
}

func (e *Engine) allRunners() []*Runner {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]*Runner, 0, len(e.runners))
	for _, r := range e.runners {
		out = append(out, r)
	}
	return out
}

// HandleIncoming routes a WhatsApp message to its chat's runner (by key, else by
// JID/alt JID). Messages for unassigned or disabled chats are dropped. Never blocks.
func (e *Engine) HandleIncoming(in model.Incoming) {
	if r := e.route(in); r != nil {
		r.enqueue(in)
	}
}

func (e *Engine) route(in model.Incoming) *Runner {
	if in.ChatKey != "" {
		if r := e.runner(in.ChatKey); r != nil {
			return r
		}
	}
	for _, jid := range []string{in.ChatJID, in.AltJID} {
		if jid == "" {
			continue
		}
		if c, ok := e.store.ChatByJID(jid); ok {
			if r := e.runner(c.Key); r != nil {
				return r
			}
		}
	}
	return nil
}

// Simulate pushes a fake message through the real pipeline of an assigned chat.
// fromMe behaves like your own message (trigger prefix → immediate reply, else context only).
// In groups the message comes from senderJID when it is a member, else from a
// member picked at random (else "Tester").
func (e *Engine) Simulate(chatKey, text string, fromMe bool, senderJID string) error {
	c, ok := e.store.Chat(chatKey)
	if !ok {
		return fmt.Errorf("chat %q: %w", chatKey, ErrNotFound)
	}
	r := e.runner(chatKey)
	if !c.Enabled || r == nil {
		return ErrChatDisabled
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("text is required")
	}
	name, sender := c.Name, ""
	if c.Kind == "group" && !fromMe {
		name, sender = e.simulatedSender(c, senderJID)
	}
	r.enqueue(model.Incoming{
		ChatKey:   c.Key,
		AltJID:    c.AltJID,
		IsGroup:   c.Kind == "group",
		IsFromMe:  fromMe,
		SenderJID: sender,
		PushName:  name,
		Text:      text,
		Timestamp: e.clock.Now(),
		MessageID: store.NewID(),
	})
	return nil
}

// ClearHistory forgets a chat's conversation, both the runner's memory and the file.
func (e *Engine) ClearHistory(chatKey string) error {
	if r := e.runner(chatKey); r != nil {
		r.mu.Lock()
		r.history = nil
		r.mu.Unlock()
	}
	return e.store.ClearHistory(chatKey)
}

// SendManual sends text as the persona and records it in history. Like your
// own reply from the phone, it stands the persona down in that chat.
func (e *Engine) SendManual(ctx context.Context, chatKey, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("text is required")
	}
	c, ok := e.store.Chat(chatKey)
	if !ok {
		return fmt.Errorf("chat %q: %w", chatKey, ErrNotFound)
	}
	r := e.runner(chatKey)
	if r != nil {
		r.youReplied()
	}
	// Your own "@Name" tags are kept (all of them) and encoded for WhatsApp.
	d := e.directory(ctx, c, e.chatHistory(r, c))
	var names []string
	if d != nil {
		var tagged []mention.Entry
		text, tagged = mention.Normalize(text, d, mention.Policy{Allow: true, Max: d.Len()})
		names = mention.Names(tagged)
	}
	jid, wire, err := e.sendText(ctx, e.targets(r, c), text, nil, d)
	if err != nil {
		e.act(model.ActError, c, "", "Manual send failed: "+err.Error(), nil)
		return err
	}
	e.recordSent(r, c, text, wire, names)
	now := e.clock.Now()
	e.updateState(c.Key, func(st *store.RunnerState) { st.LastYouRepliedAt = now })
	e.act(model.ActSent, c, "", text, mentionMeta(map[string]any{"manual": true, "jid": jid}, names))
	return nil
}

// PromptPreview renders the system prompt the persona would get in a chat.
func (e *Engine) PromptPreview(personaID, chatKey string) (string, error) {
	p, ok := e.store.Persona(personaID)
	if !ok {
		return "", fmt.Errorf("persona %q: %w", personaID, ErrNotFound)
	}
	var c model.ChatAssignment
	last := ""
	if chatKey != "" {
		if cc, ok := e.store.Chat(chatKey); ok {
			c = cc
			if h, err := e.store.History(chatKey, 1); err == nil && len(h) > 0 {
				last = h[0].Text
			}
		}
	}
	opts := e.previewOptions(c)
	return prompt.SystemPrompt(p, c, c.Kind == "group", last, opts), nil
}

// ─── shared helpers ──────────────────────────────────────────

// effective resolves a chat's behaviour from the current settings.
func (e *Engine) effective(c model.ChatAssignment) behavior.Effective {
	return e.withPersonaZone(behavior.Resolve(e.cfg.Get().Behavior.For(behavior.KindOf(c)), c), c) // routine.go
}

// updateState changes a chat's persisted runtime state (best effort).
func (e *Engine) updateState(key string, fn func(*store.RunnerState)) {
	now := e.clock.Now()
	_, _ = e.store.UpdateRunnerState(key, func(st *store.RunnerState) {
		fn(st)
		st.Prune(now)
	})
}

// targets lists the JIDs to try, preferring the address the last incoming message used.
func (e *Engine) targets(r *Runner, c model.ChatAssignment) []string {
	pref := ""
	if r != nil {
		pref = r.preferredJID()
	}
	var out []string
	for _, j := range []string{pref, c.JID, c.AltJID} {
		if j != "" && !containsStr(out, j) {
			out = append(out, j)
		}
	}
	return out
}

func (e *Engine) typing(jid string, on bool) {
	if jid == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	e.wa.Typing(ctx, jid, on)
}

// sleep waits d on the engine clock; false when ctx ends first. While parked
// on the timer the goroutine does not count as busy.
func (e *Engine) sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	ch := make(chan struct{})
	t := e.clock.AfterFunc(d, func() {
		e.busy.Add(1) // handed back to the sleeper
		close(ch)
	})
	e.busy.Add(-1)
	select {
	case <-ch:
		return ctx.Err() == nil
	case <-ctx.Done():
		if t.Stop() {
			e.busy.Add(1)
		} else {
			<-ch
		}
		return false
	}
}

// goBusy runs fn in a goroutine counted as busy.
func (e *Engine) goBusy(fn func()) {
	e.busy.Add(1)
	go func() {
		defer e.busy.Add(-1)
		fn()
	}()
}

// act publishes an activity event for a chat.
func (e *Engine) act(typ string, c model.ChatAssignment, personaName, text string, meta map[string]any) {
	if personaName == "" && c.PersonaID != "" {
		if p, ok := e.store.Persona(c.PersonaID); ok {
			personaName = p.Name
		}
	}
	e.hub.Activity(model.ActivityEvent{
		Type:        typ,
		ChatKey:     c.Key,
		ChatName:    c.Name,
		PersonaName: personaName,
		Text:        guard.Truncate(text, 500),
		Meta:        meta,
	})
}

func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
