package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
)

// What the persona reads (model.ChatContext): the chat's history from
// ContextFrom on, plus the summary of what came before and your
// instructions for the chat. Compacting folds the older messages into the
// summary and moves ContextFrom up — by hand (Compact) or, with AutoCompact,
// whenever the history it reads is full.

const (
	compactKeep     = 8 // messages kept verbatim after compacting by hand
	compactIdle     = 30 * time.Second
	compactTimeout  = 90 * time.Second
	compactKeepFrac = 2 // auto-compact keeps half the history window
)

// ErrNothingToCompact: every message it reads is among the ones kept anyway.
var ErrNothingToCompact = errors.New("nothing older to compact yet")

// errCompactStale: the summary or where the history starts changed while
// the summary was being written (you edited it, started fresh…); yours wins.
var errCompactStale = errors.New("the chat's summary or history changed meanwhile, try again")

func compactJob(key string) string { return "compact:" + key }

// fromContext drops the messages before from (nil = keep all); hist is not changed.
func fromContext(hist []model.Message, from *time.Time) []model.Message {
	if from == nil {
		return hist
	}
	i := slices.IndexFunc(hist, func(m model.Message) bool { return !m.TS.Before(*from) })
	if i < 0 {
		return nil
	}
	return hist[i:]
}

// compactTick (re)arms auto-compaction when the history it reads is full.
func (e *Engine) compactTick(c model.ChatAssignment) {
	cur, ok := e.store.Chat(c.Key)
	if !ok || !cur.AutoCompact {
		return
	}
	window := e.effective(cur).Profile.HistoryMessages
	all, err := e.store.History(cur.Key, 0)
	if err != nil || window <= 0 || len(fromContext(all, cur.ContextFrom)) < window {
		return
	}
	key, keep := cur.Key, max(compactKeep, window/compactKeepFrac)
	e.schedule(compactJob(key), compactIdle, func(ctx context.Context) {
		cc, ok := e.store.Chat(key)
		if !ok || !cc.AutoCompact {
			return
		}
		if _, err := e.compactChat(ctx, cc, keep); err != nil && !errors.Is(err, ErrNothingToCompact) && !errors.Is(err, errCompactStale) && ctx.Err() == nil {
			e.act(model.ActError, cc, "", "Couldn't summarise the older messages: "+err.Error(), nil)
		}
	})
}

// Compact folds everything but the last keep messages (0 = compactKeep) into
// the chat summary now. It waits for the background slot.
func (e *Engine) Compact(ctx context.Context, chatKey string, keep int) (model.ChatSummary, error) {
	c, ok := e.store.Chat(chatKey)
	if !ok {
		return model.ChatSummary{}, fmt.Errorf("chat %q: %w", chatKey, ErrNotFound)
	}
	if keep <= 0 {
		keep = compactKeep
	}
	e.cancelJob(compactJob(chatKey))
	var sm model.ChatSummary
	var err error
	if !e.withJobSlot(ctx, func() {
		if cur, ok := e.store.Chat(chatKey); ok { // it may have changed while waiting for the slot
			c = cur
		}
		sm, err = e.compactChat(ctx, c, keep)
	}) {
		return sm, ctx.Err()
	}
	return sm, err
}

// compactChat writes the new summary and moves the chat's ContextFrom to the
// first message kept.
func (e *Engine) compactChat(ctx context.Context, c model.ChatAssignment, keep int) (model.ChatSummary, error) {
	p, ok := e.store.Persona(c.PersonaID)
	if !ok {
		return model.ChatSummary{}, fmt.Errorf("persona %q not found", c.PersonaID)
	}
	all, err := e.store.History(c.Key, 0)
	if err != nil {
		return model.ChatSummary{}, err
	}
	active := slices.DeleteFunc(slices.Clone(fromContext(all, c.ContextFrom)), func(m model.Message) bool {
		return m.Kind == model.MsgKindFix || m.Kind == model.MsgKindReveal
	})
	if len(active) <= keep {
		return model.ChatSummary{}, ErrNothingToCompact
	}
	old, kept := active[:len(active)-keep], active[len(active)-keep:]
	prev, hadPrev := e.store.Summary(c.Key)
	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err != nil {
		return model.ChatSummary{}, err
	}
	// One request reads at most prompt.MaxCompactMessages: fold longer runs in
	// chunks, oldest first, each building on the summary so far.
	text := prev.Text
	for start := 0; start < len(old); start += prompt.MaxCompactMessages {
		chunk := old[start:min(start+prompt.MaxCompactMessages, len(old))]
		req := prompt.Compact(prompt.CompactInput{PersonaName: p.Name, ChatName: c.Name, IsGroup: isGroupChat(c),
			Previous: text, Messages: chunk, Zone: time.Local})
		req.Model = mdl
		cctx, cancel := context.WithTimeout(ctx, compactTimeout)
		resp, err := prov.Chat(cctx, req)
		cancel()
		if err != nil {
			return model.ChatSummary{}, err
		}
		text = strings.TrimSpace(resp.Text)
		if text == "" {
			return model.ChatSummary{}, errors.New("the model wrote an empty summary")
		}
		if utf8.RuneCountInString(text) > model.MaxSummaryText {
			text = string([]rune(text)[:model.MaxSummaryText])
		}
	}
	// The model call takes a while: never overwrite what you changed meanwhile.
	cur, ok := e.store.Chat(c.Key)
	if !ok {
		return model.ChatSummary{}, fmt.Errorf("chat %q: %w", c.Key, ErrNotFound)
	}
	if again, has := e.store.Summary(c.Key); has != hadPrev || !again.UpdatedAt.Equal(prev.UpdatedAt) || !sameTime(cur.ContextFrom, c.ContextFrom) {
		return model.ChatSummary{}, errCompactStale
	}
	now := e.clock.Now()
	sm := model.ChatSummary{ChatKey: c.Key, Text: text, Through: old[len(old)-1].TS, Messages: prev.Messages + len(old), UpdatedAt: now}
	if err := e.store.SaveSummary(sm); err != nil {
		return model.ChatSummary{}, err
	}
	from := kept[0].TS
	if _, err := e.store.UpdateChat(c.Key, func(x *model.ChatAssignment) { x.ContextFrom = &from }); err != nil {
		return model.ChatSummary{}, err
	}
	if r := e.runner(c.Key); r != nil {
		r.mu.Lock()
		r.cfg.ContextFrom = &from
		r.mu.Unlock()
	}
	e.act(model.ActMemory, c, p.Name, fmt.Sprintf("Summarised %d older messages — it now reads the summary and the last %d", len(old), len(kept)),
		map[string]any{"compacted": len(old)})
	if e.hub != nil {
		e.hub.Publish(events.TypeChatsChanged, struct{}{})
	}
	return sm, nil
}

// ChatContext is what the persona reads for its next reply in the chat.
func (e *Engine) ChatContext(chatKey string) (model.ChatContext, error) {
	c, ok := e.store.Chat(chatKey)
	if !ok {
		return model.ChatContext{}, fmt.Errorf("chat %q: %w", chatKey, ErrNotFound)
	}
	p, ok := e.store.Persona(c.PersonaID)
	if !ok {
		return model.ChatContext{}, fmt.Errorf("persona %q: %w", c.PersonaID, ErrNotFound)
	}
	hist := e.chatHistory(e.runner(chatKey), c)
	bp := e.effective(c).Profile
	opts, _ := e.promptOptions(c, bp, e.directory(e.ctx, c, hist), hist)
	req := prompt.Compose(p, c, hist, isGroupChat(c), opts)
	out := model.ChatContext{System: req.System, Messages: make([]model.ContextMessage, 0, len(req.Messages)), Chars: len(req.System),
		HistoryKept: len(hist), Window: bp.HistoryMessages, Instructions: c.Instructions, ContextFrom: c.ContextFrom, AutoCompact: c.AutoCompact}
	for _, m := range req.Messages {
		out.Messages = append(out.Messages, model.ContextMessage{Role: m.Role, Content: m.Content})
		out.Chars += len(m.Content)
	}
	if all, err := e.store.History(chatKey, 0); err == nil {
		out.HistoryTotal = len(all)
	}
	if sm, ok := e.store.Summary(chatKey); ok {
		out.Summary = &sm
	}
	return out, nil
}

// sameTime reports whether two optional times are both unset or equal.
func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
