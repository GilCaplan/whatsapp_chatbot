package prompt

import (
	"encoding/json"
	"fmt"
	"strings"

	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
)

// ─── Hand-off check (wave 3) ─────────────────────────────────
//
// A weak keyword hit ("I owe you for the pizza", "meet you at the gym lol")
// is confirmed by one tiny JSON call before the persona is paused.

// MaxHandoffContext is how many earlier messages the hand-off check reads.
const MaxHandoffContext = 4

// handoffTopics describes each category for the check.
var handoffTopics = map[string]string{
	model.HandoffMoney:    "asking for or arranging money, payments, loans, bank or card details",
	model.HandoffHealth:   "a real physical illness, injury, hospital visit or medical emergency",
	model.HandoffMeeting:  "making concrete plans to meet in person (a time, a place, an address, picking someone up)",
	model.HandoffDistress: "someone sad, depressed, scared or in emotional crisis, or talking about hurting themselves",
	model.HandoffBot:      "asking whether they are talking to a bot, an AI or someone other than the real person",
	model.HandoffLegal:    "lawyers, police, courts, lawsuits or contracts",
}

// HandoffCheck builds the "does this message need the real person?" request.
// cats are the enabled categories (display order), hist the messages before
// the latest one, sender who wrote it. The caller sets Model.
func HandoffCheck(personaName string, cats []string, hist []model.Message, sender, text string) llm.Request {
	if len(hist) > MaxHandoffContext {
		hist = hist[len(hist)-MaxHandoffContext:]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s is an AI persona texting for its owner in a WhatsApp chat. The owner takes over personally when a message is really about one of these topics:\n", personaName)
	for _, c := range cats {
		fmt.Fprintf(&b, "- %s: %s\n", c, handoffTopics[c])
	}
	b.WriteString("Jokes, figures of speech (\"this movie killed me\", \"I'm dead lol\"), passing mentions and everyday small talk are NOT serious.\n\n")
	if len(hist) > 0 {
		b.WriteString("Recent messages (oldest first):\n")
		for _, m := range hist {
			label := m.Name
			if m.Speaker == "me" {
				label = personaName
			} else if label == "" {
				label = "Them"
			}
			b.WriteString(label + ": " + strings.ReplaceAll(m.Text, "\n", " ") + "\n")
		}
		b.WriteString("\n")
	}
	if sender == "" {
		sender = "them"
	}
	fmt.Fprintf(&b, "Latest message from %s: %q\n\n", sender, text)
	fmt.Fprintf(&b, "Is the latest message seriously about one of the topics above? Answer with JSON: {\"category\": one of %s or \"none\", \"serious\": true or false}.", strings.Join(quoteAll(cats), ", "))
	enum := append(append([]string{}, cats...), "none")
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"category": map[string]any{"type": "string", "enum": enum},
			"serious":  map[string]any{"type": "boolean"},
		},
		"required":             []string{"category", "serious"},
		"additionalProperties": false,
	})
	return llm.Request{
		System:      "You decide whether a chat message needs a real person to answer it. JSON only.",
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: b.String()}},
		MaxTokens:   40,
		Temperature: 0,
		JSON:        true,
		Schema:      schema,
	}
}

// ParseHandoffCheck reads a HandoffCheck answer. It returns the category
// only when the model says the message is serious and the category is one
// of cats; anything unreadable is "not serious".
func ParseHandoffCheck(raw string, cats []string) (category string, serious bool) {
	js := ExtractJSON(raw)
	if js == "" {
		return "", false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		return "", false
	}
	for k, v := range m {
		switch strings.ToLower(k) {
		case "category", "topic":
			if s, ok := v.(string); ok {
				category = strings.ToLower(strings.TrimSpace(s))
			}
		case "serious", "needs_owner", "handoff":
			switch x := v.(type) {
			case bool:
				serious = x
			case string:
				serious = strings.EqualFold(strings.TrimSpace(x), "true") || strings.EqualFold(strings.TrimSpace(x), "yes")
			}
		}
	}
	valid := false
	for _, c := range cats {
		valid = valid || c == category
	}
	if !serious || !valid {
		return "", false
	}
	return category, true
}

func quoteAll(xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = `"` + x + `"`
	}
	return out
}
