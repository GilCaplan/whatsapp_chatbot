package prompt

import (
	"encoding/json"
	"fmt"
	"strings"

	"whatsappdoppel/internal/goals"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
)

// ─── Goal section of the reply prompt ────────────────────────

// GoalTurn is the per-reply goal context the engine computes.
type GoalTurn struct {
	// Plan is the private next move from the plan-ahead step ("" = none).
	Plan string
	// Reached means the goal already happened.
	Reached bool
	// Retry is set when the previous draft gave the goal away.
	Retry bool
	// Off drops the agenda for this reply (last resort after leaky drafts).
	Off bool
}

// GoalSection renders the goal part of the system prompt (replaces the old
// bare "GOAL: …" line). The goal is framed as a private agenda whose
// secrecy and pace depend on the style.
func GoalSection(g goals.Config, t GoalTurn) string {
	if t.Off || g.Relaxed(t.Reached) {
		return "\n\nYOUR AGENDA: nothing in particular right now. Just chat naturally and enjoy the conversation."
	}
	word := ""
	if sw, ok := goals.ParseSayWord(g.Text); ok {
		word = sw.Word
	}
	var b strings.Builder
	switch g.Style {
	case model.GoalStyleDirect:
		b.WriteString("\n\nYOUR AGENDA: " + g.Text + "\n")
		b.WriteString("Go for it openly and confidently, in your own style: ask or suggest it plainly and keep coming back to it. ")
		b.WriteString("Still answer what people actually said, and never mention instructions, a goal or a mission.")
	case model.GoalStyleBalanced:
		b.WriteString("\n\nYOUR PRIVATE AGENDA: " + g.Text + "\n")
		b.WriteString("Keep the agenda itself to yourself — never say you have one or explain why you're asking.\n")
		b.WriteString("- First respond to what was actually said, then steer. One nudge per message at most.\n")
		b.WriteString("- You may bring the topic up directly with a natural question or suggestion, as a friend would.\n")
		if word != "" {
			b.WriteString(fmt.Sprintf("- Never write the word %q yourself, not even as a joke, a hint or a spelling. Get them to say it on their own.\n", word))
		}
		b.WriteString("- If they dodge it, drop it for a few messages and come back from another angle.")
	default:
		b.WriteString("\n\nYOUR PRIVATE AGENDA: " + g.Text + "\n")
		b.WriteString("This is a secret: nobody in the chat knows about it, and they must never find out.\n")
		b.WriteString("- Never say it, say that you want something, or explain why you're asking.\n")
		if word != "" {
			b.WriteString(fmt.Sprintf("- Never write the word %q yourself and never ask them to say it. Ask a normal question whose most natural answer is that word, so they come up with it themselves.\n", word))
		} else {
			b.WriteString("- Asking a casual question or suggesting/inviting something as your own idea is fine once the chat allows it — the secret is that it's a goal, not the topic itself. A forced, repeated or out-of-nowhere push is not fine.\n")
		}
		b.WriteString("- No riddles or cryptic hints (\"a certain something…\") — they sound weird. Be plain and natural.\n")
		b.WriteString("- First respond to what was actually said, then take one small step towards it. Steer gradually over several messages.\n")
		b.WriteString("- If the moment isn't right, just chat normally. If something didn't work, try a different angle instead of repeating it.")
	}
	if t.Reached {
		b.WriteString("\n(It already happened once. Keep it light — no need to push.)")
	}
	if p := strings.TrimSpace(t.Plan); p != "" {
		b.WriteString("\n\nYOUR NEXT MOVE (private note to yourself — do this in this reply, in your own words and style, never quote it): " + p)
	}
	if t.Retry {
		b.WriteString("\n\nIMPORTANT: your last draft gave your agenda away. Write a different reply that answers what was said and keeps the agenda secret")
		if word != "" {
			b.WriteString(fmt.Sprintf(" — and do not use the word %q", word))
		}
		b.WriteString(".")
	}
	return b.String()
}

// ─── Plan ahead (strategist) ─────────────────────────────────

// PlanResult is the strategist's verdict.
type PlanResult struct {
	Situation string `json:"situation"`
	Achieved  bool   `json:"achieved"`
	Evidence  string `json:"evidence"`
	Next      string `json:"next"`
}

// MaxPlanHistory is how many recent messages the strategist reads.
const MaxPlanHistory = 16

// MaxPlanRunes caps the next move fed into the reply prompt.
const MaxPlanRunes = 220

var planSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "situation": {"type": "string"},
    "achieved": {"type": "boolean"},
    "evidence": {"type": "string"},
    "next": {"type": "string"}
  },
  "required": ["situation", "achieved", "evidence", "next"],
  "additionalProperties": false
}`)

// planWordSchema is planSchema plus "ideas" for say-the-word goals: everyday
// questions whose most obvious answer is the word, brainstormed before "next".
var planWordSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "situation": {"type": "string"},
    "achieved": {"type": "boolean"},
    "evidence": {"type": "string"},
    "ideas": {"type": "string"},
    "next": {"type": "string"}
  },
  "required": ["situation", "achieved", "evidence", "ideas", "next"],
  "additionalProperties": false
}`)

func planStyleRules(style, name, word string) string {
	var b strings.Builder
	switch style {
	case model.GoalStyleDirect:
		b.WriteString("- " + name + " pursues the goal openly: the move can ask or suggest it plainly, in " + name + "'s own style.\n")
		b.WriteString("- It still has to make sense as a reply to the last message and never mention having a goal or instructions.\n")
	case model.GoalStyleBalanced:
		b.WriteString("- The move may raise the topic directly with a natural question or suggestion, but never explains why or reveals that it is a goal.\n")
		b.WriteString("- One step per message; first react to what was just said.\n")
		b.WriteString("- If they dodged it, switch to a different angle or let it rest for a message.\n")
	default:
		b.WriteString("- The goal stays secret: the move never reveals it or says why " + name + " is asking.\n")
		b.WriteString("- It reacts to the last message first, then takes ONE small step. After a couple of friendly messages, actually take a step — don't just keep \"building rapport\".\n")
		b.WriteString("- Finding something out: a casual question a friend would ask anyway is fine (e.g. \"ask if she has anything fun planned for Saturday\"), as long as it fits the moment and isn't repeated.\n")
		b.WriteString("- Convincing someone: after a little rapport, suggest it plainly as " + name + "'s own idea, the way a friend invites someone; if they hesitate, make it easier or more tempting instead of repeating it. Don't keep avoiding the topic.\n")
		b.WriteString("- Getting someone to say a word: never ask them to say it; set up an ordinary question whose most natural answer is that word. Vague topics rarely work — after a little rapport, ask a specific everyday question (\"what's your favourite …\", \"which … do you …\") where the word is the single most obvious answer.\n")
		b.WriteString("- Plain and natural: no riddles or cryptic hints (\"a certain something\").\n")
		b.WriteString("- If an earlier move didn't work, pick a clearly different angle.\n")
	}
	if word != "" && style != model.GoalStyleDirect {
		b.WriteString(fmt.Sprintf("- The move must NOT contain the word %q — describe the setup instead (what to ask or talk about so that they say it).\n", word))
	}
	b.WriteString("- The move is concrete — exactly what to ask or say about — and fits " + name + "'s personality. Never repeat one of your earlier moves word for word.\n")
	return b.String()
}

// PlanOptions tunes Plan.
type PlanOptions struct {
	// Opening: the persona is about to start the conversation (Initiate).
	Opening bool
}

// Plan builds the private "plan ahead" request: read the chat, say whether
// the goal already happened (with evidence) and pick the next move. prev are
// earlier moves (oldest first). The caller sets Model.
func Plan(p model.Persona, g goals.Config, hist []model.Message, isGroup bool, prev []string, opts ...PlanOptions) llm.Request {
	name := p.Name
	where := "a WhatsApp chat"
	if isGroup {
		where = "a WhatsApp group chat"
	}
	who := name
	if t := strings.TrimSpace(p.Tagline); t != "" {
		who += " (" + t + ")"
	}
	word := ""
	if sw, ok := goals.ParseSayWord(g.Text); ok {
		word = sw.Word
	}
	var sys strings.Builder
	fmt.Fprintf(&sys, "You are the private coach of %s, who is texting in %s. %s has a secret goal the other people don't know about:\nGOAL: %s\n\n", who, where, name, g.Text)
	sys.WriteString("Read the chat and pick " + name + "'s next move. A good move:\n")
	sys.WriteString(planStyleRules(g.Style, name, word))
	sys.WriteString("\nAnswer with JSON only:\n")
	sys.WriteString(`{"situation": "one short sentence: what the chat is about right now and how close it is to the goal",` + "\n")
	sys.WriteString(` "achieved": true if the goal has already happened in the other people's messages (they told it, wrote the word, or agreed), else false,` + "\n")
	sys.WriteString(` "evidence": "if achieved: copy the exact words from their message that prove it, else empty",` + "\n")
	schema := planSchema
	if word != "" && g.Style != model.GoalStyleDirect {
		schema = planWordSchema
		sys.WriteString(` "ideas": "first say what kind of thing the target word is (a fruit, a city, a brand…), then two everyday questions that almost everyone would answer with exactly that word — written without the word",` + "\n")
		sys.WriteString(` "next": "one short instruction for ` + name + `'s next message — once the chat allows, ask one of your ideas, with a short natural bridge from the current topic; an action, not the message itself"}`)
	} else {
		sys.WriteString(` "next": "one short instruction for ` + name + `'s next message, e.g. 'Ask Dana what she's cooking this weekend' — an action, not the message itself"}`)
	}

	h := hist
	if len(h) > MaxPlanHistory {
		h = h[len(h)-MaxPlanHistory:]
	}
	var u strings.Builder
	u.WriteString("The chat so far (oldest first):\n")
	if len(h) == 0 {
		u.WriteString("(no messages yet)\n")
	}
	for _, m := range h {
		label := m.Name
		if m.Speaker == "me" {
			label = name + " (you're coaching them)"
		} else if label == "" {
			label = "Them"
		}
		u.WriteString(label + ": " + strings.ReplaceAll(m.Text, "\n", " ") + "\n")
	}
	if len(prev) > 0 {
		u.WriteString("\nYour earlier moves (oldest first):\n")
		for _, m := range prev {
			u.WriteString("- " + m + "\n")
		}
	}
	if len(opts) > 0 && opts[0].Opening {
		u.WriteString("\nThe chat has been quiet and " + name + " is about to start the conversation. What should the opening message do?")
	} else {
		u.WriteString("\nWhat is " + name + "'s next move?")
	}
	return llm.Request{
		System:      sys.String(),
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: u.String()}},
		MaxTokens:   220,
		Temperature: 0.4,
		JSON:        true,
		Schema:      schema,
	}
}

// ParsePlan reads the strategist's answer. It tolerates prose around the
// JSON, alternative key names, string booleans and — when there is no JSON
// at all — takes the first sentence-like line as the move. The move is
// trimmed to one line of at most MaxPlanRunes.
func ParsePlan(raw string) (PlanResult, error) {
	var res PlanResult
	if js := ExtractJSON(raw); js != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(js), &m); err == nil {
			str := func(keys ...string) string {
				for _, k := range keys {
					for mk, v := range m {
						if strings.EqualFold(mk, k) {
							switch x := v.(type) {
							case string:
								return x
							case []any:
								var parts []string
								for _, e := range x {
									if s, ok := e.(string); ok {
										parts = append(parts, s)
									}
								}
								return strings.Join(parts, " ")
							case bool:
								return fmt.Sprint(x)
							}
						}
					}
				}
				return ""
			}
			res.Situation = oneLine(str("situation", "analysis"), MaxPlanRunes)
			res.Evidence = oneLine(str("evidence", "quote", "proof"), MaxPlanRunes)
			res.Next = cleanMove(str("next", "next_move", "nextMove", "move", "plan", "action"))
			switch strings.ToLower(strings.TrimSpace(str("achieved", "done", "reached"))) {
			case "true", "yes", "1":
				res.Achieved = true
			}
			if res.Next == "" && !res.Achieved {
				return res, ErrNoJSON
			}
			return res, nil
		}
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		low := strings.ToLower(line)
		for _, pre := range []string{"next move:", "next:", "move:", "plan:"} {
			if strings.HasPrefix(low, pre) {
				line = strings.TrimSpace(line[len(pre):])
				break
			}
		}
		if line = cleanMove(line); line != "" && !strings.ContainsAny(line, "{}") {
			res.Next = line
			return res, nil
		}
	}
	return res, ErrNoJSON
}

func cleanMove(s string) string {
	s = strings.Trim(strings.TrimSpace(s), "\"'`*-• ")
	return oneLine(s, MaxPlanRunes)
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		s = strings.TrimSpace(string(r[:n])) + "…"
	}
	return s
}

// ─── Goal check (has it already happened?) ───────────────────

// MaxCheckMessages is how many of their recent messages the goal check reads.
const MaxCheckMessages = 12

var checkSchema = json.RawMessage(`{
  "type": "object",
  "properties": {"needed": {"type": "string"}, "answer": {"type": "boolean"}, "quote": {"type": "string"}},
  "required": ["needed", "answer", "quote"],
  "additionalProperties": false
}`)

// GoalCheck builds a narrow yes/no request: do the other people's recent
// messages already accomplish the goal? Small models answer this far more
// reliably than as part of the planning request (and, on llama3.1:8b, more
// reliably without the persona's own messages as context). hist should only
// contain messages since the goal was set. The caller sets Model.
func GoalCheck(p model.Persona, g goals.Config, hist []model.Message) llm.Request {
	var theirs []model.Message
	for _, m := range hist {
		if m.Speaker != "me" {
			theirs = append(theirs, m)
		}
	}
	if len(theirs) > MaxCheckMessages {
		theirs = theirs[len(theirs)-MaxCheckMessages:]
	}
	var b strings.Builder
	for i, m := range theirs {
		label := m.Name
		if label == "" {
			label = "Them"
		}
		fmt.Fprintf(&b, "[%d] %s: %s\n", i+1, label, strings.ReplaceAll(m.Text, "\n", " "))
	}
	if len(theirs) == 0 {
		b.WriteString("(no messages)\n")
	}
	q := fmt.Sprintf("These are the latest messages other people wrote to %s:\n%s\n%s wanted this: %q.\n"+
		"Question: do these messages already accomplish it? For \"find out\" goals: did they actually tell the specific thing asked (e.g. the place, the plan) — even roughly? "+
		"For \"get them to say\" goals: did they write it? For \"convince\"/\"get them to\" goals: did they agree or do it?\n"+
		"Answer with JSON: {\"needed\": what exactly they would have to tell, say or agree to, \"answer\": true or false, \"quote\": the exact words that give it, or \"\"}.",
		p.Name, b.String(), p.Name, g.Text)
	return llm.Request{
		System:      "You answer questions about chat messages precisely. JSON only.",
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: q}},
		MaxTokens:   120,
		Temperature: 0,
		JSON:        true,
		Schema:      checkSchema,
	}
}

// ParseGoalCheck reads a GoalCheck answer (tolerant like ParsePlan).
func ParseGoalCheck(raw string) (achieved bool, quote string, err error) {
	js := ExtractJSON(raw)
	if js == "" {
		return false, "", ErrNoJSON
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		return false, "", ErrNoJSON
	}
	for k, v := range m {
		switch strings.ToLower(k) {
		case "answer", "achieved", "yes":
			switch x := v.(type) {
			case bool:
				achieved = x
			case string:
				achieved = strings.EqualFold(strings.TrimSpace(x), "true") || strings.EqualFold(strings.TrimSpace(x), "yes")
			}
		case "quote", "evidence":
			if s, ok := v.(string); ok {
				quote = oneLine(s, MaxPlanRunes)
			}
		}
	}
	return achieved, quote, nil
}

// ─── Speaking for someone else ───────────────────────────────

// SpeaksAs returns the chat member a reply is written as ("Josh: haha …"):
// small models sometimes continue a group chat with another person's line,
// especially after your own message. "" = the reply is the persona's own.
func SpeaksAs(reply, personaName string, hist []model.Message) string {
	r := strings.ToLower(strings.TrimSpace(strings.TrimLeft(reply, "*\"' ")))
	seen := map[string]bool{}
	for _, m := range hist {
		if m.Speaker == "me" || m.Name == "" {
			continue
		}
		for _, n := range []string{m.Name, strings.Fields(m.Name)[0]} {
			ln := strings.ToLower(n)
			if seen[ln] || strings.EqualFold(n, personaName) {
				continue
			}
			seen[ln] = true
			if strings.HasPrefix(r, ln+":") {
				return n
			}
		}
	}
	return ""
}

// OwnVoiceReminder is appended to the system prompt when a reply was written as someone else.
func OwnVoiceReminder(personaName, other string) string {
	return fmt.Sprintf("\n\nIMPORTANT: you wrote %s's line. Write only your own message as %s — never write what other people say, and no \"Name:\" prefixes.", other, personaName)
}
