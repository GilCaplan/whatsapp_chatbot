package prompt

import (
	"encoding/json"
	"regexp"
	"strings"

	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
)

// ─── Co-pilot drafts (wave 3) ────────────────────────────────
//
// In co-pilot mode one call writes three alternatives (brief, warm, playful)
// and you pick one in Approvals.

// DraftTones are the co-pilot tones in the order they are offered.
var DraftTones = []string{model.DraftToneBrief, model.DraftToneWarm, model.DraftTonePlayful}

// DraftsNote is appended to the reply system prompt for co-pilot drafts.
const DraftsNote = "\n\nCO-PILOT: write THREE different replies to the last message, each a complete WhatsApp message in your own voice and language:\n" +
	"1) brief: the shortest natural answer\n" +
	"2) warm: engaged and kind, asks or adds something\n" +
	"3) playful: a light joke or tease if it fits (otherwise just more relaxed)\n" +
	"Keep the same facts in all three and follow every rule above. Answer with JSON only: " +
	`{"drafts": [{"tone": "brief", "text": "..."}, {"tone": "warm", "text": "..."}, {"tone": "playful", "text": "..."}]}`

var draftsSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "drafts": {
      "type": "array",
      "minItems": 3,
      "maxItems": 3,
      "items": {
        "type": "object",
        "properties": {
          "tone": {"type": "string", "enum": ["brief", "warm", "playful"]},
          "text": {"type": "string"}
        },
        "required": ["tone", "text"],
        "additionalProperties": false
      }
    }
  },
  "required": ["drafts"],
  "additionalProperties": false
}`)

// Drafts turns a reply request (prompt.Compose) into the co-pilot request:
// the same prompt and history plus DraftsNote, in JSON mode. The caller sets
// Model, MaxTokens and Temperature.
func Drafts(base llm.Request) llm.Request {
	base.System += DraftsNote
	base.JSON = true
	base.Schema = draftsSchema
	return base
}

var numberedLine = regexp.MustCompile(`^\s*(?:\d[.)]|[-•*])\s*(?:\(?(brief|warm|playful)\)?\s*[:\-–]\s*)?(.+)$`)

// ParseDrafts reads a co-pilot answer. It tolerates prose around the JSON,
// a bare array, tone keys at the top level ({"brief": "..."}), other text
// keys ("reply", "message") and — without JSON — numbered lines. Tones that
// are missing or unknown follow the offered order. Empty texts are dropped;
// at most three drafts are returned.
func ParseDrafts(raw string) []model.Draft {
	var out []model.Draft
	add := func(tone, text string) {
		text = strings.TrimSpace(text)
		if text == "" || len(out) >= len(DraftTones) {
			return
		}
		out = append(out, model.Draft{Tone: strings.ToLower(strings.TrimSpace(tone)), Text: text})
	}
	if js := extractAnyJSON(raw); js != "" {
		var v any
		if json.Unmarshal([]byte(js), &v) == nil {
			collectDrafts(v, add)
		}
	}
	if len(out) == 0 && !strings.Contains(raw, "{") {
		for _, line := range strings.Split(raw, "\n") {
			if m := numberedLine.FindStringSubmatch(line); m != nil {
				add(m[1], strings.Trim(m[2], `"' `))
			}
		}
	}
	// Tones: keep known, unique ones; give the rest the free tones in order.
	used := map[string]bool{}
	for i := range out {
		if !isDraftTone(out[i].Tone) || used[out[i].Tone] {
			out[i].Tone = ""
			continue
		}
		used[out[i].Tone] = true
	}
	for i := range out {
		if out[i].Tone != "" {
			continue
		}
		for _, t := range DraftTones {
			if !used[t] {
				out[i].Tone, used[t] = t, true
				break
			}
		}
	}
	return out
}

func collectDrafts(v any, add func(tone, text string)) {
	switch x := v.(type) {
	case []any:
		for _, it := range x {
			switch d := it.(type) {
			case string:
				add("", d)
			case map[string]any:
				add(strField(d, "tone", "style", "type"), strField(d, "text", "reply", "message", "draft", "content"))
			}
		}
	case map[string]any:
		for k, inner := range x {
			if lk := strings.ToLower(k); lk == "drafts" || lk == "replies" || lk == "options" || lk == "messages" {
				collectDrafts(inner, add)
				return
			}
		}
		for _, t := range DraftTones {
			for k, inner := range x {
				if s, ok := inner.(string); ok && strings.EqualFold(k, t) {
					add(t, s)
				}
			}
		}
	}
}

func strField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		for mk, v := range m {
			if strings.EqualFold(mk, k) {
				if s, ok := v.(string); ok {
					return s
				}
			}
		}
	}
	return ""
}

func isDraftTone(t string) bool {
	for _, x := range DraftTones {
		if x == t {
			return true
		}
	}
	return false
}

// extractAnyJSON returns the outermost [...] array when s starts with one
// (before any object), else the first JSON object (ExtractJSON).
func extractAnyJSON(s string) string {
	i, j := strings.Index(s, "["), strings.LastIndex(s, "]")
	if o := strings.Index(s, "{"); i >= 0 && j > i && (o < 0 || i < o) {
		if json.Valid([]byte(s[i : j+1])) {
			return s[i : j+1]
		}
	}
	return ExtractJSON(s)
}
