package prompt

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
)

// ─── Daily recap (wave 3) ────────────────────────────────────

// MaxRecapMessages is how many messages one recap reads.
const MaxRecapMessages = 60

// RecapInput is what a recap is written from.
type RecapInput struct {
	ChatName    string
	PersonaName string
	IsGroup     bool
	Messages    []model.Message // oldest first; trimmed to the last MaxRecapMessages
	Goal        string          // the chat's goal ("" = none)
	GoalReached bool
	GoalPlan    string // the planner's last move, if any
	Memories    []string
	Handoff     *model.HandoffState
	Zone        *time.Location // times are shown in it (nil = Local)
}

// RecapContent is the model's part of a recap.
type RecapContent struct {
	Headline     string
	Topics       []string
	GoalProgress string
	ToKnow       []string
	Mood         string
}

var recapSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "headline": {"type": "string"},
    "topics": {"type": "array", "items": {"type": "string"}},
    "goalProgress": {"type": "string"},
    "toKnow": {"type": "array", "items": {"type": "string"}},
    "mood": {"type": "string"}
  },
  "required": ["headline", "topics", "goalProgress", "toKnow", "mood"],
  "additionalProperties": false
}`)

// Recap builds the daily recap request for one chat. The caller sets Model.
func Recap(in RecapInput) llm.Request {
	msgs := in.Messages
	if len(msgs) > MaxRecapMessages {
		msgs = msgs[len(msgs)-MaxRecapMessages:]
	}
	zone := in.Zone
	if zone == nil {
		zone = time.Local
	}
	where := "a private chat with " + orDefault(in.ChatName, "a contact")
	if in.IsGroup {
		where = "the group chat \"" + orDefault(in.ChatName, "a group") + "\""
	}
	var u strings.Builder
	fmt.Fprintf(&u, "%s is an AI persona answering WhatsApp messages for its owner in %s.\n", in.PersonaName, where)
	if g := strings.TrimSpace(in.Goal); g != "" {
		fmt.Fprintf(&u, "%s's secret goal in this chat: %s", in.PersonaName, g)
		if in.GoalReached {
			u.WriteString(" (already reached)")
		} else if in.GoalPlan != "" {
			fmt.Fprintf(&u, " (last planned move: %s)", in.GoalPlan)
		}
		u.WriteString("\n")
	}
	if len(in.Memories) > 0 {
		u.WriteString("Things learned about the people here today:\n")
		for _, m := range in.Memories {
			u.WriteString("- " + oneLine(m, 160) + "\n")
		}
	}
	if h := in.Handoff; h != nil {
		fmt.Fprintf(&u, "The chat is paused waiting for the owner because of a sensitive message (%s): %q\n", h.Category, h.Excerpt)
	}
	u.WriteString("\nThe messages (oldest first):\n")
	for _, m := range msgs {
		label := m.Name
		switch {
		case m.Speaker == "me" && m.FromBot:
			label = in.PersonaName
		case m.Speaker == "me":
			label = "Owner"
		case label == "":
			label = orDefault(in.ChatName, "Them")
		}
		ts := ""
		if !m.TS.IsZero() {
			ts = "[" + m.TS.In(zone).Format("15:04") + "] "
		}
		u.WriteString(ts + label + ": " + oneLine(m.Text, 400) + "\n")
	}
	u.WriteString("\nWrite the owner's recap of this chat as JSON:\n")
	u.WriteString(`{"headline": "one sentence, at most 15 words, on what happened",` + "\n")
	u.WriteString(` "topics": ["up to 4 short topics, a few words each"],` + "\n")
	if strings.TrimSpace(in.Goal) != "" {
		u.WriteString(` "goalProgress": "one short sentence on progress toward the goal",` + "\n")
	} else {
		u.WriteString(` "goalProgress": "",` + "\n")
	}
	u.WriteString(` "toKnow": ["up to 3 things the owner should know or follow up on: plans made, promises the persona made, questions still waiting"],` + "\n")
	u.WriteString(` "mood": "one or two words for the overall mood"}`)
	return llm.Request{
		System: "You write short, private daily recaps of WhatsApp chats for their owner. Be factual, warm and brief, in English. " +
			"Only use what is in the messages: never invent plans, names or facts. JSON only.",
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: u.String()}},
		MaxTokens:   450,
		Temperature: 0.3,
		JSON:        true,
		Schema:      recapSchema,
	}
}

// ParseRecap reads a recap answer (tolerant: alternative keys, lists given
// as one string, prose around the JSON). An answer without a headline is an
// error.
func ParseRecap(raw string) (RecapContent, error) {
	var rc RecapContent
	js := ExtractJSON(raw)
	if js == "" {
		return rc, ErrNoJSON
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		return rc, ErrNoJSON
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
	rc.Headline = oneLine(flexString(get("headline", "summary", "title"), " "), 200)
	rc.GoalProgress = oneLine(flexString(get("goalProgress", "goal_progress", "goal"), " "), 240)
	rc.Mood = oneLine(flexString(get("mood", "vibe", "tone"), ", "), 40)
	rc.Topics = flexList(get("topics", "topic"), 4, 60)
	rc.ToKnow = flexList(get("toKnow", "to_know", "followUps", "follow_ups", "notes"), 3, 200)
	if rc.Headline == "" {
		return rc, ErrNoJSON
	}
	return rc, nil
}

// flexList reads a list of strings (or one string split on newlines and
// semicolons), trimmed, deduplicated and capped at n items of max runes.
func flexList(raw json.RawMessage, n, maxRunes int) []string {
	var items []string
	var arr []any
	if json.Unmarshal(raw, &arr) == nil {
		for _, v := range arr {
			if s, ok := v.(string); ok {
				items = append(items, s)
			}
		}
	} else {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			items = strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == ';' })
		}
	}
	out := []string{}
	seen := map[string]bool{}
	for _, s := range items {
		s = oneLine(strings.Trim(strings.TrimSpace(s), "-•* "), maxRunes)
		if s == "" || seen[strings.ToLower(s)] {
			continue
		}
		seen[strings.ToLower(s)] = true
		out = append(out, s)
		if len(out) == n {
			break
		}
	}
	return out
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}
