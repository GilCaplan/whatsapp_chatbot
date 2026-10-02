// Package goals is the pure logic of goal pursuit (leaf: imports only model
// and persona): resolving a chat's goal settings, recognising "say the word"
// goals and spotting them in messages, the leak check for replies, checking
// planner evidence, and the API view of a chat's goal.
package goals

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/persona"
)

// ─── Settings ────────────────────────────────────────────────

// Config is a chat's resolved goal: text and how to pursue it.
type Config struct {
	Text            string
	Source          string // chat|persona|default
	Style           string // subtle|balanced|direct
	StyleSource     string // chat|persona
	PlanAhead       bool
	PlanAheadSource string // chat|persona
	AfterReached    string // relax|continue
}

// NormalizeStyle maps unknown/empty styles to subtle.
func NormalizeStyle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if slices.Contains(model.GoalStyles, s) {
		return s
	}
	return model.GoalStyleSubtle
}

// Resolve combines the chat's overrides with the persona's goal settings.
func Resolve(p model.Persona, chat model.ChatAssignment) Config {
	g := Config{Text: Text(p, chat), Source: "default", Style: NormalizeStyle(p.GoalStyle), StyleSource: "persona",
		PlanAhead: p.GoalPlanAhead == nil || *p.GoalPlanAhead, PlanAheadSource: "persona", AfterReached: model.GoalAfterRelax}
	switch {
	case strings.TrimSpace(chat.GoalOverride) != "":
		g.Source = "chat"
	case strings.TrimSpace(p.Goal) != "":
		g.Source = "persona"
	}
	if chat.GoalStyle != nil && slices.Contains(model.GoalStyles, *chat.GoalStyle) {
		g.Style, g.StyleSource = *chat.GoalStyle, "chat"
	}
	if chat.GoalPlanAhead != nil {
		g.PlanAhead, g.PlanAheadSource = *chat.GoalPlanAhead, "chat"
	}
	if p.GoalAfterReached == model.GoalAfterContinue {
		g.AfterReached = model.GoalAfterContinue
	}
	return g
}

// Text returns the chat's goal override, else the persona goal, else the default.
func Text(p model.Persona, chat model.ChatAssignment) string {
	if t := strings.TrimSpace(chat.GoalOverride); t != "" {
		return t
	}
	if t := strings.TrimSpace(p.Goal); t != "" {
		return t
	}
	return persona.DefaultGoal
}

// Relaxed reports whether the persona stops pursuing a goal that was reached.
func (g Config) Relaxed(reached bool) bool {
	return reached && g.AfterReached != model.GoalAfterContinue
}

// ─── "Say the word" goals ────────────────────────────────────

// SayWord is a parsed "get X to say Y" goal.
type SayWord struct {
	Word string // the word or phrase they have to say
	Who  string // who has to say it ("" = anyone in the chat)
}

var (
	sayQuotedRe = regexp.MustCompile(`(?i)\b(?:say|says|saying|write|type|use|mention)\b[^"“”'‘’]{0,30}?["“'‘]([^"”'’]{1,40})["”'’]`)
	sayWordRe   = regexp.MustCompile(`(?i)\b(?:say|says|saying|write|type|use|mention)\s+(?:the\s+)?(?:word|name|phrase)\s+([\p{L}\p{N}][\p{L}\p{N}'’-]*)`)
	sayBareRe   = regexp.MustCompile(`(?i)\b(?:say|write|type)\s+([\p{L}\p{N}][\p{L}\p{N}'’-]*)\s*[.!]?\s*$`)
	sayWhoRe    = regexp.MustCompile(`(?i)\b(?:get|make|have|let|convince|trick|persuade)\s+(.{1,40}?)\s+(?:to\s+)?(?:say|write|type|use|mention)\b`)

	heQuotedRe = regexp.MustCompile(`(?:להגיד|לומר|יגיד|תגיד|שיגיד|שתגיד|יגידו|שיגידו|לכתוב|שיכתוב|שתכתוב)[^"“”'׳״]{0,20}?["“”'׳״]([^"“”'׳״]{1,40})["“”'׳״]`)
	heWordRe   = regexp.MustCompile(`(?:להגיד|לומר|יגיד|תגיד|שיגיד|שתגיד|יגידו|שיגידו|לכתוב|שיכתוב|שתכתוב)\s+(?:את\s+)?(?:המילה|המשפט|השם)\s+([\p{L}\p{N}]+)`)
	heWhoRe    = regexp.MustCompile(`לגרום\s+ל(\S+)`)
)

var anyoneWords = []string{"them", "him", "her", "someone", "somebody", "anyone", "anybody", "everyone", "everybody", "people", "the group", "they", "you", "my friend", "the other person", "the person"}

// ParseSayWord recognises goals like `get Josh to say the word apple`,
// `make them say "good morning"` or `לגרום ליוסי להגיד את המילה תפוח`.
func ParseSayWord(goal string) (SayWord, bool) {
	goal = strings.TrimSpace(goal)
	var sw SayWord
	for _, re := range []*regexp.Regexp{sayQuotedRe, sayWordRe, heQuotedRe, heWordRe, sayBareRe} {
		if m := re.FindStringSubmatch(goal); m != nil {
			sw.Word = strings.Trim(strings.TrimSpace(m[1]), ".,!?;:")
			break
		}
	}
	if sw.Word == "" || len(wordTokens(sw.Word)) == 0 {
		return SayWord{}, false
	}
	if all := sayWhoRe.FindAllStringSubmatch(goal, -1); all != nil {
		m := all[len(all)-1]
		who := strings.TrimSpace(m[1])
		// "trick Maya into… get Maya Katz to say": keep what follows the last verb.
		for _, v := range []string{"get ", "make ", "have ", "convince ", "persuade "} {
			if j := strings.LastIndex(strings.ToLower(who), " "+v); j >= 0 {
				who = strings.TrimSpace(who[j+1+len(v):])
			}
		}
		if !slices.Contains(anyoneWords, strings.ToLower(who)) {
			sw.Who = who
		}
	} else if m := heWhoRe.FindStringSubmatch(goal); m != nil {
		if who := m[1]; !slices.Contains([]string{"ו", "ה", "הם", "הן", "מישהו", "כולם"}, who) {
			sw.Who = who
		}
	}
	return sw, true
}

// hebrewPrefixes are one-letter prefixes glued to Hebrew words (ו ה ב ל מ ש כ).
const hebrewPrefixes = "והבלמשכ"

func isHebrew(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Hebrew, r) {
			return true
		}
	}
	return false
}

// wordTokens lowercases s and splits it into letter/digit runs.
func wordTokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func tokenMatches(tok, want string, first, last bool) bool {
	if tok == want {
		return true
	}
	if isHebrew(want) {
		// Up to two glued prefixes on the first word: "והתפוח" → "תפוח".
		if first {
			t := tok
			for i := 0; i < 2 && t != ""; i++ {
				r, size := utf8.DecodeRuneInString(t)
				if !strings.ContainsRune(hebrewPrefixes, r) {
					break
				}
				t = t[size:]
				if t == want {
					return true
				}
			}
		}
		return false
	}
	if last {
		// English plurals / possessives: apples, apple's (tokenised as apple + s).
		for _, suf := range []string{"s", "es"} {
			if tok == want+suf {
				return true
			}
		}
	}
	return false
}

// SaysWord reports whether text contains word (or phrase) as whole words,
// ignoring case and punctuation, allowing English plurals and Hebrew prefixes.
func SaysWord(text, word string) bool {
	want := wordTokens(word)
	if len(want) == 0 {
		return false
	}
	toks := wordTokens(text)
	for i := 0; i+len(want) <= len(toks); i++ {
		ok := true
		for j, w := range want {
			if !tokenMatches(toks[i+j], w, j == 0, j == len(want)-1) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// SpeakerMatches reports whether a message by name counts for who ("" = anyone).
// Names match case-insensitively on the full name or the first word.
func SpeakerMatches(name, who string) bool {
	who = strings.ToLower(strings.TrimSpace(who))
	if who == "" {
		return true
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	first := func(s string) string {
		if f := strings.Fields(s); len(f) > 0 {
			return f[0]
		}
		return s
	}
	return name == who || first(name) == who || name == first(who) || first(name) == first(who)
}

// ─── Leak check ──────────────────────────────────────────────

var leakRe = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`\bsay the (word|phrase|name)\b`,
	`\bmy (secret |hidden |real )?(goal|mission|objective|agenda)\b`,
	`\b(secret|hidden) (goal|mission|agenda|objective)\b`,
	`\bi (need|want) (you|him|her|them|\p{Lu}\p{L}+) to (say|tell me|admit)\b`,
	`\b(trying|supposed|meant) to (get|make) (you|him|her|them|\p{Lu}\p{L}+) (to )?(say|tell|admit|agree|come)\b`,
	`\bi('m| am) (supposed|meant|trying) to find out\b`,
	`\bi (need|have) to find out\b`,
}, "|"))

// Leak reports the phrases of reply that give the goal away: meta talk
// about goals/missions or "say the word …", and — for say-the-word goals in
// subtle/balanced style — the target word itself. nil = no leak.
func Leak(g Config, reply string) []string {
	var out []string
	for _, m := range leakRe.FindAllString(strings.ReplaceAll(reply, "’", "'"), -1) {
		out = append(out, m)
	}
	if g.Style != model.GoalStyleDirect {
		if sw, ok := ParseSayWord(g.Text); ok && SaysWord(reply, sw.Word) {
			out = append(out, sw.Word)
		}
	}
	return out
}

// ─── Planner evidence and API view ───────────────────────────

// EvidenceFound looks for the strategist's evidence in the other people's
// messages (not the persona's): a case/punctuation-insensitive substring, or
// at least 70 % of its words (min. 2) inside one message. It returns the
// matching message.
func EvidenceFound(evidence string, msgs []model.Message) (model.Message, bool) {
	ev := wordTokens(evidence)
	if len(ev) == 0 {
		return model.Message{}, false
	}
	evJoined := " " + strings.Join(ev, " ") + " "
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Speaker == "me" {
			continue
		}
		toks := wordTokens(m.Text)
		if strings.Contains(" "+strings.Join(toks, " ")+" ", evJoined) {
			return m, true
		}
		if len(ev) < 2 {
			continue
		}
		hit := 0
		for _, w := range ev {
			if slices.Contains(toks, w) {
				hit++
			}
		}
		if hit*10 >= len(ev)*7 {
			return m, true
		}
	}
	return model.Message{}, false
}

// View resolves a chat's goal settings and status for the API. st is
// the chat's persisted status (nil = none); a status for another goal text is
// ignored (the goal changed, so it starts over).
func View(p model.Persona, c model.ChatAssignment, st *model.GoalStatus) model.ChatGoal {
	cfg := Resolve(p, c)
	out := model.ChatGoal{
		Text: cfg.Text, Source: cfg.Source, Style: cfg.Style, StyleSource: cfg.StyleSource,
		PlanAhead: cfg.PlanAhead, PlanAheadSource: cfg.PlanAheadSource, AfterReached: cfg.AfterReached,
		State: "working",
	}
	if st != nil && st.Goal == cfg.Text {
		g := st.Clone()
		if g.Reached {
			out.State = "reached"
		}
		out.ReachedAt, out.How, out.Evidence, out.LastPlan, out.LastPlanAt = g.ReachedAt, g.How, g.Evidence, g.LastPlan, g.LastPlanAt
	}
	return out
}
