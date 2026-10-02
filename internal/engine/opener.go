package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"whatsappdoppel/internal/guard"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
	"whatsappdoppel/internal/store"
)

// ErrReplying means the persona is already busy with a reply in that chat.
var ErrReplying = errors.New("the persona is already writing in this chat — try again in a moment")

// openerFor completes a cycle's opener options with how long the chat has been quiet.
func (e *Engine) openerFor(op prompt.Opener, hist []model.Message) prompt.Opener {
	var last time.Time
	for _, m := range hist {
		if m.TS.After(last) {
			last = m.TS
		}
	}
	if !last.IsZero() {
		if d := e.clock.Now().Sub(last); d > 0 {
			op.Silence = d
		}
	}
	return op
}

// Initiate makes the persona start a conversation now ("Start the
// conversation" in the chat panel). It runs the normal reply cycle from
// thinking on — goal-aware opener, plan ahead, typing and bubbles per the
// chat's behaviour — or queues the opener for approval in approval mode.
// Because you asked explicitly it ignores active hours, Away and reply
// limits; the chat must be enabled and not already replying.
func (e *Engine) Initiate(ctx context.Context, chatKey, hint string) error {
	c, ok := e.store.Chat(chatKey)
	if !ok {
		return fmt.Errorf("chat %q: %w", chatKey, ErrNotFound)
	}
	if _, ok := e.store.Persona(c.PersonaID); !ok {
		return fmt.Errorf("persona %q: %w", c.PersonaID, ErrNotFound)
	}
	r := e.runner(chatKey)
	if !c.Enabled || r == nil {
		return ErrChatDisabled
	}
	hint = strings.TrimSpace(guard.Truncate(hint, prompt.MaxOpenerHint))
	var fx effects
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return ErrChatDisabled
	}
	if r.phase != phaseIdle {
		r.mu.Unlock()
		return ErrReplying
	}
	r.startCycleLocked(nil, false, true)
	r.cyc.opener = prompt.Opener{Manual: true, Hint: hint}
	meta := map[string]any{"stage": "manual"}
	text := "Starting a conversation because you asked"
	if hint != "" {
		meta["hint"] = hint
		text += " — about: " + guard.Truncate(hint, 80)
	}
	if r.cfg.ApprovalMode {
		text += " (it will wait for your approval)"
	}
	r.actFx(&fx, model.ActProactive, text, meta)
	r.thinkLocked(&fx)
	r.mu.Unlock()
	fx.run()
	return nil
}

// Initiate asks the persona for an opener in a playground session (the
// persona speaks first). hint is optional.
func (pg *playground) Initiate(ctx context.Context, id string, group bool, hint string) (model.PlaygroundReply, error) {
	e := pg.e
	s, err := pg.session(id)
	if err != nil {
		return model.PlaygroundReply{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now()
	p, ok := e.store.Persona(s.personaID)
	if !ok {
		return model.PlaygroundReply{}, fmt.Errorf("persona %q: %w", s.personaID, ErrNotFound)
	}
	rp := e.cfg.Get().Behavior.Private
	kind := "dm"
	if group {
		rp, kind = e.cfg.Get().Behavior.Group, "group"
	}
	out := model.PlaygroundReply{}
	if prov, mdl, err := e.llm.Resolve(p.LLM); err == nil {
		out.Provider, out.Model = prov.Name(), mdl
	}
	chat := model.ChatAssignment{Key: "playground:" + id, Kind: kind, Name: "Playground", PersonaID: p.ID, Enabled: true, GoalOverride: s.goal}
	hist := append([]model.Message(nil), s.history...)
	opts := prompt.Options{LengthBias: rp.LengthBias, Opener: e.openerFor(prompt.Opener{Manual: true, Hint: strings.TrimSpace(guard.Truncate(hint, prompt.MaxOpenerHint))}, hist)}
	start := time.Now()
	gcfg, gturn := e.playgroundGoalTurn(ctx, p, chat, &s.goalStatus, hist, group, "", true)
	res, err := e.expressiveReply(ctx, p, rp, hist, true, gcfg, gturn, nil, func(t prompt.GoalTurn, _ prompt.CrossTurn) llm.Request {
		opts.Goal = t
		return prompt.Initiate(p, chat, hist, group, opts)
	})
	out.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		return out, err
	}
	out.Reply, out.Provider, out.Model = res.Text, res.Provider, res.Model
	out.Goal = playgroundGoalView(gcfg, gturn, s.goalStatus, res)
	s.history = trimHistory(append(s.history, model.Message{
		ID: store.NewID(), TS: time.Now(), Speaker: "me", Text: res.Text, FromBot: true,
	}), rp.HistoryMessages, rp.HistoryChars)
	return out, nil
}
