package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/core/store"
	"whatsappdoppel/internal/runtime/prompt"
)

// The digest (a context system with background "digest"): one call every
// DigestEvery messages from them does the background jobs that would
// otherwise each make their own call — memories, the brief its other chats
// may read and a secretary's notes — on the same messages, with a strict
// format; each part is checked and saved by the job's own code. Folding
// into the long-term summary (context.go) stays on its own schedule.

const (
	digestIdle      = 20 * time.Second
	digestTimeout   = 120 * time.Second
	digestMaxTokens = 900
)

func digestJob(key string) string { return "digest:" + key }

// digestTick counts a message from them and arms the digest when enough
// have come in.
func (e *Engine) digestTick(c model.ChatAssignment) {
	every := e.paramsOf(c).DigestEvery
	pending := 0
	e.updateState(c.Key, func(st *store.RunnerState) {
		st.DigestPending++
		pending = st.DigestPending
	})
	if pending < every {
		return
	}
	key := c.Key
	e.schedule(digestJob(key), digestIdle, func(ctx context.Context) {
		cc, ok := e.store.Chat(key)
		if !ok || !cc.Enabled {
			return
		}
		if err := e.digest(ctx, cc); err != nil && ctx.Err() == nil {
			e.act(model.ActError, cc, "", "Couldn't do the background digest: "+err.Error(), nil)
		}
	})
}

// digest runs one digest call for a chat.
func (e *Engine) digest(ctx context.Context, c model.ChatAssignment) error {
	p, ok := e.store.Persona(c.PersonaID)
	if !ok {
		return fmt.Errorf("persona %q not found", c.PersonaID)
	}
	defer e.updateState(c.Key, func(st *store.RunnerState) { st.DigestPending = 0 })
	now := e.clock.Now()
	in := prompt.DigestInput{PersonaName: p.Name, ChatName: c.Name, IsGroup: isGroupChat(c), Zone: time.Local, Now: now}

	var memMsgs, briefMsgs []model.Message
	if e.memoryOn(c) {
		msgs, known, err := e.memoryBatch(c, prompt.MaxDigestMessages)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(msgs, func(m model.Message) bool { return m.Speaker == "them" }) {
			in.Memories, in.Known, memMsgs = true, known, msgs
		} else {
			e.memoryDone(c.Key, msgs)
		}
	}
	if e.isCrossSource(c) {
		msgs, err := e.briefBatch(c, prompt.MaxDigestMessages)
		if err != nil {
			return err
		}
		if len(msgs) > 0 {
			in.Brief, briefMsgs = true, msgs
		}
	}
	_, sec := e.secretaryOf(c)
	var nb noteBatch
	if sec != nil && (sec.TakeMessages || sec.FlagUrgent) {
		var err error
		if nb, err = e.notesBatch(c); err != nil {
			return err
		}
		if !nb.done {
			in.Notes, in.Owner, in.Open = true, sec.OwnerName, nb.open
		}
	}
	if !in.Memories && !in.Brief && !in.Notes {
		return nil
	}
	// One window covering every part (the longest one), oldest first.
	in.Messages = longest(memMsgs, briefMsgs, nb.read)
	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err != nil {
		return err
	}
	req := prompt.Digest(in)
	req.Model, req.MaxTokens = mdl, digestMaxTokens
	var mem, notes, brief string
	for try := 0; ; try++ { // a strict format, one retry
		cctx, cancel := context.WithTimeout(ctx, digestTimeout)
		resp, err := prov.Chat(cctx, req)
		cancel()
		if err != nil {
			return err
		}
		if mem, notes, brief, err = prompt.DigestParts(resp.Text); err == nil {
			break
		}
		if try == 1 || ctx.Err() != nil {
			return errors.New("the model didn't answer in the digest format")
		}
		req.System += prompt.DigestRetryNote
	}
	var errs []error
	if in.Memories && mem != "" {
		if found, err := prompt.ParseMemories(mem); err == nil {
			_, err = e.saveMemories(c, p, memMsgs, found, now)
			errs = append(errs, err)
		}
	}
	if in.Brief && brief != "" {
		if bc, err := prompt.ParseBrief(brief, isGroupChat(c)); err == nil {
			_, err = e.saveBrief(c, briefMsgs, &bc)
			errs = append(errs, err)
		}
	}
	if in.Notes && notes != "" {
		if taken, err := prompt.ParseNotes(notes); err == nil {
			errs = append(errs, e.saveNotes(c, p, sec, nb, taken))
		}
	}
	return errors.Join(errs...)
}

func longest(lists ...[]model.Message) []model.Message {
	var out []model.Message
	for _, l := range lists {
		if len(l) > len(out) {
			out = l
		}
	}
	return out
}
