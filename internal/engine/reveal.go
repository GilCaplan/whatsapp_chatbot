package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/mention"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

// Reveal (wave 3): you tell a chat it was talking to a persona. The message
// goes out as yours (no typos, no delays), the chat is paused and marked
// revealed. See WAVE3_PLAN §3.8.

// ErrAlreadyRevealed means the chat was revealed (and is still paused);
// pass force to send the message again.
var ErrAlreadyRevealed = errors.New("this chat was already revealed")

// RenderReveal fills the reveal template: {persona} → the persona's name,
// {me} → your WhatsApp name ("me" when unknown).
func RenderReveal(tpl, persona, me string) string {
	if strings.TrimSpace(persona) == "" {
		persona = "a persona"
	}
	if strings.TrimSpace(me) == "" {
		me = "me"
	}
	return strings.TrimSpace(strings.NewReplacer("{persona}", persona, "{me}", me).Replace(tpl))
}

// myName is your WhatsApp push name ("" when unknown).
func (e *Engine) myName() string {
	if e.wa == nil {
		return ""
	}
	if me := e.wa.Status().Me; me != nil {
		return strings.TrimSpace(me.PushName)
	}
	return ""
}

// Reveal sends the "it was a persona" message (text "" = the settings
// template with {persona} and {me} filled in), pauses the chat and returns
// the text that was sent. force sends it again after an earlier reveal.
func (e *Engine) Reveal(ctx context.Context, chatKey, text string, force bool) (string, error) {
	c, ok := e.store.Chat(chatKey)
	if !ok {
		return "", fmt.Errorf("chat %q: %w", chatKey, ErrNotFound)
	}
	if c.RevealedAt != nil && !c.Enabled && !force {
		return "", ErrAlreadyRevealed
	}
	again := c.RevealedAt != nil
	p, _ := e.store.Persona(c.PersonaID)
	text = strings.TrimSpace(text)
	if text == "" {
		text = RenderReveal(e.cfg.Get().Safety.Reveal.Template, p.Name, e.myName())
	}
	if text == "" {
		return "", errors.New("the reveal message is empty")
	}

	// Stand the persona down: no reply in flight, nothing waiting to go out.
	r := e.runner(chatKey)
	if r != nil {
		var fx effects
		r.mu.Lock()
		if r.cyc != nil || r.phase != phaseIdle {
			r.endCycleLocked(&fx)
		}
		r.mu.Unlock()
		fx.run()
	}
	for _, a := range e.store.Approvals() {
		if a.ChatKey == chatKey {
			_ = e.Discard(a.ID)
		}
	}

	// Your own "@Name" tags are kept and encoded, like a manual send.
	d := e.directory(ctx, c, e.chatHistory(r, c))
	var names []string
	if d != nil {
		var tagged []mention.Entry
		text, tagged = mention.Normalize(text, d, mention.Policy{Allow: true, Max: d.Len()})
		names = mention.Names(tagged)
	}
	jid, wire, res, err := e.sendTextResult(ctx, e.targets(r, c), text, nil, d)
	if err != nil {
		e.act(model.ActError, c, p.Name, "Couldn't send the reveal: "+err.Error(), nil)
		return "", err
	}
	now := e.clock.Now()
	msg := model.Message{ID: store.NewID(), TS: now, Speaker: "me", Text: text, Kind: model.MsgKindReveal, WAID: res.ID, Mentions: names}
	if r != nil {
		r.recordSent(msg, wire)
	} else {
		_ = e.store.AppendHistory(c.Key, msg, e.effective(c).Profile.HistoryMessages)
	}
	e.store.TouchChat(c.Key, now)
	e.updateState(c.Key, func(st *store.RunnerState) { st.LastYouRepliedAt = now; st.QueuedWakeAt = nil })
	if updated, err := e.store.UpdateChat(c.Key, func(x *model.ChatAssignment) {
		x.Enabled = false
		x.RevealedAt = &now
		x.Handoff = nil
	}); err == nil {
		c = updated
	}
	e.Reload()           // the chat is paused: its runner stops
	e.forgetBrief(c.Key) // a revealed chat is never a source for other chats
	who := p.Name
	if who == "" {
		who = "the persona"
	}
	where := c.Name
	if where == "" {
		where = "this chat"
	}
	e.act(model.ActReveal, c, p.Name, fmt.Sprintf("Revealed %s to %s and paused this chat", who, where),
		map[string]any{"jid": jid, "message": text, "again": again})
	if e.hub != nil {
		e.hub.Publish(events.TypeChatsChanged, struct{}{})
	}
	return text, nil
}
