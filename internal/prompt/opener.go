package prompt

import (
	"fmt"
	"strings"
	"time"

	"whatsappdoppel/internal/goals"
	"whatsappdoppel/internal/model"
)

// Opener tunes Initiate: a check-in or a manual "start a conversation".
// The zero value keeps the classic check-in note (InitiateNote).
type Opener struct {
	// Manual means the account owner asked for an opener right now.
	Manual bool
	// Hint is what the opener should be about (the owner's words, trusted).
	Hint string
	// Silence is how long the chat has been quiet (0 = unknown).
	Silence time.Duration
}

func (o Opener) isZero() bool { return !o.Manual && strings.TrimSpace(o.Hint) == "" && o.Silence == 0 }

// MaxOpenerHint caps the hint (runes).
const MaxOpenerHint = 300

// initiateNote is the instruction appended to the system prompt by Initiate:
// aware of whether the chat is new, how long it has been quiet, groups, the
// owner's hint and the private agenda (the opener may be its first small
// step, never revealing it).
func initiateNote(p model.Persona, chat model.ChatAssignment, history []model.Message, isGroup bool, opts ...Options) string {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	op := o.Opener
	if op.isZero() {
		return InitiateNote
	}
	var b strings.Builder
	switch {
	case len(history) == 0 && isGroup:
		b.WriteString("\n\nYou're starting a conversation in this group — there are no recent messages. Write one short, natural message a real friend would send to the group.")
	case len(history) == 0:
		b.WriteString("\n\nYou're starting the conversation — there are no recent messages. Write one short, natural first message a real friend would send.")
	case op.Silence >= 20*time.Hour:
		fmt.Fprintf(&b, "\n\nIt has been %s since the last message. Write one short, natural opener a real friend would send to restart the conversation — don't mention the silence unless it fits.", humanSince(op.Silence))
	case op.Silence > 0:
		fmt.Fprintf(&b, "\n\nThe chat has gone quiet for %s. Write one short, natural message to pick it back up — it can follow up on what you last talked about or bring up something new.", humanSince(op.Silence))
	default:
		b.WriteString("\n\nWrite one short, natural message to start talking again, like a real friend would.")
	}
	if isGroup {
		b.WriteString(" Talk to the group, not to one person, unless your topic is about someone.")
	}
	b.WriteString(" Don't explain why you're writing.")
	if h := strings.TrimSpace(op.Hint); h != "" {
		if r := []rune(h); len(r) > MaxOpenerHint {
			h = string(r[:MaxOpenerHint])
		}
		b.WriteString(" Open with this (your own idea — never say someone suggested it): " + oneLine(h, MaxOpenerHint) + ".")
	}
	g := goals.Resolve(p, chat)
	if strings.TrimSpace(g.Text) != "" && !o.Goal.Off && !g.Relaxed(o.Goal.Reached) {
		b.WriteString(" If it fits, make it a natural first small step towards your private agenda — without revealing it.")
	}
	return b.String()
}

// humanSince renders a silence length: "about an hour", "5 hours", "a day", "3 days".
func humanSince(d time.Duration) string {
	switch h := int(d / time.Hour); {
	case h < 2:
		return "about an hour"
	case h < 24:
		return fmt.Sprintf("%d hours", h)
	case h < 48:
		return "a day"
	default:
		return fmt.Sprintf("%d days", h/24)
	}
}
