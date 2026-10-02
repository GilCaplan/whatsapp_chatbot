package prompt

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/persona"
)

const builderSystem = `You design chat personas for a WhatsApp role-play app. Given a short description (and optionally sample messages showing how the persona writes), invent a vivid, specific, believable character.

Respond with ONLY one JSON object, no prose and no markdown, with exactly these fields:
{
  "name": "first name (or short nickname) the persona answers to",
  "tagline": "one catchy line, max 12 words",
  "avatar": {"glyph": "the picture that fits best, one of: spark, rocket, crystal, cash, dumbbell, martini, sofa, palette, coffee, headphones, leaf, moon, sun, wave, flame, heart, book, camera, crown, paw, flower, bolt, chat, globe"},
  "bio": "role, background, where they live — 2-3 sentences",
  "personality": "core traits and vibe, plus 3-4 numbered profile facts on separate lines",
  "style": "tone and texting style: sentence length, casing, punctuation habits",
  "vocabulary": "typical words and catch-phrases",
  "rules": "3-5 behavioural guidelines, one per line, each starting with '- '",
  "language": "language(s) they reply in",
  "emoji": {"usage": "none|rare|some|lots", "favorites": ["up to 4 emoji"]},
  "messageLength": "short|medium|long",
  "goal": "what they want out of a casual catch-up conversation",
  "decisionHint": "when they jump into a group chat, e.g. 'speaks up about food and travel'",
  "fallbackReply": "an in-character line used when they are confused"
}

Write every field from the persona's perspective as instructions for an actor. Never mention AI, models or prompts.`

// Builder builds the AI persona-builder request (strict JSON).
func Builder(description, samples string) llm.Request {
	var u strings.Builder
	u.WriteString("Description: " + strings.TrimSpace(description))
	if s := strings.TrimSpace(samples); s != "" {
		u.WriteString("\n\nSample messages written by this persona (match their voice):\n" + s)
	}
	return llm.Request{
		System:      builderSystem,
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: u.String()}},
		MaxTokens:   4096,
		Temperature: 0.7,
		JSON:        true,
		Schema:      builderSchema,
	}
}

// builderSchema constrains local models to exactly the persona fields.
var builderSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": {"type": "string"},
    "tagline": {"type": "string"},
    "avatar": {"type": "object", "properties": {"glyph": {"type": "string", "enum": ["spark", "rocket", "crystal", "cash", "dumbbell", "martini", "sofa", "palette", "coffee", "headphones", "leaf", "moon", "sun", "wave", "flame", "heart", "book", "camera", "crown", "paw", "flower", "bolt", "chat", "globe"]}}, "required": ["glyph"], "additionalProperties": false},
    "bio": {"type": "string"},
    "personality": {"type": "string"},
    "style": {"type": "string"},
    "vocabulary": {"type": "string"},
    "rules": {"type": "string"},
    "language": {"type": "string"},
    "emoji": {"type": "object", "properties": {
      "usage": {"type": "string", "enum": ["none", "rare", "some", "lots"]},
      "favorites": {"type": "array", "items": {"type": "string"}}
    }, "required": ["usage", "favorites"], "additionalProperties": false},
    "messageLength": {"type": "string", "enum": ["short", "medium", "long"]},
    "goal": {"type": "string"},
    "decisionHint": {"type": "string"},
    "fallbackReply": {"type": "string"}
  },
  "required": ["name", "tagline", "avatar", "bio", "personality", "style", "vocabulary", "rules", "language",
    "emoji", "messageLength", "goal", "decisionHint", "fallbackReply"],
  "additionalProperties": false
}`)

// ErrNoJSON means the model output contained no JSON object.
var ErrNoJSON = errors.New("the model did not return a JSON persona")

type draft struct {
	Name          string          `json:"name"`
	Tagline       string          `json:"tagline"`
	Avatar        json.RawMessage `json:"avatar"`
	Bio           json.RawMessage `json:"bio"`
	Personality   json.RawMessage `json:"personality"`
	Style         json.RawMessage `json:"style"`
	Vocabulary    json.RawMessage `json:"vocabulary"`
	Rules         json.RawMessage `json:"rules"`
	Language      json.RawMessage `json:"language"`
	Emoji         json.RawMessage `json:"emoji"`
	MessageLength string          `json:"messageLength"`
	Goal          json.RawMessage `json:"goal"`
	DecisionHint  json.RawMessage `json:"decisionHint"`
	FallbackReply json.RawMessage `json:"fallbackReply"`
}

// ParseDraft extracts the first balanced {...} object from raw model output and
// turns it into a normalized, unsaved persona (ID "", not built in).
func ParseDraft(raw string) (model.Persona, error) {
	obj := ExtractJSON(raw)
	if obj == "" {
		return model.Persona{}, ErrNoJSON
	}
	var d draft
	if err := json.Unmarshal([]byte(obj), &d); err != nil {
		return model.Persona{}, fmt.Errorf("could not parse persona JSON: %w", err)
	}
	p := model.Persona{
		Name:          strings.TrimSpace(d.Name),
		Tagline:       strings.TrimSpace(d.Tagline),
		Bio:           flexString(d.Bio, " "),
		Personality:   flexString(d.Personality, "\n"),
		Style:         flexString(d.Style, " "),
		Vocabulary:    flexString(d.Vocabulary, ", "),
		Rules:         bulletize(flexString(d.Rules, "\n")),
		Language:      flexString(d.Language, ", "),
		MessageLength: d.MessageLength,
		Goal:          flexString(d.Goal, " "),
		DecisionHint:  flexString(d.DecisionHint, " "),
		FallbackReply: flexString(d.FallbackReply, " "),
	}
	var av struct {
		Glyph string `json:"glyph"`
	}
	if json.Unmarshal(d.Avatar, &av) == nil {
		p.Avatar.Glyph = strings.ToLower(strings.TrimSpace(av.Glyph))
	} else {
		p.Avatar.Glyph = strings.ToLower(flexString(d.Avatar, ""))
	}
	var em struct {
		Usage     string          `json:"usage"`
		Favorites json.RawMessage `json:"favorites"`
	}
	if json.Unmarshal(d.Emoji, &em) == nil {
		p.Emoji.Usage = em.Usage
		var favs []string
		if json.Unmarshal(em.Favorites, &favs) == nil {
			p.Emoji.Favorites = favs
		} else if s := flexString(em.Favorites, " "); s != "" {
			p.Emoji.Favorites = strings.Fields(s)
		}
	} else {
		p.Emoji.Usage = flexString(d.Emoji, "")
	}
	if p.Name == "" {
		p.Name = "New Persona"
	}
	if err := persona.Normalize(&p); err != nil {
		return model.Persona{}, err
	}
	return p, nil
}

// ExtractJSON returns the first balanced {...} in s (string- and escape-aware), or "".
func ExtractJSON(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			esc = false
		case inStr && c == '\\':
			esc = true
		case c == '"':
			inStr = !inStr
		case inStr:
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// flexString accepts a JSON string, array of strings, or other value and returns text.
func flexString(raw json.RawMessage, sep string) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var arr []any
	if json.Unmarshal(raw, &arr) == nil {
		parts := make([]string, 0, len(arr))
		for _, v := range arr {
			if t := strings.TrimSpace(fmt.Sprint(v)); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, sep)
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		keys := slices.Sorted(maps.Keys(obj))
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s: %v", k, obj[k]))
		}
		return strings.Join(parts, sep)
	}
	return strings.Trim(string(raw), `"`)
}

var inlineBullets = regexp.MustCompile(`\s+-\s+`)

// bulletize makes every non-empty line a "- " bullet. A single line holding
// several "- " items ("- a. - b. - c") is split into one bullet each.
func bulletize(s string) string {
	var out []string
	lines := strings.Split(s, "\n")
	if len(lines) == 1 && strings.HasPrefix(strings.TrimSpace(s), "-") && strings.Count(s, " - ") >= 2 {
		lines = inlineBullets.Split(strings.TrimSpace(s), -1)
	}
	for _, l := range lines {
		if l = strings.TrimSpace(l); l == "" {
			continue
		}
		if !strings.HasPrefix(l, "-") {
			l = "- " + strings.TrimLeft(l, "•*0123456789.) ")
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
