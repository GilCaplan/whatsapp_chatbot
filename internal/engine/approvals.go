package engine

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/mention"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
	"whatsappdoppel/internal/store"
)

// queueApproval stores a generated reply for review. A chat keeps at most one
// pending reply: a newer generation replaces the text of the existing one.
// mentions are the display names the reply tags.
func (e *Engine) queueApproval(r *Runner, c model.ChatAssignment, p model.Persona, res genResult, hist []model.Message, mentions []string) {
	now := e.clock.Now()
	pend := model.PendingReply{
		ID:          store.NewID(),
		ChatKey:     c.Key,
		ChatName:    c.Name,
		PersonaID:   p.ID,
		PersonaName: p.Name,
		CreatedAt:   now,
	}
	action := "new"
	r.mu.Lock()
	existing := r.pendingID
	r.mu.Unlock()
	if existing != "" {
		if old, ok := e.store.Approval(existing); ok {
			pend.ID, pend.CreatedAt, action = old.ID, old.CreatedAt, "updated"
		}
	}
	e.fillPending(&pend, res, hist, now, autoSendFor(c, e.effective(c).Profile))
	pend.Mentions = mentions
	if err := e.store.UpsertApproval(pend); err != nil {
		e.act(model.ActError, c, p.Name, "Couldn't save the pending reply: "+err.Error(), nil)
		return
	}
	r.setPending(pend.ID)
	e.armAutoSend(pend)
	e.hub.Publish(events.TypeApproval, events.ApprovalEvent{Action: action, Pending: pend})
	m := mentionMeta(res.meta(), mentions)
	m["approvalId"] = pend.ID
	if pend.AutoSendAt != nil {
		m["autoSendSeconds"] = e.effective(c).Profile.AutoSendSeconds
	}
	text := res.Text
	if n := len(pend.Drafts); n > 0 {
		m["drafts"] = n
		text = fmt.Sprintf("%d ideas ready to pick from — first: %s", n, res.Text)
	}
	e.act(model.ActApprovalQueued, c, p.Name, text, m)
}

// autoSendFor is the auto-send delay of a chat's pending replies: none in
// co-pilot mode (you always pick) or while the chat waits for you (hand-off).
func autoSendFor(c model.ChatAssignment, bp model.BehaviorProfile) int {
	if c.Mode == model.ChatModeCopilot || c.Handoff != nil {
		return 0
	}
	return bp.AutoSendSeconds
}

func (e *Engine) fillPending(pend *model.PendingReply, res genResult, hist []model.Message, now time.Time, autoSendSeconds int) {
	pend.Text = res.Text
	pend.Drafts = res.Drafts
	pend.Opener = res.Opener
	pend.Stale = false
	pend.Provider, pend.Model = res.Provider, res.Model
	pend.CrossUsed = res.crossUsed()
	pend.Context = hist[max(0, len(hist)-contextSize):]
	pend.AutoSendAt = nil
	if secs := autoSendSeconds; secs > 0 {
		t := now.Add(time.Duration(secs) * time.Second)
		pend.AutoSendAt = &t
	}
}

// armAutoSend (re)starts the auto-send timer for a pending reply, if it has one.
func (e *Engine) armAutoSend(p model.PendingReply) {
	e.apMu.Lock()
	defer e.apMu.Unlock()
	if t, ok := e.autoTimers[p.ID]; ok {
		t.Stop()
		delete(e.autoTimers, p.ID)
	}
	if p.AutoSendAt == nil {
		return
	}
	id := p.ID
	d := max(0, p.AutoSendAt.Sub(e.clock.Now()))
	e.autoTimers[id] = e.clock.AfterFunc(d, func() {
		e.apMu.Lock()
		delete(e.autoTimers, id)
		e.apMu.Unlock()
		pend, ok := e.store.Approval(id)
		if !ok {
			return
		}
		if c, ok := e.store.Chat(pend.ChatKey); !ok || !c.Enabled || c.Handoff != nil || c.Mode == model.ChatModeCopilot || e.ctx.Err() != nil {
			return
		}
		// Delivery waits on the clock (typing), so never inside the timer callback.
		e.goBusy(func() {
			ctx, cancel := context.WithTimeout(e.ctx, 60*time.Second)
			defer cancel()
			_ = e.SendApproved(ctx, id, "", -1)
		})
	})
}

func (e *Engine) cancelAutoSend(id string) {
	e.apMu.Lock()
	if t, ok := e.autoTimers[id]; ok {
		t.Stop()
		delete(e.autoTimers, id)
	}
	e.apMu.Unlock()
}

// SendApproved sends a pending reply (optionally edited) and removes it from
// the queue. In co-pilot mode draft picks one of p.Drafts (-1 = none); text,
// when given, always wins (your edit of the chosen draft).
func (e *Engine) SendApproved(ctx context.Context, id, text string, draft int) error {
	p, ok := e.store.Approval(id)
	if !ok {
		return fmt.Errorf("approval %q: %w", id, ErrNotFound)
	}
	if draft >= len(p.Drafts) || draft < -1 {
		return fmt.Errorf("draft %d: %w", draft, ErrNoSuchDraft)
	}
	if strings.TrimSpace(text) == "" && draft >= 0 {
		text = p.Drafts[draft].Text
	}
	e.apMu.Lock()
	if e.sending[id] {
		e.apMu.Unlock()
		return ErrBusy
	}
	e.sending[id] = true
	e.apMu.Unlock()
	e.busy.Add(1) // delivery may park on the clock (typing)
	defer e.busy.Add(-1)
	defer func() {
		e.apMu.Lock()
		delete(e.sending, id)
		e.apMu.Unlock()
	}()

	c, ok := e.store.Chat(p.ChatKey)
	if !ok {
		return fmt.Errorf("chat %q: %w", p.ChatKey, ErrNotFound)
	}
	if text = strings.TrimSpace(text); text == "" {
		text = p.Text
	}
	r := e.runner(c.Key)
	eff := e.effective(c)
	edited := text != p.Text
	// Tags in the (possibly edited) text are resolved again: you may tag
	// anyone in the group yourself.
	d := e.directory(ctx, c, e.chatHistory(r, c))
	if d != nil {
		text, _ = mention.Normalize(text, d, mention.Policy{Allow: true, Max: d.Len()})
	}
	// Approved replies are delivered like any other (typing, split bubbles),
	// but quickly: typing and gaps are capped at 3 s and nothing is quoted.
	bubbles := behavior.PlanBubbles(eff.Profile, e.rng, text, nil, true)
	targets := e.targets(r, c)
	typingOn := eff.Profile.TypingIndicator
	n := len(bubbles)
	for i, b := range bubbles {
		if i > 0 && !e.sleep(ctx, b.GapBefore) {
			return ctx.Err()
		}
		if typingOn {
			e.typingFor(r, targets, true)
		}
		if b.Typing > 0 {
			e.act(model.ActTyping, c, p.PersonaName, typingText(b, i, n), map[string]any{
				"typingSeconds": b.Typing.Seconds(), "chars": utf8.RuneCountInString(b.Text), "part": i + 1, "parts": n, "approvalId": id,
			})
		}
		ok := e.sleep(ctx, b.Typing)
		var jid, wire string
		var err error
		if ok {
			jid, wire, err = e.sendText(ctx, targets, b.Text, nil, d)
		} else {
			err = ctx.Err()
		}
		if typingOn {
			e.typingFor(r, targets, false)
		}
		if err != nil {
			e.act(model.ActError, c, p.PersonaName, "Send failed: "+err.Error(), map[string]any{"approvalId": id})
			if i > 0 {
				e.recordReplyFor(r, c.Key)
			}
			return err
		}
		if i == 0 {
			e.cancelAutoSend(id)
			_ = e.store.DeleteApproval(id)
			if r != nil {
				r.clearPending(id)
			}
			removed := p
			removed.Text = text
			e.hub.Publish(events.TypeApproval, events.ApprovalEvent{Action: "removed", Pending: removed})
		}
		tags := bubbleMentions(b.Text, d)
		var crossUsed []string
		if !edited {
			crossUsed = p.CrossUsed
		}
		e.recordSent(r, c, b.Text, wire, tags, crossUsed)
		e.act(model.ActApprovalSent, c, p.PersonaName, b.Text, mentionMeta(map[string]any{
			"approvalId": id, "edited": edited, "jid": jid, "provider": p.Provider, "model": p.Model, "part": i + 1, "parts": n,
		}, tags))
	}
	e.recordReplyFor(r, c.Key)
	return nil
}

// typingFor switches the typing indicator through the chat's runner (ordered
// with its own typing calls) or directly.
func (e *Engine) typingFor(r *Runner, targets []string, on bool) {
	if r != nil {
		r.setTypingDirect(on)
		return
	}
	if len(targets) > 0 {
		e.typing(targets[0], on)
	}
}

// recordReplyFor counts a delivered reply (rate limits, cooldown, silence).
func (e *Engine) recordReplyFor(r *Runner, key string) {
	if r != nil {
		r.recordReply(false)
		return
	}
	now := e.clock.Now()
	e.updateState(key, func(st *store.RunnerState) {
		st.LastReplyAt = now
		st.Replies = append(st.Replies, now)
	})
}

// Regenerate rewrites a pending reply from the chat's current history.
func (e *Engine) Regenerate(ctx context.Context, id string) (model.PendingReply, error) {
	p, ok := e.store.Approval(id)
	if !ok {
		return model.PendingReply{}, fmt.Errorf("approval %q: %w", id, ErrNotFound)
	}
	c, ok := e.store.Chat(p.ChatKey)
	if !ok {
		return model.PendingReply{}, fmt.Errorf("chat %q: %w", p.ChatKey, ErrNotFound)
	}
	per, ok := e.store.Persona(c.PersonaID)
	if !ok {
		return model.PendingReply{}, fmt.Errorf("persona %q: %w", c.PersonaID, ErrNotFound)
	}
	var hist []model.Message
	if r := e.runner(c.Key); r != nil {
		hist = r.historySnapshot()
	} else {
		hist, _ = e.store.History(c.Key, e.effective(c).Profile.HistoryMessages)
	}
	bp := e.effective(c).Profile
	dir := e.directory(ctx, c, hist)
	opts, cross := e.promptOptions(c, bp, dir, hist)
	if p.Opener {
		opts.Opener = e.openerFor(prompt.Opener{Manual: true}, hist) // regenerate an opener as an opener
	}
	gcfg, gturn := e.chatGoalTurn(ctx, c, per, hist, c.Kind == "group", p.Opener)
	copilot := c.Mode == model.ChatModeCopilot && !p.Opener // "More ideas" in co-pilot mode
	res, err := e.replyFor(ctx, copilot, per, bp, hist, false, gcfg, gturn, cross, func(t prompt.GoalTurn, x prompt.CrossTurn) llm.Request {
		opts.Goal, opts.CrossTurn = t, x
		if p.Opener {
			return prompt.Initiate(per, c, hist, c.Kind == "group", opts)
		}
		return prompt.Compose(per, c, hist, c.Kind == "group", opts)
	})
	if err != nil {
		return model.PendingReply{}, err
	}
	tagged := e.applyMentionsAll(&res, dir, bp, lastFromThem(hist), hist, false)
	res.Opener = p.Opener
	// It may have been sent or discarded meanwhile.
	if _, ok := e.store.Approval(id); !ok {
		return model.PendingReply{}, fmt.Errorf("approval %q: %w", id, ErrNotFound)
	}
	p.PersonaID, p.PersonaName = per.ID, per.Name
	e.fillPending(&p, res, hist, e.clock.Now(), autoSendFor(c, bp))
	p.Mentions = mention.Names(tagged)
	if err := e.store.UpsertApproval(p); err != nil {
		return model.PendingReply{}, err
	}
	e.armAutoSend(p)
	e.hub.Publish(events.TypeApproval, events.ApprovalEvent{Action: "updated", Pending: p})
	m := mentionMeta(res.meta(), p.Mentions)
	m["approvalId"], m["regenerated"] = id, true
	text := res.Text
	if n := len(p.Drafts); n > 0 {
		m["drafts"] = n
		text = fmt.Sprintf("%d new ideas ready to pick from — first: %s", n, res.Text)
	}
	e.act(model.ActApprovalQueued, c, per.Name, text, m)
	return p, nil
}

// Discard drops a pending reply without sending it.
func (e *Engine) Discard(id string) error {
	p, ok := e.store.Approval(id)
	if !ok {
		return fmt.Errorf("approval %q: %w", id, ErrNotFound)
	}
	e.cancelAutoSend(id)
	if err := e.store.DeleteApproval(id); err != nil {
		return err
	}
	if r := e.runner(p.ChatKey); r != nil {
		r.clearPending(id)
	}
	e.hub.Publish(events.TypeApproval, events.ApprovalEvent{Action: "removed", Pending: p})
	c, _ := e.store.Chat(p.ChatKey)
	if c.Key == "" {
		c = model.ChatAssignment{Key: p.ChatKey, Name: p.ChatName}
	}
	e.act(model.ActApprovalDiscarded, c, p.PersonaName, p.Text, map[string]any{"approvalId": id})
	return nil
}

// markStale flags a pending reply when new messages arrived after it was generated.
func (e *Engine) markStale(id string) {
	p, ok := e.store.Approval(id)
	if !ok || p.Stale {
		return
	}
	p.Stale = true
	if e.store.UpsertApproval(p) == nil {
		e.hub.Publish(events.TypeApproval, events.ApprovalEvent{Action: "updated", Pending: p})
	}
}
