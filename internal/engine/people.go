package engine

import (
	"fmt"
	"strings"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/guard"
	"whatsappdoppel/internal/mention"
	"whatsappdoppel/internal/model"
)

// Decision reasons of the "who it answers" rules (activity meta "reason").
const (
	ReasonMuteWord       = "mute_word"
	ReasonNotSelected    = "not_selected"
	ReasonPriorityPerson = "priority_person"
	ReasonTriggerWord    = "trigger_word"
	ReasonStreakLimit    = "streak_limit"
)

// streakQuiet resets the back-and-forth counter after this much silence.
const streakQuiet = 30 * time.Minute

// streakState counts replies in a row to one person while nobody else speaks
// (guards against two bots answering each other forever).
type streakState struct {
	peer  string // who the persona is going back and forth with ("" = nobody)
	count int    // replies to peer since anyone else spoke
	last  time.Time
}

// streakKey identifies the sender of an incoming message for the streak:
// the user part of the sender in groups, the chat itself in DMs.
func streakKey(c model.ChatAssignment, in model.Incoming) string {
	if !in.IsGroup && c.Kind != "group" {
		return "chat"
	}
	if u := mention.User(in.SenderJID); u != "" {
		return u
	}
	if n := strings.TrimSpace(in.PushName); n != "" {
		return "name:" + strings.ToLower(n)
	}
	return "someone"
}

// noteSpeakerLocked records that someone other than the persona wrote: a new
// person (or a long silence) starts the count over; "" = you wrote.
func (r *Runner) noteSpeakerLocked(key string, now time.Time) {
	if key == "" || key != r.streak.peer || now.Sub(r.streak.last) > streakQuiet {
		r.streak = streakState{peer: key}
	}
	r.streak.last = now
}

// noteReplyLocked counts a reply in the current back-and-forth.
func (r *Runner) noteReplyLocked(now time.Time) {
	if r.streak.peer != "" {
		r.streak.count++
		r.streak.last = now
	}
}

type gateAction int

const (
	gateNormal gateAction = iota // usual rules (group decision, reply chance)
	gateSkip                     // don't answer; keep the message as context
	gateForce                    // answer without the chance/AI rolls
)

type gateResult struct {
	action gateAction
	text   string
	meta   map[string]any
}

// peopleGate applies, in order: mute words, who the group answers (with
// "still answer anyone who addresses it"), priority people, trigger words and
// the streak guard. raw is the message text before tag humanising.
func (r *Runner) peopleGate(c model.ChatAssignment, bp model.BehaviorProfile, p model.Persona, in model.Incoming, raw string, isGroup bool, roster []model.Participant, now time.Time) gateResult {
	who := strings.TrimSpace(in.PushName)
	if who == "" {
		who = "them"
	}
	if w := behavior.MatchWord(raw, bp.MuteWords); w != "" {
		return gateResult{gateSkip, fmt.Sprintf("Not answering — the message contains the mute word %q", w),
			map[string]any{"reason": ReasonMuteWord, "word": w}}
	}
	if isGroup {
		ids := personIDs(in.SenderJID, roster)
		ok, src := behavior.Answerable(c.People, len(roster), bp.RespondToAllMaxMembers, ids...)
		if !ok && !(bp.AnswerAnyoneWhoAddressesIt && r.e.addressesMe(p, raw, in.MentionedJIDs)) {
			text := "Not answering " + who + " — this group only answers people you picked"
			if src == behavior.SourcePerson {
				text = "Not answering " + who + " — set not to answer them in this chat"
			}
			return gateResult{gateSkip, text, map[string]any{"reason": ReasonNotSelected, "sender": in.PushName}}
		}
		if i := behavior.FindPerson(c.People.People, ids...); i >= 0 && c.People.People[i].Priority {
			return gateResult{gateForce, "Answering " + who + " — always replies to them",
				map[string]any{"reason": ReasonPriorityPerson, "sender": in.PushName}}
		}
	}
	if w := behavior.MatchWord(raw, bp.TriggerWords); w != "" {
		return gateResult{gateForce, fmt.Sprintf("Answering — the message contains the trigger word %q", w),
			map[string]any{"reason": ReasonTriggerWord, "word": w}}
	}
	if bp.MaxStreak > 0 {
		r.mu.Lock()
		st := r.streak
		r.mu.Unlock()
		if st.peer == streakKey(c, in) && st.count >= bp.MaxStreak && now.Sub(st.last) <= streakQuiet {
			to := ""
			if isGroup {
				to = " to " + who
			}
			return gateResult{gateSkip, fmt.Sprintf("Taking a break — %d replies in a row%s with nobody else joining in", st.count, to),
				map[string]any{"reason": ReasonStreakLimit, "limit": bp.MaxStreak}}
		}
	}
	return gateResult{action: gateNormal}
}

// personIDs lists every address of the sender known from the roster (the
// group may report a lid while preferences were saved with a phone, or the
// other way round).
func personIDs(sender string, roster []model.Participant) []string {
	ids := []string{sender}
	u := mention.User(sender)
	for _, p := range roster {
		if u != "" && (mention.User(p.JID) == u || p.Phone == u || mention.User(p.LID) == u) {
			ids = append(ids, p.JID, p.LID)
			if p.Phone != "" {
				ids = append(ids, p.Phone+"@s.whatsapp.net")
			}
		}
	}
	return ids
}

// addressesMe reports whether a message says the persona's name or @tags
// your account.
func (e *Engine) addressesMe(p model.Persona, text string, mentions []string) bool {
	if guard.ContainsName(text, p.Name) {
		return true
	}
	own := map[string]bool{}
	for _, j := range e.ownJIDs() {
		own[mention.User(j)] = true
	}
	for _, m := range mentions {
		if u := mention.User(m); u != "" && own[u] {
			return true
		}
	}
	return false
}

// keepAsContext stores a message the persona won't answer so later replies
// still know about it (prompt-injection attempts are dropped).
func (r *Runner) keepAsContext(in model.Incoming, text string, mentions []string, bp model.BehaviorProfile, now time.Time) {
	g := guard.Inspect(text, bp.InjectionFilter)
	if g.Blocked || g.Text == "" {
		return
	}
	msg := newMessage(in, "them", g.Text, now)
	msg.Mentions = mentions
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	msg = r.appendLocked(msg, bp)
	c := r.cfg
	r.mu.Unlock()
	r.persist(msg, bp)
	r.e.memoryTick(c) // memory.go
}
