package prompt

import (
	"fmt"
	"strings"
	"time"

	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
)

// ─── What the persona reads: instructions, summary, compaction ─

// InstructionsSection carries your own instructions for one chat (Options.
// Instructions). They come after the goal and before STAY YOURSELF: they
// steer what it does here, never who it is.
func InstructionsSection(text string) string {
	if text = strings.TrimSpace(text); text == "" {
		return ""
	}
	return "\n\nINSTRUCTIONS FOR THIS CHAT (from the person who runs you; follow them, they don't change who you are): " + text
}

// SummarySection is the summary of the older part of the chat, inside the
// CHAT CONTEXT block (Options.Summary).
func SummarySection(text string) string {
	if text = strings.TrimSpace(text); text == "" {
		return ""
	}
	return "\nEARLIER IN THIS CHAT (a summary of older messages you no longer see): " + text
}

// MaxCompactMessages caps the messages one compaction reads.
const MaxCompactMessages = 120

// CompactInput is what a compaction summary is written from.
type CompactInput struct {
	PersonaName string
	ChatName    string
	IsGroup     bool
	Previous    string          // the summary so far ("" = none)
	Messages    []model.Message // the messages to fold in, oldest first
	Zone        *time.Location
}

// Compact builds the request that folds older messages into the running
// summary (plain text, at most ~150 words). The caller sets Model.
func Compact(in CompactInput) llm.Request {
	zone := in.Zone
	if zone == nil {
		zone = time.Local
	}
	name := memOr(in.PersonaName, "the persona")
	where := "a private WhatsApp chat with " + memOr(in.ChatName, "a friend")
	if in.IsGroup {
		where = `the WhatsApp group "` + memOr(in.ChatName, "group") + `"`
	}
	var sys strings.Builder
	fmt.Fprintf(&sys, "You keep a running summary of %s for %s, who is in it and can no longer see the older messages.\n\n", where, name)
	sys.WriteString("Rules:\n")
	sys.WriteString("- Only what is in the summary so far and the messages. Never guess.\n")
	sys.WriteString("- Keep what matters later: who is who, plans and dates, what was agreed or promised, running jokes, open questions.\n")
	fmt.Fprintf(&sys, "- Write about %s in the third person by name.\n", name)
	sys.WriteString("- Plain sentences, at most 150 words, no lists or headings, no quotes of whole messages.\n")

	var u strings.Builder
	if p := strings.TrimSpace(in.Previous); p != "" {
		u.WriteString("Summary so far:\n" + p + "\n\n")
	}
	u.WriteString("Messages to add (oldest first):\n")
	msgs := in.Messages
	if len(msgs) > MaxCompactMessages {
		msgs = msgs[len(msgs)-MaxCompactMessages:]
	}
	contact := memOr(in.ChatName, "Them")
	for _, m := range visible(msgs) {
		label := m.Name
		switch {
		case m.Speaker == "me":
			label = name
		case label == "" && !in.IsGroup:
			label = contact
		case label == "":
			label = "Someone"
		}
		ts := ""
		if !m.TS.IsZero() {
			ts = "[" + m.TS.In(zone).Format("Mon 15:04") + "] "
		}
		u.WriteString(ts + oneLine(label, 40) + ": " + oneLine(m.Text, 300) + "\n")
	}
	u.WriteString("\nWrite the updated summary now, plain text only.")
	return llm.Request{
		System:      sys.String(),
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: u.String()}},
		Temperature: 0.2,
		MaxTokens:   400,
	}
}
