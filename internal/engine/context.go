package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/ctxsys"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
)

// What the persona reads (model.ChatContext): every message since
// ContextFrom word for word, plus the long-term summary of what came before
// and your instructions for the chat. Long-term memory (settings
// memory.compact, per chat CompactEvery): once Every new messages have come
// in on top of the Keep it always reads, all but the last Keep are folded
// into the summary and ContextFrom moves up — so it reads Keep..Keep+Every
// messages and nothing falls between the summary and the history. A fast
// chat folds every FastEvery. "Summarise now" does the same by hand.

const (
	compactIdle    = 30 * time.Second
	compactTimeout = 90 * time.Second
	compactSlack   = 10 // extra history kept in memory past Keep+Every
)

// compactPlan is how a chat's long-term memory works right now.
type compactPlan struct {
	on, fast    bool
	keep, every int
	maxEvery    int // the largest Every it may use (sizes the history window)
}

func (e *Engine) compactPlan(c model.ChatAssignment, all []model.Message) compactPlan {
	k := e.cfg.Get().Memory.Compact
	p := compactPlan{on: !k.Off, keep: k.Keep, every: k.Every, maxEvery: max(k.Every, k.FastEvery)}
	switch {
	case c.CompactEvery < 0:
		p.on = false
	case c.CompactEvery > 0:
		p.on, p.every, p.maxEvery = true, c.CompactEvery, c.CompactEvery
	default:
		since := e.clock.Now().Add(-time.Duration(k.FastHours) * time.Hour)
		n := 0
		for i := len(all) - 1; i >= 0 && !all[i].TS.Before(since); i-- {
			n++
		}
		if p.fast = n >= k.FastMessages; p.fast {
			p.every = k.FastEvery
		}
	}
	p.keep, p.every = max(p.keep, 1), max(p.every, 1)
	return p
}

// historyCap is how many messages (and characters) a chat keeps in memory
// for the prompt: with long-term memory, enough for everything since the
// last summary; otherwise the behaviour's historyMessages/historyChars.
func (e *Engine) historyCap(c model.ChatAssignment, bp model.BehaviorProfile) (int, int) {
	p := e.compactPlan(c, nil)
	if !p.on {
		return bp.HistoryMessages, bp.HistoryChars
	}
	n := max(bp.HistoryMessages, p.keep+p.maxEvery+compactSlack)
	return n, max(bp.HistoryChars, n*120)
}

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

// compactTick (re)arms the long-term summary once Keep+Every messages
// since the last one have come in.
func (e *Engine) compactTick(c model.ChatAssignment) {
	cur, ok := e.store.Chat(c.Key)
	if !ok {
		return
	}
	all, err := e.store.History(cur.Key, 0)
	if err != nil {
		return
	}
	p := e.compactPlan(cur, all)
	if !p.on || len(fromContext(all, cur.ContextFrom)) < p.keep+p.every {
		return
	}
	key, keep := cur.Key, p.keep
	e.schedule(compactJob(key), compactIdle, func(ctx context.Context) {
		cc, ok := e.store.Chat(key)
		if !ok || !e.compactPlan(cc, nil).on {
			return
		}
		if _, err := e.compactChat(ctx, cc, keep); err != nil && !errors.Is(err, ErrNothingToCompact) && !errors.Is(err, errCompactStale) && ctx.Err() == nil {
			e.act(model.ActError, cc, "", "Couldn't summarise the older messages: "+err.Error(), nil)
		}
	})
}

// Compact folds everything but the last keep messages (0 = memory.compact.keep)
// into the chat summary now. It waits for the background slot.
func (e *Engine) Compact(ctx context.Context, chatKey string, keep int) (model.ChatSummary, error) {
	c, ok := e.store.Chat(chatKey)
	if !ok {
		return model.ChatSummary{}, fmt.Errorf("chat %q: %w", chatKey, ErrNotFound)
	}
	if keep <= 0 {
		keep = e.cfg.Get().Memory.Compact.Keep
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

// proseSummaryMax is the size a folded paragraph summary is kept to: what it
// was before context systems, so Classic stays exactly as it was (the 4000
// of model.MaxSummaryText is for facts and for your own edits).
const proseSummaryMax = 1500

func summaryLimit(facts bool) int {
	if facts {
		return model.MaxSummaryText
	}
	return proseSummaryMax
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
	// chunks, oldest first, each building on the summary so far. A context
	// system with summary facts keeps one fact per line and asks only for
	// the changes, so untouched lines never drift.
	facts := e.paramsOf(c).Summary == model.SummaryFacts
	prose := prev.Format != model.SummaryFacts && strings.TrimSpace(prev.Text) != ""
	text := prev.Text
	for start := 0; start < len(old); start += prompt.MaxCompactMessages {
		chunk := old[start:min(start+prompt.MaxCompactMessages, len(old))]
		in := prompt.CompactInput{PersonaName: p.Name, ChatName: c.Name, IsGroup: isGroupChat(c),
			Previous: text, PreviousProse: prose, Messages: chunk, Zone: time.Local}
		req := prompt.Compact(in)
		if facts {
			req = prompt.CompactFacts(in)
		}
		req.Model = mdl
		cctx, cancel := context.WithTimeout(ctx, compactTimeout)
		resp, err := prov.Chat(cctx, req)
		cancel()
		if err != nil {
			return model.ChatSummary{}, err
		}
		if facts {
			if text, err = prompt.ApplyFacts(text, prose, resp.Text); err != nil {
				return model.ChatSummary{}, errors.New("the model didn't write readable facts")
			}
			prose = false
		} else {
			text = strings.TrimSpace(resp.Text)
		}
		if text == "" {
			return model.ChatSummary{}, errors.New("the model wrote an empty summary")
		}
		if limit := summaryLimit(facts); utf8.RuneCountInString(text) > limit {
			text = clipText(text, limit)
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
	if facts {
		sm.Format = model.SummaryFacts
	}
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
	sys, choice := e.systemOf(c)
	hist = e.shapePrompt(&opts, ctxsys.Clamp(sys.Params, sys.Base), p, hist)
	req := prompt.Compose(p, c, hist, isGroupChat(c), opts)
	out := model.ChatContext{System: req.System, Sections: prompt.ComposeSections(p, c, hist, isGroupChat(c), opts), Messages: make([]model.ContextMessage, 0, len(req.Messages)), Chars: len(req.System),
		HistoryKept: len(hist), Window: bp.HistoryMessages, Instructions: c.Instructions, ContextFrom: c.ContextFrom,
		ContextSystem: choice, SystemName: sys.Name}
	for _, m := range req.Messages {
		out.Messages = append(out.Messages, model.ContextMessage{Role: m.Role, Content: m.Content})
		out.Chars += len(m.Content)
	}
	if all, err := e.store.History(chatKey, 0); err == nil {
		out.HistoryTotal = len(all)
		p := e.compactPlan(c, all)
		out.Compact = model.CompactStatus{Setting: c.CompactEvery, On: p.on, Keep: p.keep, Every: p.every, Fast: p.fast,
			NextIn: max(0, p.keep+p.every-len(fromContext(all, c.ContextFrom)))}
		if p.on {
			out.Window, _ = e.historyCap(c, bp)
		}
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
