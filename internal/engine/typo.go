package engine

import (
	"context"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/mention"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

// Typos (wave 3, Engineer A; WAVE3_PLAN §3.4). Now and then (typoPercent)
// one bubble of an automatic reply goes out with a small slip; then,
// depending on typoFixStyle, the persona sends a "*word" correction, edits
// the message on WhatsApp, or leaves it. Never in approved replies, trigger
// replies, manual sends or reveals. History keeps what was meant
// (Message.Corrected) so the model never learns to make typos.

// planTypo decides whether this delivery gets a typo and in which bubble
// (-1 = none).
func (r *Runner) planTypo(cyc *cycle, c model.ChatAssignment, bubbles []behavior.Bubble) (int, behavior.Typo) {
	if cyc.immediate || len(bubbles) == 0 {
		return -1, behavior.Typo{}
	}
	bp := r.e.effective(c).Profile
	if bp.TypoPercent <= 0 || !r.e.rng.Hit(bp.TypoPercent) {
		return -1, behavior.Typo{}
	}
	start := r.e.rng.Index(len(bubbles))
	for k := range bubbles {
		i := (start + k) % len(bubbles)
		if t, ok := behavior.MakeTypo(bubbles[i].Text, r.e.rng); ok {
			return i, t
		}
	}
	return -1, behavior.Typo{}
}

// recordSentBubble appends a sent persona bubble to history: text is what
// went out, intended what was meant (≠ text after a typo), kind "" or
// "fix"; crossUsed names the people whose context from other chats
// informed it. It returns the history message id.
func (e *Engine) recordSentBubble(r *Runner, c model.ChatAssignment, text, intended, wire string, mentions []string, waID, kind string, crossUsed []string) string {
	msg := model.Message{ID: store.NewID(), TS: e.clock.Now(), Speaker: "me", Text: text, FromBot: true, Mentions: mentions, WAID: waID, Kind: kind, CrossUsed: crossUsed}
	if intended != "" && intended != text {
		msg.Corrected = intended
	}
	if r != nil {
		r.recordSent(msg, wire)
	} else {
		_ = e.store.AppendHistory(c.Key, msg, e.effective(c).Profile.HistoryMessages)
	}
	e.store.TouchChat(c.Key, time.Now())
	if kind == "" {
		e.briefTick(c) // the persona's own words feed the brief (brief.go)
	}
	return msg.ID
}

// updateHistory changes one message in the runner's memory and on disk.
func (r *Runner) updateHistory(id string, fn func(*model.Message)) {
	r.mu.Lock()
	for i := range r.history {
		if r.history[i].ID == id {
			fn(&r.history[i])
			break
		}
	}
	r.mu.Unlock()
	_ = r.e.store.UpdateHistory(r.key, id, fn)
}

// fixTypo follows a bubble sent with a typo: a "*word" correction, an edit,
// or nothing (typoFixStyle). false when the cycle was cancelled meanwhile.
func (r *Runner) fixTypo(cyc *cycle, c model.ChatAssignment, p model.Persona, t behavior.Typo, jid, waID, msgID, intended string, dir *mention.Directory, typingOn bool) bool {
	e := r.e
	bp := e.effective(c).Profile
	style := bp.TypoFixStyle
	if style == "none" {
		return true
	}
	if style == "edit" && waID != "" {
		if !e.sleep(cyc.ctx, e.rng.Between(2, 6)) {
			return false
		}
		msg := model.OutMessage{Text: intended}
		if dir != nil {
			msg.Text, msg.Mentions = mention.Encode(intended, dir)
		}
		ctx, cancel := context.WithTimeout(cyc.ctx, 10*time.Second)
		err := e.wa.EditMessage(ctx, jid, waID, msg)
		cancel()
		if err == nil {
			r.updateHistory(msgID, func(m *model.Message) {
				m.Text, m.Corrected, m.Kind = intended, "", model.MsgKindEdited
			})
			e.act(model.ActSent, c, p.Name, intended, map[string]any{"edited": true, "jid": jid, "typo": t.Wrong})
			return true
		}
		if cyc.ctx.Err() != nil {
			return false
		}
		// Editing failed (old phone, network): correct it the classic way.
	}
	if !e.sleep(cyc.ctx, e.rng.Between(1, 4)) {
		return false
	}
	fix := behavior.FixBubble(t)
	if typingOn {
		r.setTyping(cyc, true)
	}
	if !r.typeFor(cyc, behavior.TypingDuration(bp, e.rng, utf8.RuneCountInString(fix)), typingOn) {
		return false
	}
	_, wire, res, err := e.sendTextResult(cyc.ctx, []string{jid}, fix, nil, nil)
	if typingOn {
		r.setTyping(nil, false)
	}
	if err != nil {
		return cyc.ctx.Err() == nil // the reply itself went out; a lost correction is fine
	}
	e.recordSentBubble(r, c, fix, "", wire, nil, res.ID, model.MsgKindFix, nil)
	e.act(model.ActSent, c, p.Name, fix, map[string]any{"fix": true, "jid": jid, "typo": t.Wrong})
	return true
}
