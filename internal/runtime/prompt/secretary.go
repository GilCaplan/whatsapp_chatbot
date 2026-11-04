package prompt

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/runtime/llm"
)

// ─── Personal secretary ──────────────────────────────────────
//
// A persona with a Secretary block works as the owner's assistant: open
// about it, taking messages, flagging urgent ones, answering FAQs and
// helping find meeting times (a Find a time task, engine/task.go: it only
// ever sees the times it may offer, never the calendar).

// SecretarySection is the secretary's job, right after the character.
func SecretarySection(s *model.SecretaryConfig) string {
	if s == nil {
		return ""
	}
	owner := memOr(s.OwnerName, "the owner")
	var b strings.Builder
	fmt.Fprintf(&b, "\n\nYOUR JOB: you are %s's personal assistant on WhatsApp. Always be open that you are %s's assistant (for example \"this is %s's assistant\"); never pretend to be %s, and never claim to be human if someone sincerely asks.", owner, owner, owner, owner)
	if s.Tone == "formal" {
		b.WriteString(" Be polite and professional: full sentences, no slang.")
	} else {
		b.WriteString(" Be warm and friendly, short and natural.")
	}
	fmt.Fprintf(&b, "\n- %s isn't answering personally right now; you look after the messages.", owner)
	if s.TakeMessages {
		fmt.Fprintf(&b, "\n- When someone wants something from %s, take a message: make sure you know who they are, what it is about, how urgent it is and how or when to get back to them. Ask for what is missing, one question at a time, then confirm you'll pass it on. Never promise what %s will do.", owner, owner)
	}
	if s.FlagUrgent {
		fmt.Fprintf(&b, "\n- If it's urgent, say you'll flag it to %s right away.", owner)
	}
	if s.AnswerFAQs {
		if len(s.FAQs) > 0 {
			b.WriteString("\n- You may answer these questions with these answers:")
			for _, f := range s.FAQs {
				fmt.Fprintf(&b, "\n  Q: %s A: %s", oneLine(f.Q, 200), oneLine(f.A, 600))
			}
		}
		fmt.Fprintf(&b, "\n- For anything else about %s you don't know from here, say you'll check with %s.", owner, owner)
	}
	if sh := strings.TrimSpace(s.Shareable); sh != "" {
		fmt.Fprintf(&b, "\n- Things you may tell people: %s", oneLine(sh, 1000))
	}
	if aw := strings.TrimSpace(s.AwayMessage); aw != "" {
		fmt.Fprintf(&b, "\n- %s's away message, if it fits: %s", owner, oneLine(aw, 500))
	}
	if s.Meetings {
		fmt.Fprintf(&b, "\n- You can help find a time to meet %s (a meeting is about %d minutes). Suggest times only when YOUR TASK RIGHT NOW below gives them; otherwise take the request and say you'll find a time. Never confirm a meeting unless that task says it is set.", owner, s.MeetingMinutes)
	}
	fmt.Fprintf(&b, "\n- Never share %s's private details, whereabouts, plans or what is in the calendar, beyond what is allowed above.", owner)
	if nv := strings.TrimSpace(s.Never); nv != "" {
		fmt.Fprintf(&b, "\n- Never: %s", oneLine(nv, 1000))
	}
	return b.String()
}

// ─── Taking messages ─────────────────────────────────────────

// MaxNoteMessages caps the messages one note extraction reads.
const MaxNoteMessages = 40

// NotesInput is what new notes are read from.
type NotesInput struct {
	Owner     string
	Assistant string
	ChatName  string
	IsGroup   bool
	Messages  []model.Message // the new messages, oldest first (with a little context)
	Open      []string        // notes already taken in this chat (don't repeat them)
	Zone      *time.Location
	Now       time.Time
}

// TakenNote is one parsed note.
type TakenNote struct {
	From, Text, Callback, Meeting string
	Urgent                        bool
}

var notesSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "notes": {"type": "array", "items": {"type": "object", "properties": {
      "from": {"type": "string"}, "text": {"type": "string"}, "urgent": {"type": "boolean"},
      "callback": {"type": "string"}, "meeting": {"type": "string"}
    }, "required": ["from", "text", "urgent", "callback", "meeting"], "additionalProperties": false}}
  },
  "required": ["notes"],
  "additionalProperties": false
}`)

// Notes builds the request that picks out messages left for the owner.
func Notes(in NotesInput) llm.Request {
	zone := in.Zone
	if zone == nil {
		zone = time.Local
	}
	owner := memOr(in.Owner, "the owner")
	var sys strings.Builder
	fmt.Fprintf(&sys, "You keep %s's message pad. %s, %s's assistant, is chatting on WhatsApp for %s. From the new messages, write down what people want from %s. JSON only.\n\n", owner, memOr(in.Assistant, "The assistant"), owner, owner, owner)
	sys.WriteString("Rules:\n")
	fmt.Fprintf(&sys, "- A note is a real request or message for %s: something to do, answer, call back, meet, or know. Small talk is not a note.\n", owner)
	sys.WriteString("- \"text\": one or two plain sentences: who wants what. \"callback\": how/when to get back to them, \"\" if not said. \"meeting\": a meeting they asked for (time, topic), \"\" if none.\n")
	sys.WriteString("- \"urgent\": true only if they said it's urgent or time-critical (today, asap, emergency) or it clearly can't wait.\n")
	sys.WriteString("- Don't repeat a note already taken unless there is new information; then write the full updated note.\n")
	fmt.Fprintf(&sys, "- Today is %s.\n", in.Now.In(zone).Format("Monday 2 January 2006"))

	var u strings.Builder
	if len(in.Open) > 0 {
		u.WriteString("Notes already taken in this chat:\n")
		for _, n := range in.Open {
			u.WriteString("- " + oneLine(n, 300) + "\n")
		}
		u.WriteString("\n")
	}
	u.WriteString("Messages (oldest first):\n")
	msgs := in.Messages
	if len(msgs) > MaxNoteMessages {
		msgs = msgs[len(msgs)-MaxNoteMessages:]
	}
	contact := memOr(in.ChatName, "Them")
	for _, m := range visible(msgs) {
		label := m.Name
		switch {
		case m.Speaker == "me" && m.FromBot:
			label = memOr(in.Assistant, "Assistant")
		case m.Speaker == "me":
			label = owner
		case label == "" && !in.IsGroup:
			label = contact
		case label == "":
			label = "Someone"
		}
		u.WriteString("[" + m.TS.In(zone).Format("15:04") + "] " + oneLine(label, 40) + ": " + oneLine(m.Text, 400) + "\n")
	}
	u.WriteString(`
Answer with JSON only: {"notes": [{"from": "name", "text": "...", "urgent": false, "callback": "", "meeting": ""}]}, or {"notes": []} if nobody left a message.`)
	return llm.Request{
		System:      sys.String(),
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: u.String()}},
		Temperature: 0.1,
		MaxTokens:   500,
		JSON:        true,
		Schema:      notesSchema,
	}
}

// ParseNotes reads a notes answer; an unreadable answer is an error, an
// empty list is fine.
func ParseNotes(raw string) ([]TakenNote, error) {
	js := ExtractJSON(raw)
	if js == "" {
		return nil, ErrNoJSON
	}
	var v struct {
		Notes []struct {
			From     string          `json:"from"`
			Text     string          `json:"text"`
			Urgent   json.RawMessage `json:"urgent"`
			Callback string          `json:"callback"`
			Meeting  string          `json:"meeting"`
		} `json:"notes"`
	}
	if err := json.Unmarshal([]byte(js), &v); err != nil {
		return nil, ErrNoJSON
	}
	var out []TakenNote
	for _, n := range v.Notes {
		text := oneLine(n.Text, 400)
		if text == "" {
			continue
		}
		urgent := strings.EqualFold(strings.Trim(strings.TrimSpace(string(n.Urgent)), `"`), "true")
		out = append(out, TakenNote{From: oneLine(n.From, 60), Text: text, Urgent: urgent,
			Callback: oneLine(n.Callback, 200), Meeting: oneLine(n.Meeting, 200)})
	}
	return out, nil
}
