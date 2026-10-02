package prompt

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
)

// ─── Chat brief (cross-chat context) ─────────────────────────
//
// A background job writes a short private brief of each chat that other
// chats of the same persona may draw on: topics, what the persona promised,
// the tone and (groups) a line per member. Only the brief crosses, never
// the messages.

// MaxBriefMessages caps the messages one brief reads.
const MaxBriefMessages = 30

// BriefInput is what a brief is written from.
type BriefInput struct {
	PersonaName string
	ChatName    string
	IsGroup     bool
	Messages    []model.Message // oldest first
	Zone        *time.Location  // where the times are shown (nil = Local)
	Now         time.Time
}

// BriefContent is a parsed brief answer.
type BriefContent struct {
	Topics      []string
	Commitments []string
	Tone        string
	People      []model.BriefPerson
	Sensitive   []string
}

var briefSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "topics": {"type": "array", "items": {"type": "string"}},
    "commitments": {"type": "array", "items": {"type": "string"}},
    "tone": {"type": "string"},
    "people": {"type": "array", "items": {"type": "object", "properties": {"name": {"type": "string"}, "note": {"type": "string"}}, "required": ["name", "note"], "additionalProperties": false}},
    "sensitive": {"type": "array", "items": {"type": "string", "enum": ["money", "health", "meeting", "distress", "legal", "romance", "secret"]}}
  },
  "required": ["topics", "commitments", "tone", "people", "sensitive"],
  "additionalProperties": false
}`)

// Brief builds the request for a chat's brief. The caller sets Model and MaxTokens.
func Brief(in BriefInput) llm.Request {
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
	fmt.Fprintf(&sys, "You write a short private brief for %s about %s so %s stays consistent in other chats. JSON only.\n\n", name, where, name)
	sys.WriteString("Rules:\n")
	sys.WriteString("- Only what is in the messages. Never guess.\n")
	fmt.Fprintf(&sys, "- \"commitments\": only things %s promised, agreed or said it would do, with the day if given. Empty if none.\n", name)
	fmt.Fprintf(&sys, "- Today is %s. Write relative days as dates like \"Sat 4 Oct\".\n", in.Now.In(zone).Format("Monday 2 January 2006"))
	sys.WriteString("- Never note moods of the moment.\n")
	sys.WriteString("- Leave anything sensitive out of topics, commitments and people; only name its category in \"sensitive\".\n")
	sys.WriteString("- \"sensitive\": every topic the messages touch among money, health, meeting (meeting up in person), distress (someone struggling), legal, romance (dating, partners, breakups), secret (asked to keep quiet); [] if none.\n")

	var u strings.Builder
	u.WriteString("Messages (oldest first):\n")
	msgs := in.Messages
	if len(msgs) > MaxBriefMessages {
		msgs = msgs[len(msgs)-MaxBriefMessages:]
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
			ts = "[" + m.TS.In(zone).Format("15:04") + "] "
		}
		u.WriteString(ts + oneLine(label, 40) + ": " + oneLine(m.Text, 300) + "\n")
	}
	people := `"people": [{"name": "...", "note": "what this person said or did, at most 12 words"}] (up to 6)`
	if !in.IsGroup {
		people = `"people": [] (always empty for a private chat)`
	}
	fmt.Fprintf(&u, "\nAnswer with JSON only: {\"topics\": [\"up to 4, a few words each\"], \"commitments\": [\"up to 3 things %s promised, agreed or said it would do, with the day if given, e.g. 'bring wine on Sat 4 Oct'; empty if none\"], \"tone\": \"one or two words\", %s, \"sensitive\": []}", name, people)
	return llm.Request{
		System:      sys.String(),
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: u.String()}},
		Temperature: 0.2,
		MaxTokens:   300,
		JSON:        true,
		Schema:      briefSchema,
	}
}

// ParseBrief reads a brief answer (tolerant like ParseRecap: other key
// names, lists given as one string, prose around the JSON).
func ParseBrief(raw string, isGroup bool) (BriefContent, error) {
	var bc BriefContent
	js := ExtractJSON(raw)
	if js == "" {
		return bc, ErrNoJSON
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		return bc, ErrNoJSON
	}
	get := func(keys ...string) json.RawMessage {
		for _, k := range keys {
			for mk, v := range m {
				if strings.EqualFold(mk, k) {
					return v
				}
			}
		}
		return nil
	}
	bc.Topics = flexList(get("topics", "topic"), 4, 60)
	bc.Commitments = flexList(get("commitments", "promises", "commitment"), 3, 120)
	bc.Tone = oneLine(flexString(get("tone", "mood", "vibe"), ", "), 30)
	for _, s := range flexList(get("sensitive", "sensitiveTopics"), 8, 20) {
		s = strings.ToLower(s)
		if containsStr(sensitiveEnum, s) && !containsStr(bc.Sensitive, s) {
			bc.Sensitive = append(bc.Sensitive, s)
		}
	}
	if isGroup {
		var people []map[string]any
		_ = json.Unmarshal(get("people", "members"), &people)
		for _, p := range people {
			name, _ := p["name"].(string)
			note, _ := p["note"].(string)
			name, note = oneLine(name, 40), oneLine(note, 120)
			if name == "" || note == "" {
				continue
			}
			bc.People = append(bc.People, model.BriefPerson{Name: name, Note: note})
			if len(bc.People) == 6 {
				break
			}
		}
	}
	return bc, nil
}
