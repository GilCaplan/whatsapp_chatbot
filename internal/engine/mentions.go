package engine

import (
	"context"
	"errors"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/mention"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
	"whatsappdoppel/internal/store"
)

// rosterTimeout bounds a group member lookup (cached by the wa layer).
const rosterTimeout = 5 * time.Second

// tagReplySpeakers is how many recent messages are checked for "several
// people are talking" before tagging the person a reply answers.
const tagReplySpeakers = 10

func isGroupChat(c model.ChatAssignment) bool { return c.Kind == "group" }

func (e *Engine) ownJIDs() []string {
	if e.wa == nil {
		return nil
	}
	return e.wa.OwnJIDs()
}

// roster returns a group's members; nil for DMs or when WhatsApp can't tell
// (tagging and the size rule then degrade gracefully).
func (e *Engine) roster(ctx context.Context, c model.ChatAssignment) []model.Participant {
	if !isGroupChat(c) || e.wa == nil {
		return nil
	}
	jid := c.JID
	if jid == "" {
		jid = c.Key
	}
	ctx, cancel := context.WithTimeout(ctx, rosterTimeout)
	defer cancel()
	ps, err := e.wa.GroupParticipants(ctx, jid)
	if err != nil {
		return nil
	}
	return ps
}

// directoryFrom builds the taggable members of a group from its roster plus
// people seen in hist (fallback when the roster is unavailable or they left).
func (e *Engine) directoryFrom(roster []model.Participant, hist []model.Message) *mention.Directory {
	d := mention.NewDirectory(roster, e.ownJIDs())
	for i := len(hist) - 1; i >= 0; i-- {
		if m := hist[i]; m.Speaker == "them" && m.SenderJID != "" {
			d.AddSeen(m.SenderJID, m.Name)
		}
	}
	return d
}

// directory is directoryFrom with a fresh roster; nil for DMs.
func (e *Engine) directory(ctx context.Context, c model.ChatAssignment, hist []model.Message) *mention.Directory {
	if !isGroupChat(c) {
		return nil
	}
	return e.directoryFrom(e.roster(ctx, c), hist)
}

// recentSpeakers lists the JIDs of the people who wrote in hist, most recent first.
func recentSpeakers(hist []model.Message) []string {
	var out []string
	seen := map[string]bool{}
	for i := len(hist) - 1; i >= 0; i-- {
		m := hist[i]
		if m.Speaker != "them" || m.SenderJID == "" {
			continue
		}
		if u := mention.User(m.SenderJID); !seen[u] {
			seen[u] = true
			out = append(out, m.SenderJID)
		}
	}
	return out
}

// distinctSpeakers counts the different people among the last n messages.
func distinctSpeakers(hist []model.Message, n int) int {
	return len(recentSpeakers(hist[max(0, len(hist)-n):]))
}

// promptOptions builds the per-chat prompt options: length bias, the group's
// members for tagging, the owner's notes about people, memories and what
// the persona knows from its other chats (the *crossUse goes to the leak
// guard; nil when there is none).
func (e *Engine) promptOptions(c model.ChatAssignment, bp model.BehaviorProfile, d *mention.Directory, hist []model.Message) (prompt.Options, *crossUse) {
	o := prompt.Options{LengthBias: bp.LengthBias}
	// Realism (wave 3): the time where the persona lives, and what it
	// remembers about the people here (routine.go, memory.go).
	o.Now, o.MacZone = e.clock.Now(), time.Local
	o.Memories = e.memoryLines(c, hist, o.Now)
	cross := e.crossContext(c, d, hist, o.Now) // cross.go
	if cross.active() {
		o.Cross = cross.Input
	}
	if isGroupChat(c) {
		if d != nil {
			o.Participants = d.Names(recentSpeakers(hist), prompt.MaxParticipants)
		}
		o.AllowMentions, o.MentionMax = bp.AllowMentions, bp.MentionMax
		for _, pp := range c.People.People {
			if pp.Notes == "" {
				continue
			}
			name := pp.Name
			if en, ok := d.ByJID(pp.JID); ok {
				name = en.Display
			}
			if name = mention.CleanName(name); name != "" {
				o.PeopleNotes = append(o.PeopleNotes, prompt.PersonNote{Name: name, Notes: pp.Notes})
			}
		}
		return o, cross
	}
	if i := behavior.FindPerson(c.People.People, c.JID, c.AltJID); i >= 0 {
		o.ContactNote = c.People.People[i].Notes
	} else if len(c.People.People) == 1 {
		o.ContactNote = c.People.People[0].Notes
	}
	return o, cross
}

// applyMentions validates the tags of a generated group reply (Normalize)
// and, when several people are talking, sometimes addresses the person it
// answers with "@Name" (tagReplyPercent). It returns the display text and
// the people tagged.
func (e *Engine) applyMentions(text string, d *mention.Directory, bp model.BehaviorProfile, answered pendingMsg, hist []model.Message, proactive bool) (string, []mention.Entry) {
	if d == nil {
		return text, nil
	}
	out, tagged := mention.Normalize(text, d, mention.Policy{Allow: bp.AllowMentions, Max: bp.MentionMax})
	if !bp.AllowMentions || bp.TagReplyPercent <= 0 || proactive || answered.SenderJID == "" || len(tagged) >= bp.MentionMax {
		return out, tagged
	}
	if distinctSpeakers(hist, tagReplySpeakers) < 2 {
		return out, tagged
	}
	who, ok := d.ByJID(answered.SenderJID)
	if !ok || !e.rng.Hit(bp.TagReplyPercent) {
		return out, tagged
	}
	if t, ok := mention.TagFirst(out, who, tagged); ok {
		out, tagged = t, append([]mention.Entry{who}, tagged...)
	}
	return out, tagged
}

// humanizeIncoming rewrites "@<number>" tags in an incoming group message to
// "@Name" for history, activity and the LLM; the decision logic still sees
// the raw text.
func (e *Engine) humanizeIncoming(roster []model.Participant, hist []model.Message, in model.Incoming, text, selfName string) (string, []string) {
	if !in.IsGroup || len(in.MentionedJIDs) == 0 {
		return text, nil
	}
	return mention.Humanize(text, in.MentionedJIDs, e.directoryFrom(roster, hist), e.ownJIDs(), selfName)
}

// sendText tries each target in order (phone ↔ lid fallback) and returns the
// one that worked plus the text as sent. Tags in text ("@Dana") are encoded
// for WhatsApp with d (nil = no tags); a non-nil quote sends a quoted reply.
func (e *Engine) sendText(ctx context.Context, jids []string, text string, quote *model.QuoteRef, d *mention.Directory) (jid, wire string, err error) {
	jid, wire, _, err = e.sendTextResult(ctx, jids, text, quote, d)
	return jid, wire, err
}

// sendTextResult is sendText plus WhatsApp's SendResult (the message id, used
// to edit the bubble later and kept as Message.WAID).
func (e *Engine) sendTextResult(ctx context.Context, jids []string, text string, quote *model.QuoteRef, d *mention.Directory) (jid, wire string, res model.SendResult, err error) {
	if len(jids) == 0 {
		return "", "", res, errors.New("chat has no WhatsApp address")
	}
	msg := model.OutMessage{Text: text, Quote: quote}
	if d != nil {
		msg.Text, msg.Mentions = mention.Encode(text, d)
	}
	for _, j := range jids {
		if res, err = e.wa.SendMessage(ctx, j, msg); err == nil {
			return j, msg.Text, res, nil
		}
		if ctx.Err() != nil {
			return "", "", model.SendResult{}, ctx.Err()
		}
	}
	return "", "", model.SendResult{}, err
}

// recordSent appends a persona message (display text and tagged names) to
// history (runner or store) and touches the chat. wire is the text as sent,
// remembered to recognise its echo; crossUsed see Message.CrossUsed.
func (e *Engine) recordSent(r *Runner, c model.ChatAssignment, text, wire string, mentions, crossUsed []string) {
	msg := model.Message{ID: store.NewID(), TS: e.clock.Now(), Speaker: "me", Text: text, FromBot: true, Mentions: mentions, CrossUsed: crossUsed}
	if r != nil {
		r.recordSent(msg, wire)
	} else {
		_ = e.store.AppendHistory(c.Key, msg, e.effective(c).Profile.HistoryMessages)
	}
	e.store.TouchChat(c.Key, time.Now())
	e.briefTick(c) // brief.go
}

// bubbleMentions lists the people a bubble tags (display names), per d.
func bubbleMentions(text string, d *mention.Directory) []string {
	if d == nil {
		return nil
	}
	_, tagged := mention.Normalize(text, d, mention.Policy{Allow: true, Max: d.Len()})
	return mention.Names(tagged)
}

// chatHistory is the chat's recent history (runner memory, else the store).
func (e *Engine) chatHistory(r *Runner, c model.ChatAssignment) []model.Message {
	if r != nil {
		return r.historySnapshot()
	}
	h, _ := e.store.History(c.Key, e.effective(c).Profile.HistoryMessages)
	return h
}

// previewOptions are the prompt options for PromptPreview (no chat = defaults).
func (e *Engine) previewOptions(c model.ChatAssignment) prompt.Options {
	bp := e.effective(c).Profile
	if c.Key == "" {
		return prompt.Options{LengthBias: bp.LengthBias}
	}
	hist := e.chatHistory(e.runner(c.Key), c)
	o, _ := e.promptOptions(c, bp, e.directory(e.ctx, c, hist), hist)
	return o
}

// simulatedSender picks who a simulated group message comes from: senderJID
// when it is a member, else a random member (never you), else "Tester".
func (e *Engine) simulatedSender(c model.ChatAssignment, senderJID string) (name, jid string) {
	var others []model.Participant
	for _, p := range e.roster(e.ctx, c) {
		if !p.IsSelf {
			others = append(others, p)
		}
	}
	if senderJID != "" {
		u := mention.User(senderJID)
		for _, p := range others {
			if mention.User(p.JID) == u || p.Phone == u || mention.User(p.LID) == u {
				return cmpName(p), p.JID
			}
		}
	}
	if len(others) == 0 {
		return "Tester", ""
	}
	p := others[e.rng.Index(len(others))]
	return cmpName(p), p.JID
}

func cmpName(p model.Participant) string {
	if n := mention.CleanName(p.Name); n != "" {
		return n
	}
	if p.Phone != "" {
		return "+" + p.Phone
	}
	return "Someone"
}

// lastFromThem is the most recent message by someone else, as the message a
// regenerated reply answers.
func lastFromThem(hist []model.Message) pendingMsg {
	for i := len(hist) - 1; i >= 0; i-- {
		if m := hist[i]; m.Speaker == "them" {
			return pendingMsg{ID: m.ID, SenderJID: m.SenderJID, Name: m.Name, Text: m.Text}
		}
	}
	return pendingMsg{}
}

// mentionMeta adds "mentions" (display names) to activity meta.
func mentionMeta(m map[string]any, names []string) map[string]any {
	if len(names) > 0 {
		if m == nil {
			m = map[string]any{}
		}
		m["mentions"] = names
	}
	return m
}
