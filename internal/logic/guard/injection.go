// Package guard detects prompt-injection attempts in incoming chat messages
// and character breaks in model replies.
package guard

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Filter levels (settings.replies.injectionFilter).
const (
	LevelStrict   = "strict"
	LevelBalanced = "balanced"
	LevelOff      = "off"
)

const (
	hardWeight = 3
	softWeight = 1
)

// Result of inspecting one message. Text is always the sanitized text.
type Result struct {
	Text    string   `json:"text"`
	Blocked bool     `json:"blocked"`
	Score   int      `json:"score"`
	Matches []string `json:"matches"`
}

type pattern struct {
	label  string
	re     *regexp.Regexp
	weight int
}

func p(label, expr string, weight int) pattern {
	return pattern{label: label, re: regexp.MustCompile(`(?i)` + expr), weight: weight}
}

// Hard patterns are unambiguous jailbreak phrasing; one is enough to block at "balanced".
// Soft patterns are common in ordinary chat and only add up.
var patterns = []pattern{
	p("ignore previous instructions", `\bignore\s+(all\s+|any\s+)?(of\s+)?(the\s+|your\s+|my\s+|these\s+)?(previous|prior|above|earlier|preceding|former)\s+(instructions?|prompts?|rules|messages|directions)`, hardWeight),
	p("ignore your instructions", `\bignore\s+(all\s+)?your\s+(instructions|rules|programming|guidelines|system\s+prompt)`, hardWeight),
	p("disregard previous", `\bdisregard\s+(all\s+|the\s+|your\s+|any\s+)?(previous|prior|above|earlier|preceding)\b`, hardWeight),
	p("system prompt", `\bsystem\s*prompt\b`, hardWeight),
	p("role tag", `\[/?\s*(system|inst|assistant)\s*\]|</?\s*(system|assistant)\s*>|<\|\s*im_(start|end)\s*\|>`, hardWeight),
	p("role label", `(?m)^\s*(system|assistant)\s*:`, hardWeight),
	p("you are now", `\byou\s+are\s+now\s+(a|an|the|my|dan|in|called|named|no\s+longer)\b`, hardWeight),
	p("you are no longer", `\byou\s+are\s+no\s+longer\b`, hardWeight),
	p("new instructions", `\bnew\s+(instructions?|persona|character|role|rules|identity)\s*:`, hardWeight),
	p("jailbreak mode", `\b(developer|dan|god|sudo|admin|jailbreak)\s+mode\b|\bjailbreak\b|\bdo\s+anything\s+now\b`, hardWeight),
	p("forget your instructions", `\bforget\s+(everything|all)(\s+that)?\s+(you('ve|\s+have)?\s+(know|been\s+told|were\s+told|learned)|your\s+(instructions|rules|programming|training))`, hardWeight),
	p("as an ai", `\bas\s+an\s+ai\b`, hardWeight),
	p("pretend to be an ai", `\bpretend\s+(you\s+are|you're|to\s+be)\s+(an?\s+)?(ai|assistant|chatbot|language\s+model)\b`, hardWeight),
	p("act as an ai", `\bact\s+as\s+(an?\s+)?(ai|assistant|chatbot|language\s+model)\b`, hardWeight),
	p("reveal prompt", `\b(reveal|show|print|repeat|output|tell\s+me)\s+(me\s+)?(your|the)\s+(initial\s+|original\s+|hidden\s+)?(instructions|prompt)\b`, hardWeight),

	p("you must", `\byou\s+must\b`, softWeight),
	p("from now on", `\bfrom\s+now\s+on\b|\bstarting\s+now\b`, softWeight),
	p("override", `\boverrid(e|ing)\b`, softWeight),
	p("simulate", `\bsimulat(e|ion|ing)\b`, softWeight),
	p("reset", `\breset\b`, softWeight),
	p("hypothetically", `\bhypothetical(ly)?\b|\bfor\s+educational\s+purposes\b`, softWeight),
	p("translate:", `\b(translate|decode|encode|execute|run)\s*:`, softWeight),
	p("encoding", `\b(base64|rot13)\b`, softWeight),
	p("code", `\bprint\s*\(|\bconsole\.log\b|\beval\s*\(|<\s*script\b|\bjavascript:`, softWeight),
	p("role play request", `\b(act\s+as|pretend\s+(to\s+be|you\s+are)|your\s+role\s+is|in\s+reality\s+you\s+are|you're\s+actually)\b`, softWeight),
}

var (
	zeroWidth   = strings.NewReplacer("\u200b", "", "\u200c", "", "\u2060", "", "\ufeff", "", "\u00ad", "")
	fenceRe     = regexp.MustCompile("`+")
	roleTagRe   = regexp.MustCompile(`(?i)</?\s*(system|assistant|user|inst|im_start|im_end)\b[^>]*>|<\|[^|>]{0,32}\|>|\[/?\s*(system|assistant|user|inst)\s*\]`)
	ruleRe      = regexp.MustCompile(`#{3,}|-{3,}|={3,}`)
	hSpaceRe    = regexp.MustCompile(`[ \t\f\v]+`)
	newlinesRe  = regexp.MustCompile(`\s*\n\s*`)
	threshold   = map[string]int{LevelStrict: 2, LevelBalanced: hardWeight}
	defaultTres = hardWeight
)

// Inspect scores text for injection attempts and returns the sanitized text.
// level is strict (block at score ≥ 2), balanced (≥ 3, i.e. one hard match) or off (never block).
func Inspect(text, level string) Result {
	norm := zeroWidth.Replace(text)
	res := Result{Text: Sanitize(text), Matches: []string{}}
	for _, pt := range patterns {
		if pt.re.MatchString(norm) {
			res.Score += pt.weight
			res.Matches = append(res.Matches, pt.label)
		}
	}
	if level == LevelOff {
		return res
	}
	t, ok := threshold[level]
	if !ok {
		t = defaultTres
	}
	res.Blocked = res.Score >= t
	return res
}

// Sanitize strips formatting tricks (code fences, role tags, rule lines) and
// collapses whitespace. It never removes ordinary words.
func Sanitize(text string) string {
	s := zeroWidth.Replace(text)
	s = strings.ToValidUTF8(s, "")
	s = fenceRe.ReplaceAllString(s, "")
	s = roleTagRe.ReplaceAllString(s, " ")
	s = ruleRe.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = hSpaceRe.ReplaceAllString(s, " ")
	s = newlinesRe.ReplaceAllString(s, "\n")
	return strings.TrimSpace(s)
}

// Truncate shortens s to at most n runes, appending "…" when cut. Never splits UTF-8.
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}
