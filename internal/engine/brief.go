package engine

import (
	"context"
	"fmt"
	"slices"
	"time"

	"whatsappdoppel/internal/crossctx"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
	"whatsappdoppel/internal/store"
)

// Chat briefs (cross-chat context): every few messages (and a quiet spell) a
// background job writes a short private summary of a chat — topics, what the
// persona promised, the tone and, for groups, a line per member — that other
// chats of the same persona may draw on (cross.go). Only chats that can be a
// source (sharing on, another chat with the same persona exists) get one.

const (
	briefEvery     = 8               // new messages before a brief
	briefIdle      = 3 * time.Minute // …and this long after the last one
	briefTimeout   = 45 * time.Second
	briefMaxTokens = 300
	briefContext   = 12 // the brief always reads at least this many recent messages
)

func briefJob(key string) string { return "brief:" + key }

// isCrossSource reports whether a chat's brief could be used anywhere: the
// feature and its sharing are on, it isn't paused or revealed, and the same
// persona has another chat.
func (e *Engine) isCrossSource(c model.ChatAssignment) bool {
	r := e.crossRules()
	if !crossKeyAllowed(c.Key) || c.PersonaID == "" || e.crossBlocked(c, r) != "" {
		return false
	}
	return slices.ContainsFunc(e.store.Chats(), func(s model.ChatAssignment) bool {
		return s.Key != c.Key && s.PersonaID == c.PersonaID
	})
}

// briefTick counts a new message (theirs or the persona's) and (re)arms the
// brief job once enough have arrived.
func (e *Engine) briefTick(c model.ChatAssignment) {
	if !e.isCrossSource(c) {
		return
	}
	pending := 0
	e.updateState(c.Key, func(st *store.RunnerState) {
		st.BriefPending++
		pending = st.BriefPending
	})
	if pending < briefEvery {
		return
	}
	key := c.Key
	e.schedule(briefJob(key), briefIdle, func(ctx context.Context) {
		cc, ok := e.store.Chat(key)
		if !ok || !e.isCrossSource(cc) {
			return
		}
		if _, err := e.refreshBrief(ctx, cc); err != nil && ctx.Err() == nil {
			e.act(model.ActError, cc, "", "Couldn't update the chat summary for other chats: "+err.Error(), nil)
		}
	})
}

// RefreshBrief writes a chat's brief now (POST /api/chats/{key}/cross/refresh).
// It waits for the background slot.
func (e *Engine) RefreshBrief(ctx context.Context, chatKey string) (model.Brief, error) {
	c, ok := e.store.Chat(chatKey)
	if !ok {
		return model.Brief{}, fmt.Errorf("chat %q: %w", chatKey, ErrNotFound)
	}
	e.cancelJob(briefJob(chatKey))
	var b model.Brief
	var err error
	if !e.withJobSlot(ctx, func() { b, err = e.refreshBrief(ctx, c) }) {
		return b, ctx.Err()
	}
	return b, err
}

// refreshBrief reads the messages since the last brief (plus some context),
// asks the model for the brief, classifies each line and saves it.
func (e *Engine) refreshBrief(ctx context.Context, c model.ChatAssignment) (model.Brief, error) {
	p, ok := e.store.Persona(c.PersonaID)
	if !ok {
		return model.Brief{}, fmt.Errorf("persona %q not found", c.PersonaID)
	}
	all, err := e.store.History(c.Key, 0)
	if err != nil {
		return model.Brief{}, err
	}
	all = slices.DeleteFunc(slices.Clone(all), func(m model.Message) bool {
		return m.Kind == model.MsgKindFix || m.Kind == model.MsgKindReveal
	})
	st := e.store.RunnerState(c.Key)
	start := len(all)
	for i, m := range all {
		if m.TS.After(st.BriefCursor) {
			start = i
			break
		}
	}
	start = min(start, max(0, len(all)-briefContext))
	msgs := all[start:]
	if len(msgs) > prompt.MaxBriefMessages {
		msgs = msgs[len(msgs)-prompt.MaxBriefMessages:]
	}
	now := e.clock.Now()
	isGroup := isGroupChat(c)
	b := model.Brief{ChatKey: c.Key, PersonaID: c.PersonaID, Kind: c.Kind, GeneratedAt: now, MessageCount: len(msgs),
		Topics: []string{}, Commitments: []string{}, People: []model.BriefPerson{}, Sensitive: []string{}}
	if len(msgs) > 0 {
		b.From, b.To = msgs[0].TS, msgs[len(msgs)-1].TS
		prov, mdl, err := e.llm.Resolve(p.LLM)
		if err != nil {
			return b, err
		}
		req := prompt.Brief(prompt.BriefInput{PersonaName: p.Name, ChatName: c.Name, IsGroup: isGroup, Messages: msgs, Zone: time.Local, Now: now})
		req.Model, req.MaxTokens = mdl, briefMaxTokens
		cctx, cancel := context.WithTimeout(ctx, briefTimeout)
		resp, err := prov.Chat(cctx, req)
		cancel()
		if err != nil {
			return b, err
		}
		bc, err := prompt.ParseBrief(resp.Text, isGroup)
		if err != nil {
			return b, err
		}
		b.Topics, b.Commitments, b.Tone = nonNilStrings(bc.Topics), nonNilStrings(bc.Commitments), bc.Tone
		if bc.People != nil {
			b.People = bc.People
		}
		addSens := func(cat string) {
			if cat != "" && !slices.Contains(b.Sensitive, cat) {
				b.Sensitive = append(b.Sensitive, cat)
			}
		}
		for _, s := range bc.Sensitive {
			addSens(s)
		}
		// The keyword classifier runs over the lines and the messages too:
		// a window that touched something sensitive carries only its
		// (individually checked) promises.
		for _, s := range append(append(slices.Clone(b.Topics), b.Commitments...), b.Tone) {
			addSens(crossctx.Classify(s))
		}
		for _, bp := range b.People {
			addSens(crossctx.Classify(bp.Note))
		}
		for _, m := range msgs {
			if m.Speaker == "them" {
				addSens(crossctx.Classify(m.Text))
			}
		}
		// Who each group note is about, by the senders' ids.
		if isGroup {
			for i := range b.People {
				for _, m := range msgs {
					if m.Speaker == "them" && m.SenderJID != "" && crossctx.Matches(crossctx.Item{Person: m.Name}, crossctx.Person{Name: b.People[i].Name}) {
						b.People[i].JID = m.SenderJID
						break
					}
				}
			}
		}
	}
	if err := e.store.SaveBrief(b); err != nil {
		return b, err
	}
	cursor := b.To
	e.updateState(c.Key, func(s *store.RunnerState) {
		s.BriefPending = 0
		if cursor.After(s.BriefCursor) {
			s.BriefCursor = cursor
		}
	})
	if e.hub != nil {
		e.hub.Publish(events.TypeMemoriesChanged, map[string]string{"chatKey": c.Key})
	}
	return b, nil
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// forgetBrief drops a chat's brief and its counters (hand-off, reveal,
// cleared history).
func (e *Engine) forgetBrief(key string) {
	e.cancelJob(briefJob(key))
	_ = e.store.DeleteBrief(key)
	e.updateState(key, func(s *store.RunnerState) { s.BriefPending = 0 })
}
