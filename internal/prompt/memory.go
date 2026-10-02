package prompt

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
)

// ─── Memory of people (wave 3) ───────────────────────────────
//
// A background job asks the model what is worth remembering about the
// other people in a chat (ExtractMemories); the reply prompt then gets the
// most relevant memories (MemorySection).

// MaxMemoryLines caps the memories shown in a reply prompt.
const MaxMemoryLines = 12

// MemoryLine is one memory as shown in the reply prompt.
type MemoryLine struct {
	Person string // "" in DMs (it's about the contact)
	Text   string
}

// MemorySection renders what the persona remembers ("" when nothing).
func MemorySection(lines []MemoryLine, isGroup bool) string {
	var out []string
	for _, l := range lines {
		text := cleanLine(l.Text, model.MaxMemoryText)
		if text == "" {
			continue
		}
		if who := cleanLine(l.Person, 40); who != "" && isGroup {
			text = who + ": " + text
		}
		out = append(out, "- "+text)
		if len(out) == MaxMemoryLines {
			break
		}
	}
	if len(out) == 0 {
		return ""
	}
	head := "\n\nWHAT YOU REMEMBER ABOUT THEM"
	if isGroup {
		head = "\n\nWHAT YOU REMEMBER ABOUT PEOPLE HERE"
	}
	return head + " (bring something up only when it fits naturally, like a friend would — never recite this list or say you \"remember\" it):\n" + strings.Join(out, "\n")
}

// ─── extraction ──────────────────────────────────────────────

// MaxExtractMessages caps the new messages one extraction reads.
const MaxExtractMessages = 40

// MaxExtractKnown caps the existing memories listed for deduplication.
const MaxExtractKnown = 40

// FoundMemory is one memory proposed by the extractor.
type FoundMemory struct {
	Person   string `json:"person"`
	Text     string `json:"text"`
	Kind     string `json:"kind"`
	Expires  string `json:"expires"` // YYYY-MM-DD or ""
	Evidence string `json:"evidence"`
	// Sensitive is "" or the sensitive topic the note touches (money,
	// health, meeting, distress, legal, romance, secret).
	Sensitive string `json:"sensitive"`
}

// sensitiveEnum are the extractor's sensitive values (model.SensitiveCategories minus "bot").
var sensitiveEnum = []string{"money", "health", "meeting", "distress", "legal", "romance", "secret"}

var memorySchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "memories": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "person": {"type": "string"},
          "evidence": {"type": "string"},
          "text": {"type": "string"},
          "kind": {"type": "string", "enum": ["fact", "preference", "event", "relationship"]},
          "expires": {"type": "string"},
          "sensitive": {"type": "string", "enum": ["", "money", "health", "meeting", "distress", "legal", "romance", "secret"]}
        },
        "required": ["person", "evidence", "text", "kind", "expires", "sensitive"],
        "additionalProperties": false
      }
    }
  },
  "required": ["memories"],
  "additionalProperties": false
}`)

// ExtractMemories builds the background request that finds new durable
// facts about the other people in a chat. known are the memories already
// stored ("Dana: works as a nurse"), msgs the new messages, now the date
// relative dates are resolved against (in the Mac's zone). The caller sets
// Model, MaxTokens and Temperature.
func ExtractMemories(personaName string, isGroup bool, contact string, known []string, msgs []model.Message, now time.Time) llm.Request {
	where := "a WhatsApp chat with " + memOr(contact, "a friend")
	if isGroup {
		where = "a WhatsApp group chat"
	}
	var sys strings.Builder
	fmt.Fprintf(&sys, "You keep a private notebook for %s about the OTHER people in %s. Read the new messages and note only things worth remembering about those people a week from now:\n", personaName, where)
	sys.WriteString("- facts about their life: job, studies, family, pets, where they live, health they mention\n")
	sys.WriteString("- clear likes and dislikes: food, music, sports teams, hobbies\n")
	sys.WriteString("- upcoming events with a date: exams, trips, birthdays, interviews, moving\n")
	sys.WriteString("- relationships: partner, siblings, best friend\n\n")
	sys.WriteString("Rules:\n")
	fmt.Fprintf(&sys, "- Never note anything about %s, or anything %s said about themselves.\n", personaName, personaName)
	sys.WriteString("- Only what a person clearly said about themselves (or someone said about them). Never guess, infer or exaggerate.\n")
	sys.WriteString("- Skip moods of the moment (tired, bored, hungry right now), greetings, jokes, small talk and opinions about the chat.\n")
	sys.WriteString("- Skip what they did today or are doing right now (\"was at work all day\", \"just got home\", \"back from a shift\") — only things that stay true, and plans or events still coming up (next week, next month).\n")
	sys.WriteString("- \"text\": one short line, at most 12 words, about the person in the third person without their name. Keep names of people, pets and places, e.g. \"works night shifts as a nurse\", \"sister Noa visits from Berlin this weekend\", \"has a driving test on Sun 4 Oct\".\n")
	fmt.Fprintf(&sys, "- Today is %s. Turn relative dates (\"tomorrow\", \"next Friday\") into real dates written like \"Fri 9 Oct\" in the text. For events set \"expires\" to the event date as YYYY-MM-DD, else \"\".\n", now.Format("Monday 2 January 2006"))
	sys.WriteString("- \"evidence\": copy the exact words from the message it comes from.\n")
	sys.WriteString("- \"person\": the name exactly as shown before their message.\n")
	sys.WriteString("- \"sensitive\": \"\" unless the note is about money or debts, health or medical matters, meeting up in person, someone struggling emotionally, legal trouble, a relationship, dating or breakup, or something they asked to keep quiet — then that one word: money, health, meeting, distress, legal, romance or secret.\n")
	sys.WriteString("- Skip anything already in the notebook unless it changed (then write the new version).\n")
	sys.WriteString("- Most messages contain nothing worth noting. An empty list is the usual answer.\n\n")
	sys.WriteString(`Answer with JSON only: {"memories": [{"person": "...", "evidence": "...", "text": "...", "kind": "fact|preference|event|relationship", "expires": "", "sensitive": ""}]}`)

	var u strings.Builder
	u.WriteString("Already in the notebook:\n")
	if len(known) == 0 {
		u.WriteString("(nothing yet)\n")
	}
	if len(known) > MaxExtractKnown {
		known = known[len(known)-MaxExtractKnown:]
	}
	for _, k := range known {
		u.WriteString("- " + oneLine(k, 200) + "\n")
	}
	if len(msgs) > MaxExtractMessages {
		msgs = msgs[len(msgs)-MaxExtractMessages:]
	}
	u.WriteString("\nNew messages (oldest first):\n")
	for _, m := range visible(msgs) {
		label := m.Name
		if m.Speaker == "me" {
			label = personaName + " (not noted)"
		} else if label == "" {
			label = memOr(contact, "Them")
		}
		u.WriteString(label + ": " + oneLine(m.Text, 400) + "\n")
	}
	u.WriteString("\nWhat should go in the notebook? JSON only.")
	return llm.Request{
		System:      sys.String(),
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: u.String()}},
		Temperature: 0,
		JSON:        true,
		Schema:      memorySchema,
	}
}

// ParseMemories reads the extractor's answer (tolerant: a bare array, other
// key names, missing fields). Lines without a text are dropped.
func ParseMemories(raw string) ([]FoundMemory, error) {
	raw = strings.TrimSpace(raw)
	var items []map[string]any
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &items); err != nil {
			return nil, fmt.Errorf("memories: %w", err)
		}
	} else {
		js := ExtractJSON(raw)
		if js == "" {
			return nil, fmt.Errorf("memories: no JSON in answer")
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(js), &m); err != nil {
			return nil, fmt.Errorf("memories: %w", err)
		}
		for k, v := range m {
			if arr, ok := v.([]any); ok && (strings.EqualFold(k, "memories") || strings.EqualFold(k, "notes") || strings.EqualFold(k, "items") || len(m) == 1) {
				for _, e := range arr {
					if o, ok := e.(map[string]any); ok {
						items = append(items, o)
					}
				}
			}
		}
	}
	str := func(o map[string]any, keys ...string) string {
		for _, k := range keys {
			for ok, v := range o {
				if strings.EqualFold(ok, k) {
					if s, isStr := v.(string); isStr {
						return strings.TrimSpace(s)
					}
				}
			}
		}
		return ""
	}
	out := make([]FoundMemory, 0, len(items))
	for _, o := range items {
		f := FoundMemory{
			Person:   oneLine(str(o, "person", "name", "who"), 60),
			Text:     oneLine(str(o, "text", "memory", "note", "fact"), model.MaxMemoryText),
			Kind:     strings.ToLower(str(o, "kind", "type", "category")),
			Expires:  str(o, "expires", "date", "expiresAt"),
			Evidence: oneLine(str(o, "evidence", "quote"), 200),
		}
		if sv := strings.ToLower(str(o, "sensitive", "private", "topic")); slices.Contains(sensitiveEnum, sv) {
			f.Sensitive = sv
		}
		if f.Text == "" {
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

// memOr is s, or d when s is blank.
func memOr(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}
