package prompt

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
)

// ─── Context systems: prompt parts ───────────────────────────
//
// The persona card, the reply check's retry note and AI judge, structured
// summary facts, the one-call digest and a flowchart drafted from your text.

// ─── Persona card ────────────────────────────────────────────

// Card asks for a short persona card: everything the reply needs about the
// persona, in at most chars characters.
func Card(p model.Persona, chars int) llm.Request {
	sys := fmt.Sprintf("You shorten a character description for a chat bot into a compact persona card of at most %d characters. "+
		"Keep what changes how the character writes: who they are, personality, tone and style, typical words and catch-phrases, language, emoji habit, message length and every rule. "+
		"Drop repetition and anything that doesn't change a reply. Write in the second person (\"You are…\"), plain lines, no markdown, no headings. Answer with the card only.", chars)
	return llm.Request{
		System:      sys,
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: "The character:\n" + strings.TrimSpace(Identity(p))}},
		Temperature: 0.2,
		MaxTokens:   max(120, chars/2),
	}
}

// CleanCard tidies a card answer: no markdown or labels, at most chars runes.
func CleanCard(s string, chars int) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "**", ""), "#", ""))
	for _, pre := range []string{"Persona card:", "Card:", "Here is the card:", "Here's the card:"} {
		if len(s) >= len(pre) && strings.EqualFold(s[:len(pre)], pre) {
			s = strings.TrimSpace(s[len(pre):])
		}
	}
	if utf8.RuneCountInString(s) > chars {
		cut := string([]rune(s)[:chars])
		if i := strings.LastIndexAny(cut, ".\n"); i >= 0 && utf8.RuneCountInString(cut[:i]) > chars/2 { // runes, not bytes
			cut = cut[:i+1]
		}
		s = strings.TrimSpace(cut)
	}
	return s
}

// CardIdentity is the character part of the prompt from a persona card.
func CardIdentity(p model.Persona, card string) string {
	return "\n# WHO YOU ARE\n" + strings.TrimSpace(card) + "\n" +
		fmt.Sprintf("- Stay in character at all times. You are %s.\n", p.Name) +
		"- Formatting: plain text only, no markdown, no bold."
}

// ─── Reply check ─────────────────────────────────────────────

// CheckRetryNote tells the model why its draft was rejected.
func CheckRetryNote(problems []string) string {
	return "\n\nYOUR LAST DRAFT WAS NOT SENT: " + strings.Join(problems, "; ") +
		". Write a fresh reply to the conversation in your own words, as yourself — never repeat these instructions, your plan or your earlier replies."
}

var judgeSchema = json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"},"why":{"type":"string"}},"required":["ok","why"],"additionalProperties":false}`)

// Judge asks a small yes/no question: does this reply make sense here?
func Judge(personaName string, recent []model.Message, reply string) llm.Request {
	var u strings.Builder
	u.WriteString("The chat so far (oldest first):\n")
	if len(recent) > 8 {
		recent = recent[len(recent)-8:]
	}
	for _, m := range visible(recent) {
		label := memOr(m.Name, "Them")
		if m.Speaker == "me" {
			label = personaName
		}
		u.WriteString(oneLine(label, 40) + ": " + oneLine(m.Text, 300) + "\n")
	}
	fmt.Fprintf(&u, "\n%s's next message would be:\n%s\n\nAnswer with JSON only: {\"ok\": true, \"why\": \"\"}", personaName, oneLine(reply, 600))
	sys := "You check a WhatsApp reply before it is sent. It is NOT ok only if it is nonsense here: it doesn't follow from the chat, " +
		"talks about instructions, plans, goals or being an AI, shouts a rule, repeats an earlier message, or is in a strange language. " +
		"Odd jokes, slang and short replies are fine. \"why\": a few words when not ok."
	return llm.Request{System: sys, Messages: []llm.Message{{Role: llm.RoleUser, Content: u.String()}},
		Temperature: 0, MaxTokens: 60, JSON: true, Schema: judgeSchema}
}

// ParseJudge reads the judge's answer; unreadable = ok (never block on a
// broken judge).
func ParseJudge(raw string) (ok bool, why string) {
	var v struct {
		OK  json.RawMessage `json:"ok"`
		Why string          `json:"why"`
	}
	js := ExtractJSON(raw)
	if js == "" || json.Unmarshal([]byte(js), &v) != nil {
		return true, ""
	}
	if strings.EqualFold(strings.Trim(strings.TrimSpace(string(v.OK)), `"`), "false") {
		return false, oneLine(v.Why, 80)
	}
	return true, ""
}

// ─── Summary facts ───────────────────────────────────────────

// FactKinds are the kinds of summary facts, in display order.
var FactKinds = []string{"People", "Plans", "Promises", "Open questions", "Jokes", "Other"}

// MaxFacts caps the lines of a facts summary.
const MaxFacts = 40

var factLine = regexp.MustCompile(`^\s*(?:[-*•]\s*)?([A-Za-z ]{3,20}):\s*(.+)$`)

// SplitFacts reads a facts summary into lines (kind, text).
func SplitFacts(text string) [][2]string {
	var out [][2]string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if m := factLine.FindStringSubmatch(l); m != nil {
			out = append(out, [2]string{factKind(m[1]), oneLine(m[2], 200)})
		} else {
			out = append(out, [2]string{"Other", oneLine(strings.TrimLeft(l, "-*• "), 200)})
		}
	}
	return out
}

func factKind(k string) string {
	k = strings.TrimSpace(k)
	for _, f := range FactKinds {
		if strings.EqualFold(f, k) || strings.EqualFold(strings.TrimSuffix(f, "s"), k) {
			return f
		}
	}
	return "Other"
}

// JoinFacts writes fact lines as the summary text (grouped by kind).
func JoinFacts(lines [][2]string) string {
	var b strings.Builder
	for _, k := range FactKinds {
		for _, l := range lines {
			if l[0] == k && l[1] != "" {
				b.WriteString(k + ": " + l[1] + "\n")
			}
		}
	}
	return strings.TrimSpace(b.String())
}

var factsSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "add": {"type": "array", "items": {"type": "object", "properties": {"kind": {"type": "string", "enum": ["People", "Plans", "Promises", "Open questions", "Jokes", "Other"]}, "text": {"type": "string"}}, "required": ["kind", "text"], "additionalProperties": false}},
    "update": {"type": "array", "items": {"type": "object", "properties": {"line": {"type": "integer"}, "text": {"type": "string"}}, "required": ["line", "text"], "additionalProperties": false}},
    "remove": {"type": "array", "items": {"type": "integer"}}
  },
  "required": ["add", "update", "remove"],
  "additionalProperties": false
}`)

// CompactFacts asks for changes to a facts summary: lines to add, update by
// number, or remove. previous is the current summary (facts lines, or a
// prose summary to turn into facts).
func CompactFacts(in CompactInput) llm.Request {
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
	fmt.Fprintf(&sys, "You keep a list of facts about %s for %s, who is in it and can no longer see the older messages. JSON only.\n\n", where, name)
	sys.WriteString("Kinds: People (who is who), Plans (with dates), Promises (who promised what), Open questions, Jokes (running jokes), Other.\n")
	sys.WriteString("Rules:\n")
	sys.WriteString("- Only what is in the facts so far and the messages. Never guess.\n")
	sys.WriteString("- Change as little as possible: add new facts, update a line only when it changed, remove a line only when it is no longer true or done.\n")
	sys.WriteString("- One short line each, at most 20 words, names instead of pronouns, dates like \"Sat 4 Oct\".\n")
	fmt.Fprintf(&sys, "- Write about %s in the third person by name.\n", name)
	sys.WriteString("- Skip small talk, greetings and moods of the moment.\n")

	var u strings.Builder
	prev := strings.TrimSpace(in.Previous)
	lines := SplitFacts(prev)
	switch {
	case prev == "":
		u.WriteString("Facts so far: none.\n\n")
	case in.PreviousProse:
		u.WriteString("The summary so far is a paragraph; turn it into facts (all as \"add\"):\n" + prev + "\n\n")
	default:
		u.WriteString("Facts so far (by line number):\n")
		for i, l := range lines {
			fmt.Fprintf(&u, "%d. %s: %s\n", i+1, l[0], l[1])
		}
		u.WriteString("\n")
	}
	u.WriteString("Messages to add (oldest first):\n")
	writeTranscript(&u, in.Messages, name, in.ChatName, in.IsGroup, zone, MaxCompactMessages)
	u.WriteString(`
Answer with JSON only: {"add": [{"kind": "Plans", "text": "..."}], "update": [{"line": 2, "text": "..."}], "remove": [3]}`)
	return llm.Request{System: sys.String(), Messages: []llm.Message{{Role: llm.RoleUser, Content: u.String()}},
		Temperature: 0.1, MaxTokens: 700, JSON: true, Schema: factsSchema}
}

// ApplyFacts applies a CompactFacts answer to the current lines: untouched
// lines stay exactly as they were.
func ApplyFacts(previous string, prose bool, raw string) (string, error) {
	js := ExtractJSON(raw)
	if js == "" {
		return "", ErrNoJSON
	}
	var v struct {
		Add []struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
		} `json:"add"`
		Update []struct {
			Line int    `json:"line"`
			Text string `json:"text"`
		} `json:"update"`
		Remove []int `json:"remove"`
	}
	if err := json.Unmarshal([]byte(js), &v); err != nil {
		return "", ErrNoJSON
	}
	var lines [][2]string
	if !prose {
		lines = SplitFacts(previous)
	}
	for _, u := range v.Update {
		if t := oneLine(u.Text, 200); u.Line >= 1 && u.Line <= len(lines) && t != "" {
			lines[u.Line-1][1] = t
		}
	}
	gone := map[int]bool{}
	for _, r := range v.Remove {
		gone[r] = true
	}
	kept := lines[:0]
	for i, l := range lines {
		if !gone[i+1] {
			kept = append(kept, l)
		}
	}
	for _, a := range v.Add {
		if t := oneLine(a.Text, 200); t != "" {
			kept = append(kept, [2]string{factKind(a.Kind), t})
		}
	}
	// Over the cap: the oldest line of the biggest kind goes first (lines are
	// grouped by kind, oldest first within a kind, so the overall age is lost
	// and cutting from the front would always drop People).
	for len(kept) > MaxFacts {
		count := map[string]int{}
		for _, l := range kept {
			count[l[0]]++
		}
		big := kept[0][0]
		for _, k := range FactKinds {
			if count[k] > count[big] {
				big = k
			}
		}
		i := slices.IndexFunc(kept, func(l [2]string) bool { return l[0] == big })
		kept = slices.Delete(kept, i, i+1)
	}
	return JoinFacts(kept), nil
}

// writeTranscript writes messages as "[Mon 15:04] Name: text" lines.
func writeTranscript(u *strings.Builder, msgs []model.Message, persona, chat string, isGroup bool, zone *time.Location, maxN int) {
	if len(msgs) > maxN {
		msgs = msgs[len(msgs)-maxN:]
	}
	contact := memOr(chat, "Them")
	for _, m := range visible(msgs) {
		label := m.Name
		switch {
		case m.Speaker == "me":
			label = persona
		case label == "" && !isGroup:
			label = contact
		case label == "":
			label = "Someone"
		}
		ts := ""
		if !m.TS.IsZero() {
			ts = "[" + m.TS.In(zone).Format("Mon 15:04") + "] "
		}
		u.WriteString(ts + oneLine(label, 40) + ": " + oneLine(m.Text, 400) + "\n")
	}
}

// ─── Digest: background jobs in one call ─────────────────────

// MaxDigestMessages caps the messages one digest reads: enough for the
// largest digestEvery (60 of theirs) plus the persona's replies, so no job's
// cursor moves past messages it never saw.
const MaxDigestMessages = 160

// DigestInput: which jobs the digest does, and their inputs.
type DigestInput struct {
	PersonaName string
	ChatName    string
	IsGroup     bool
	Messages    []model.Message
	Zone        *time.Location
	Now         time.Time

	Memories bool
	Known    []string // memories already noted
	Brief    bool
	Notes    bool
	Owner    string   // the secretary's owner
	Open     []string // notes already taken
}

// Digest builds one request doing the chosen background jobs (memories, the
// brief for other chats, a secretary's notes) on the same messages; each
// job's answer sits under its own key.
func Digest(in DigestInput) llm.Request {
	zone := in.Zone
	if zone == nil {
		zone = time.Local
	}
	name := memOr(in.PersonaName, "the persona")
	var sys strings.Builder
	var keys []string
	props := map[string]json.RawMessage{}
	var format []string
	job := func(key, rules string, schema json.RawMessage, sub string, example string) {
		keys = append(keys, key)
		fmt.Fprintf(&sys, "\n\n=== JOB \"%s\" ===\n%s", key, strings.TrimSpace(rules))
		var s map[string]json.RawMessage
		if json.Unmarshal(schema, &s) == nil {
			if sub == "" {
				props[key] = schema
			} else {
				var p map[string]json.RawMessage
				_ = json.Unmarshal(s["properties"], &p)
				props[key] = p[sub]
			}
		}
		format = append(format, fmt.Sprintf("%q: %s", key, example))
	}
	if in.Memories {
		r := ExtractMemories(name, in.IsGroup, in.ChatName, nil, nil, in.Now.In(zone))
		job("memories", strings.Split(r.System, "Answer with JSON only")[0], memorySchema, "memories",
			`[{"person": "...", "evidence": "...", "text": "...", "kind": "fact|preference|event|relationship", "expires": "", "sensitive": ""}]`)
	}
	if in.Notes {
		r := Notes(NotesInput{Owner: in.Owner, Assistant: name, ChatName: in.ChatName, IsGroup: in.IsGroup, Zone: zone, Now: in.Now})
		job("notes", r.System, notesSchema, "notes", `[{"from": "name", "text": "...", "urgent": false, "callback": "", "meeting": ""}]`)
	}
	if in.Brief {
		r := Brief(BriefInput{PersonaName: name, ChatName: in.ChatName, IsGroup: in.IsGroup, Zone: zone, Now: in.Now})
		people := `[{"name": "...", "note": "at most 12 words"}]`
		if !in.IsGroup {
			people = "[]"
		}
		job("brief", r.System, briefSchema, "",
			`{"topics": ["up to 4"], "commitments": ["up to 3 things `+name+` promised"], "tone": "one or two words", "people": `+people+`, "sensitive": []}`)
	}
	head := fmt.Sprintf("You do %d background jobs at once on the same new messages from a WhatsApp chat. Follow each job's own rules. Most messages contain nothing for most jobs: empty lists are the usual answer.", len(keys))
	var u strings.Builder
	if in.Memories {
		u.WriteString("Already in the notebook (job \"memories\"):\n")
		if len(in.Known) == 0 {
			u.WriteString("(nothing yet)\n")
		}
		known := in.Known
		if len(known) > MaxExtractKnown {
			known = known[len(known)-MaxExtractKnown:]
		}
		for _, k := range known {
			u.WriteString("- " + oneLine(k, 200) + "\n")
		}
		u.WriteString("\n")
	}
	if in.Notes && len(in.Open) > 0 {
		u.WriteString("Notes already taken (job \"notes\"):\n")
		for _, n := range in.Open {
			u.WriteString("- " + oneLine(n, 300) + "\n")
		}
		u.WriteString("\n")
	}
	u.WriteString("New messages (oldest first):\n")
	label := name
	if in.Memories {
		label = name + " (not noted)" // the notebook is only about the others
	}
	writeTranscript(&u, in.Messages, label, in.ChatName, in.IsGroup, zone, MaxDigestMessages)
	u.WriteString("\nAnswer with ONE JSON object only: {" + strings.Join(format, ", ") + "}")

	req := make([]string, 0, len(keys))
	for _, k := range keys {
		req = append(req, fmt.Sprintf("%q", k))
	}
	pj, _ := json.Marshal(props)
	schema := json.RawMessage(`{"type":"object","properties":` + string(pj) + `,"required":[` + strings.Join(req, ",") + `],"additionalProperties":false}`)
	return llm.Request{System: head + sys.String(), Messages: []llm.Message{{Role: llm.RoleUser, Content: u.String()}},
		Temperature: 0.1, MaxTokens: 900, JSON: true, Schema: schema}
}

// DigestRetryNote is added to the digest's system prompt when its answer
// wasn't in the format asked.
const DigestRetryNote = "\n\nIMPORTANT: answer with one JSON object only, exactly in the format asked."

// DigestParts splits a digest answer into each job's JSON (re-wrapped the
// way the job's own parser reads it); a missing job is "".
func DigestParts(raw string) (memories, notes, brief string, err error) {
	js := ExtractJSON(raw)
	if js == "" {
		return "", "", "", ErrNoJSON
	}
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(js), &m) != nil {
		return "", "", "", ErrNoJSON
	}
	if v, ok := m["memories"]; ok {
		memories = `{"memories":` + string(v) + `}`
	}
	if v, ok := m["notes"]; ok {
		notes = `{"notes":` + string(v) + `}`
	}
	if v, ok := m["brief"]; ok {
		brief = string(v)
	}
	return memories, notes, brief, nil
}

// ─── A chart from your words ─────────────────────────────────

var chartSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": {"type": "string"},
    "description": {"type": "string"},
    "nodes": {"type": "array", "items": {"type": "object", "properties": {"id": {"type": "string"}, "block": {"type": "string"}, "label": {"type": "string"}}, "required": ["id", "block", "label"], "additionalProperties": false}},
    "edges": {"type": "array", "items": {"type": "object", "properties": {"from": {"type": "string"}, "to": {"type": "string"}, "label": {"type": "string"}, "dashed": {"type": "boolean"}}, "required": ["from", "to", "label", "dashed"], "additionalProperties": false}}
  },
  "required": ["name", "description", "nodes", "edges"],
  "additionalProperties": false
}`)

// ChartFromText asks for a context-system flowchart made of the catalog's
// blocks, from a plain description.
func ChartFromText(text string, blocks []model.ChartBlock) llm.Request {
	var sys strings.Builder
	sys.WriteString("You draw flowcharts of how a WhatsApp chat bot decides when to reply and what it reads (its context system). Use only these building blocks (id: what it is):\n")
	for _, b := range blocks {
		fmt.Fprintf(&sys, "- %s: %s. %s\n", b.ID, b.Label, b.Description)
	}
	sys.WriteString(`
Rules:
- Start with message_in. End paths with send or quiet.
- Solid wires are the order things happen; "dashed": true for things that feed the reply (prompt parts) or run in the background.
- Short wire labels like "yes", "no", "unsure", "fails". Node ids short, unique, lowercase.
- 6 to 25 nodes. Use custom_step for anything the blocks don't cover, with your own label.
- "name": 2 to 4 words. "description": one sentence.
Answer with JSON only.`)
	return llm.Request{System: sys.String(), Messages: []llm.Message{{Role: llm.RoleUser, Content: "Draw this context system:\n" + oneLine(text, 2000)}},
		Temperature: 0.3, MaxTokens: 1500, JSON: true, Schema: chartSchema}
}

// ParseChart reads a ChartFromText answer.
func ParseChart(raw string) (name, description string, chart model.ContextChart, err error) {
	js := ExtractJSON(raw)
	if js == "" {
		return "", "", chart, ErrNoJSON
	}
	var v struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Nodes       []model.ChartNode `json:"nodes"`
		Edges       []model.ChartEdge `json:"edges"`
	}
	if json.Unmarshal([]byte(js), &v) != nil || len(v.Nodes) == 0 {
		return "", "", chart, ErrNoJSON
	}
	return oneLine(v.Name, 60), oneLine(v.Description, 300), model.ContextChart{Nodes: v.Nodes, Edges: v.Edges}, nil
}
