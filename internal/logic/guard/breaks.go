package guard

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// breakRe matches generic phrases that mean the model stepped out of character.
var breakRe = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`\bas an ai\b`,
	`\bas a (large )?language model\b`,
	`\bi('m| am) (just |only )?(an? )?(ai|artificial intelligence|language model|large language model|chatbot|virtual assistant|ai assistant)\b`,
	`\bi('m| am) (just |only )?an assistant\b`,
	`\bi (cannot|can't|can not|won't) (pretend|roleplay|role-play)\b`,
	`\bi('m| am) not (able|allowed) to (pretend|roleplay|role-play)\b`,
	`\bi('m| am) now (an? )?(ai|assistant|chatbot|language model|dan|playing|acting as|in (developer|dan|god) mode)\b`,
	`\bmy (instructions|system prompt|guidelines say)\b`,
	`\b(trained|created|developed|made) by (openai|anthropic|meta|google)\b`,
	`\bi('m| am) (claude|chatgpt|gpt-?\d|llama)\b`,
}, "|"))

// BrokeCharacter reports whether reply reads like the model dropped the persona
// ("as an AI…", "I'm not Leo", …). personaName adds name-specific denials.
func BrokeCharacter(reply, personaName string) bool {
	r := strings.ReplaceAll(reply, "’", "'")
	if breakRe.MatchString(r) {
		return true
	}
	lower := strings.ToLower(r)
	for _, name := range nameVariants(personaName) {
		for _, denial := range []string{"i'm not ", "i am not ", "my name is not ", "my name isn't ", "i'm not really ", "i am not really "} {
			if containsWord(lower, denial+name) {
				return true
			}
		}
	}
	return false
}

// nameVariants returns the lowercase full name and its first word.
func nameVariants(name string) []string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil
	}
	out := []string{name}
	if f := strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }); len(f) > 0 && f[0] != name {
		out = append(out, f[0])
	}
	return out
}

// containsWord reports whether needle occurs in s with no letter/digit directly
// before or after it (a Unicode-aware word-boundary match). Both must be lowercase.
func containsWord(s, needle string) bool {
	if needle == "" {
		return false
	}
	for i := 0; ; {
		j := strings.Index(s[i:], needle)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(needle)
		if !wordRuneBefore(s, start) && !wordRuneAfter(s, end) {
			return true
		}
		i = start + 1
		if i >= len(s) {
			return false
		}
	}
}

func wordRuneBefore(s string, i int) bool {
	if i == 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return isWordRune(r)
}

func wordRuneAfter(s string, i int) bool {
	for _, r := range s[i:] {
		return isWordRune(r)
	}
	return false
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// ContainsName reports whether text mentions name as a whole word, case-insensitively.
// Multi-word names also match on their first word ("Chad" for "Chad Remington").
func ContainsName(text, name string) bool {
	lower := strings.ToLower(text)
	for _, v := range nameVariants(name) {
		if containsWord(lower, v) {
			return true
		}
	}
	return false
}
