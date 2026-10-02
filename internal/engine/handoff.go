package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/guard"
	"whatsappdoppel/internal/handoff"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
	"whatsappdoppel/internal/store"
)

// Hand-off (wave 3). A sensitive message — money, health, meeting up,
// distress, "are you a bot?", legal — pauses the persona in that chat
// (ChatAssignment.Handoff) until you resume it. Strong keyword hits pause
// right away; weak ones are confirmed by one tiny AI check when
// safety.handoff.aiCheck is on. The check runs on every message from them,
// before the people/decision gates, so it fires even when the persona
// would have stayed quiet. See WAVE3_PLAN §3.6.

// timeoutHandoffCheck bounds the AI confirmation; on timeout nothing pauses.
const timeoutHandoffCheck = 15 * time.Second

// handoffSkipEvery: while paused, the "waiting for you" skip is logged at
// most this often (not for every message).
const handoffSkipEvery = 10 * time.Minute

// handoffCategories maps the settings to the classifier's categories.
func handoffCategories(h config.HandoffSettings) handoff.Categories {
	return handoff.Categories{
		model.HandoffMoney:    h.Money,
		model.HandoffHealth:   h.Health,
		model.HandoffMeeting:  h.Meeting,
		model.HandoffDistress: h.Distress,
		model.HandoffBot:      h.Bot,
		model.HandoffLegal:    h.Legal,
	}
}

// handoffGate runs for every message from them (Runner.process). It returns
// true when the message was taken over by the hand-off logic: the chat is
// already paused (the message is kept as context), or this message pauses it.
func (r *Runner) handoffGate(c model.ChatAssignment, p model.Persona, in model.Incoming, text string, mentions []string,
	bp model.BehaviorProfile, isGroup bool, now time.Time) bool {
	e := r.e
	if cur := r.config(); cur.Handoff != nil {
		r.keepAsContext(in, text, mentions, bp, now)
		r.mu.Lock()
		due := r.handoffNoted.IsZero() || now.Sub(r.handoffNoted) >= handoffSkipEvery
		if due {
			r.handoffNoted = now
		}
		r.mu.Unlock()
		if due {
			e.act(model.ActDecisionSkip, c, p.Name, "Paused — waiting for you to take over (kept as context)",
				map[string]any{"reason": "handoff", "category": cur.Handoff.Category})
		}
		return true
	}
	hs := e.cfg.Get().Safety.Handoff
	if !hs.Enabled {
		return false
	}
	cats := handoffCategories(hs)
	hit, ok := handoff.Detect(text, cats)
	if !ok {
		return false
	}
	who := senderName(in, c, isGroup)
	how := model.HandoffHowKeyword
	if hit.Strength == handoff.Weak {
		if !hs.AICheck {
			return false
		}
		cat, serious := e.handoffCheck(r.ctx, p, cats, r.historySnapshot(), who, text)
		if !serious {
			return false
		}
		hit.Category, how = cat, model.HandoffHowAI
	}
	e.triggerHandoff(r, c, p, hit, how, who, in, text, mentions, bp, now)
	return true
}

// handoffCheck asks the persona's model whether a weak keyword hit is really
// about one of the categories. Any error means "no".
func (e *Engine) handoffCheck(ctx context.Context, p model.Persona, cats handoff.Categories, hist []model.Message, who, text string) (string, bool) {
	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err != nil {
		return "", false
	}
	list := cats.List()
	req := prompt.HandoffCheck(p.Name, list, hist, who, text)
	req.Model = mdl
	cctx, cancel := context.WithTimeout(ctx, timeoutHandoffCheck)
	defer cancel()
	resp, err := prov.Chat(cctx, req)
	if err != nil {
		return "", false
	}
	return prompt.ParseHandoffCheck(resp.Text, list)
}

// triggerHandoff pauses a chat: the reply cycle ends (typing off), pending
// replies stop auto-sending, the message is kept as context and you are told.
func (e *Engine) triggerHandoff(r *Runner, c model.ChatAssignment, p model.Persona, hit handoff.Hit, how, who string,
	in model.Incoming, text string, mentions []string, bp model.BehaviorProfile, now time.Time) {
	if who == "" {
		who = c.Name
	}
	hs := &model.HandoffState{
		Category:  hit.Category,
		Excerpt:   guard.Truncate(strings.Join(strings.Fields(text), " "), 160),
		Sender:    who,
		MessageID: in.MessageID,
		At:        now,
		How:       how,
	}
	if _, err := e.store.UpdateChat(c.Key, func(x *model.ChatAssignment) { x.Handoff = hs }); err != nil {
		e.act(model.ActError, c, p.Name, "Couldn't save the pause: "+err.Error(), nil)
	}
	var fx effects
	r.mu.Lock()
	r.cfg.Handoff = hs
	r.handoffNoted = time.Time{} // the next message logs "waiting for you" once
	wasQueued := r.phase == phaseQueued
	if r.cyc != nil || r.phase != phaseIdle {
		r.endCycleLocked(&fx)
	}
	r.mu.Unlock()
	fx.run()
	if wasQueued {
		e.updateState(c.Key, func(st *store.RunnerState) { st.QueuedWakeAt = nil })
	}
	r.keepAsContext(in, text, mentions, bp, now)
	e.holdApprovals(c.Key)
	e.forgetBrief(c.Key) // its window held the sensitive message: nothing of it crosses (brief.go)
	meta := map[string]any{"category": hit.Category, "excerpt": hs.Excerpt, "how": how, "sender": who}
	if hit.Match != "" {
		meta["match"] = hit.Match
	}
	e.act(model.ActHandoff, c, p.Name, handoff.Reason(hit.Category, who)+" — paused so you can take over", meta)
	if e.hub != nil {
		e.hub.Publish(events.TypeChatsChanged, struct{}{})
	}
}

// holdApprovals stops a paused chat's pending replies from sending on their
// own: the auto-send timer is cancelled and they are marked stale. You can
// still send or discard them yourself.
func (e *Engine) holdApprovals(chatKey string) {
	for _, p := range e.store.Approvals() {
		if p.ChatKey != chatKey {
			continue
		}
		e.cancelAutoSend(p.ID)
		if p.AutoSendAt == nil && p.Stale {
			continue
		}
		p.AutoSendAt, p.Stale = nil, true
		if e.store.UpsertApproval(p) == nil {
			e.hub.Publish(events.TypeApproval, events.ApprovalEvent{Action: "updated", Pending: p})
		}
	}
}

// ResumeHandoff clears a chat's hand-off pause (activity handoff.resumed).
// The next message is answered normally; what was said meanwhile is in the
// history, so the persona knows. Resuming a chat that isn't paused is a no-op.
func (e *Engine) ResumeHandoff(chatKey string) error {
	c, ok := e.store.Chat(chatKey)
	if !ok {
		return fmt.Errorf("chat %q: %w", chatKey, ErrNotFound)
	}
	if c.Handoff == nil {
		return nil
	}
	prev := *c.Handoff
	c, err := e.store.UpdateChat(chatKey, func(x *model.ChatAssignment) { x.Handoff = nil })
	if err != nil {
		return err
	}
	if r := e.runner(chatKey); r != nil {
		r.mu.Lock()
		r.cfg.Handoff = nil
		r.handoffNoted = time.Time{}
		r.mu.Unlock()
	}
	name := "The persona"
	if p, ok := e.store.Persona(c.PersonaID); ok {
		name = p.Name
	}
	where := c.Name
	if where == "" {
		where = "this chat"
	}
	e.act(model.ActHandoffResumed, c, "", fmt.Sprintf("Back on — %s is answering in %s again", name, where),
		map[string]any{"category": prev.Category})
	if e.hub != nil {
		e.hub.Publish(events.TypeChatsChanged, struct{}{})
	}
	return nil
}
