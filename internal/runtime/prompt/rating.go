package prompt

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/runtime/llm"
)

// ─── Connection ratings ──────────────────────────────────────
//
// Once a day the persona scores, privately and in character, how connected
// it felt to each person who wrote in a group that day.

// MaxRatingMessages caps the messages one rating reads.
const MaxRatingMessages = 150

// RatingInput is what a day's ratings are written from.
type RatingInput struct {
	Persona  model.Persona
	ChatName string
	People   []string        // who wrote (display names), the only ones to rate
	Messages []model.Message // oldest first
	Zone     *time.Location
}

// RatedPerson is one parsed rating.
type RatedPerson struct {
	Name  string
	Score int
	Why   string
}

var ratingSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "people": {"type": "array", "items": {"type": "object", "properties": {
      "name": {"type": "string"}, "score": {"type": "integer", "minimum": 0, "maximum": 100}, "why": {"type": "string"}
    }, "required": ["name", "score", "why"], "additionalProperties": false}}
  },
  "required": ["people"],
  "additionalProperties": false
}`)

// Rating builds the request for a day's connection ratings. The caller sets Model.
func Rating(in RatingInput) llm.Request {
	zone := in.Zone
	if zone == nil {
		zone = time.Local
	}
	p := in.Persona
	name := memOr(p.Name, "the persona")
	var sys strings.Builder
	fmt.Fprintf(&sys, "You are %s. Privately, for nobody but yourself, rate how connected you felt to each person in the WhatsApp group \"%s\" today. JSON only.\n\n", name, memOr(in.ChatName, "group"))
	if bio := strings.TrimSpace(p.Personality); bio != "" {
		fmt.Fprintf(&sys, "Who you are: %s\n\n", oneLine(bio, 400))
	}
	sys.WriteString("How to score (0–100), by what happened today only:\n")
	sys.WriteString("- High: real back-and-forth with you, they talked to you, laughed with you, asked about you, shared something, you clicked.\n")
	sys.WriteString("- Middle: friendly but mostly talking to others, or only small talk with you.\n")
	sys.WriteString("- Low: ignored you, were cold or dismissive towards you, or barely talked.\n")
	sys.WriteString("- Judge the connection, not how much they wrote. Be honest and use the whole range.\n")
	fmt.Fprintf(&sys, "- \"why\": one short line in your own voice as %s, at most 15 words, about what they did today. No insults.\n", name)

	var u strings.Builder
	u.WriteString("People to rate (use these names exactly): " + strings.Join(in.People, ", ") + "\n\nToday's messages (oldest first):\n")
	msgs := in.Messages
	if len(msgs) > MaxRatingMessages {
		msgs = msgs[len(msgs)-MaxRatingMessages:]
	}
	for _, m := range visible(msgs) {
		label := m.Name
		switch {
		case m.Speaker == "me" && m.FromBot:
			label = name + " (you)"
		case m.Speaker == "me":
			continue // the owner's own messages are not part of the persona's day
		case label == "":
			label = "Someone"
		}
		ts := ""
		if !m.TS.IsZero() {
			ts = "[" + m.TS.In(zone).Format("15:04") + "] "
		}
		u.WriteString(ts + oneLine(label, 40) + ": " + oneLine(m.Text, 300) + "\n")
	}
	u.WriteString("\nAnswer with JSON only: {\"people\": [{\"name\": \"...\", \"score\": 0-100, \"why\": \"...\"}]} with one entry per person listed above.")
	return llm.Request{
		System:      sys.String(),
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: u.String()}},
		Temperature: 0.3,
		MaxTokens:   600,
		JSON:        true,
		Schema:      ratingSchema,
	}
}

// ParseRating reads a rating answer (tolerant of prose around the JSON and
// scores given as strings or 1–10). An answer with nobody rated is an error.
func ParseRating(raw string) ([]RatedPerson, error) {
	js := ExtractJSON(raw)
	if js == "" {
		return nil, ErrNoJSON
	}
	var v struct {
		People []struct {
			Name  string          `json:"name"`
			Score json.RawMessage `json:"score"`
			Why   json.RawMessage `json:"why"`
		} `json:"people"`
	}
	if err := json.Unmarshal([]byte(js), &v); err != nil {
		return nil, ErrNoJSON
	}
	var out []RatedPerson
	for _, p := range v.People {
		name := oneLine(p.Name, 40)
		var score float64
		if err := json.Unmarshal(p.Score, &score); err != nil {
			var s string
			if json.Unmarshal(p.Score, &s) == nil {
				fmt.Sscanf(strings.TrimSpace(s), "%g", &score)
			}
		}
		if name == "" {
			continue
		}
		out = append(out, RatedPerson{Name: name, Score: min(max(int(score+0.5), 0), 100), Why: oneLine(flexString(p.Why, " "), 160)})
	}
	if len(out) == 0 {
		return nil, ErrNoJSON
	}
	return out, nil
}
