// Package lang decides which languages a persona answers and the language
// of each reply (leaf: model + stdlib). Detect is a light classifier: the
// writing system settles most languages (Hebrew, Arabic, Russian, Greek…);
// Latin-script languages are told apart by short, common words and a few
// letters. Short or unclear messages ("ok", "lol", emoji) are not sure and
// never get ignored — the reply then follows the conversation (Main).
package lang

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"whatsappdoppel/internal/core/model"
)

// Language is one language the app knows.
type Language struct {
	Code   string `json:"code"`
	Name   string `json:"name"`   // in English
	Native string `json:"native"` // in itself
}

// All lists the languages Detect can recognise (and the UI offers).
var All = []Language{
	{"en", "English", "English"},
	{"he", "Hebrew", "עברית"},
	{"ar", "Arabic", "العربية"},
	{"ru", "Russian", "Русский"},
	{"uk", "Ukrainian", "Українська"},
	{"es", "Spanish", "Español"},
	{"fr", "French", "Français"},
	{"de", "German", "Deutsch"},
	{"it", "Italian", "Italiano"},
	{"pt", "Portuguese", "Português"},
	{"nl", "Dutch", "Nederlands"},
	{"tr", "Turkish", "Türkçe"},
	{"el", "Greek", "Ελληνικά"},
	{"hi", "Hindi", "हिन्दी"},
	{"th", "Thai", "ไทย"},
	{"zh", "Chinese", "中文"},
	{"ja", "Japanese", "日本語"},
	{"ko", "Korean", "한국어"},
}

// Default is what a chat speaks when nothing is set.
var Default = []string{"en", "he"}

// Known reports whether code is one of All.
func Known(code string) bool {
	return slices.ContainsFunc(All, func(l Language) bool { return l.Code == code })
}

// Name is the English name of a language ("" for an unknown code).
func Name(code string) string {
	for _, l := range All {
		if l.Code == code {
			return l.Name
		}
	}
	return ""
}

// ─── settings ────────────────────────────────────────────────

// Effective is the chat's language preferences: the chat's own block when it
// has one, else the app's for its kind, with the defaults filled in.
func Effective(app model.LanguagePrefs, chat *model.LanguagePrefs) model.LanguagePrefs {
	p := app
	if chat != nil {
		p = *chat
	}
	p.Languages = slices.Clone(p.Languages)
	p.Understands = slices.Clone(p.Understands)
	if len(p.Languages) == 0 {
		p.Languages = slices.Clone(Default)
	}
	if p.Others == "" {
		p.Others = model.LangIgnore
	}
	if p.Reply == "" {
		p.Reply = model.LangMatch
	}
	// The language it replies in always counts as spoken.
	if p.Reply != model.LangMatch && !slices.Contains(p.Languages, p.Reply) {
		p.Languages = append(p.Languages, p.Reply)
	}
	// What it replies to or in, it understands.
	if len(p.Understands) > 0 {
		for _, c := range p.Languages {
			if !slices.Contains(p.Understands, c) {
				p.Understands = append(p.Understands, c)
			}
		}
	}
	return p
}

// Understood reports whether it can read code (effective preferences).
func Understood(p model.LanguagePrefs, code string) bool {
	return len(p.Understands) == 0 || slices.Contains(p.Understands, code)
}

// Speaks reports whether the (effective) preferences include code.
func Speaks(p model.LanguagePrefs, code string) bool {
	return slices.Contains(p.Languages, code)
}

// Ignores reports whether a message detected as code (sure) is left alone.
func Ignores(p model.LanguagePrefs, code string) bool {
	return code != "" && (!Understood(p, code) || p.Others != model.LangAnswer && !Speaks(p, code))
}

// Validate checks a preferences block (zero values are fine).
func Validate(p model.LanguagePrefs) error {
	for _, list := range [][]string{p.Languages, p.Understands} {
		seen := map[string]bool{}
		for _, c := range list {
			if !Known(c) {
				return fmt.Errorf("unknown language %q", c)
			}
			if seen[c] {
				return fmt.Errorf("language %q is listed twice", c)
			}
			seen[c] = true
		}
	}
	if p.Others != "" && p.Others != model.LangIgnore && p.Others != model.LangAnswer {
		return errors.New("others must be ignore or answer")
	}
	if p.Reply != "" && p.Reply != model.LangMatch && !Known(p.Reply) {
		return fmt.Errorf("reply must be match or a language code, not %q", p.Reply)
	}
	return nil
}

// ValidPerson checks a person's reply language ("" = the chat's choice).
func ValidPerson(s string) bool { return s == "" || s == model.LangMatch || Known(s) }

// Reply picks the language of a reply: the person's choice, else the chat's;
// "match" uses the language of their message (msg), else the conversation's
// (conv), as long as it is spoken here (or other languages are answered),
// else the first language of the chat.
func Reply(p model.LanguagePrefs, person, msg, conv string) string {
	choice := p.Reply
	if person != "" {
		choice = person
	}
	if choice != "" && choice != model.LangMatch {
		return choice
	}
	for _, c := range []string{msg, conv} {
		if c != "" && Understood(p, c) && (Speaks(p, c) || p.Others == model.LangAnswer) {
			return c
		}
	}
	if len(p.Languages) > 0 {
		return p.Languages[0]
	}
	return Default[0]
}

// Code is the code of a language's English name ("" for an unknown name).
func Code(name string) string {
	for _, l := range All {
		if l.Name == name {
			return l.Code
		}
	}
	return ""
}

// Fixed reports whether the language was chosen (for the person or the chat)
// rather than following the conversation: a rule, not a preference.
func Fixed(p model.LanguagePrefs, person string) bool {
	choice := p.Reply
	if person != "" {
		choice = person
	}
	return choice != "" && choice != model.LangMatch
}

// ─── detection ───────────────────────────────────────────────

// minLatinWords is how many words a Latin-script text needs before its
// language is taken as sure, and minLatinScore how much evidence (two
// common words, or a tell-tale letter and a word): one shared word like
// "a" or "come" must never make an English message "sure" in another
// language (it would be ignored).
const (
	minLatinWords = 2
	minLatinScore = 2
)

// Detect returns the main language of text and whether it is sure. Not sure
// = too short, only emoji/numbers, or Latin words it can't place.
func Detect(text string) (string, bool) {
	counts := map[string]int{}
	latin, letters := 0, 0
	kana := false
	for _, r := range text {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		switch {
		case unicode.Is(unicode.Hebrew, r):
			counts["he"]++
		case unicode.Is(unicode.Arabic, r):
			counts["ar"]++
		case unicode.Is(unicode.Cyrillic, r):
			counts["ru"]++
		case unicode.Is(unicode.Greek, r):
			counts["el"]++
		case unicode.Is(unicode.Devanagari, r):
			counts["hi"]++
		case unicode.Is(unicode.Thai, r):
			counts["th"]++
		case unicode.Is(unicode.Hangul, r):
			counts["ko"]++
		case unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			counts["ja"]++
			kana = true
		case unicode.Is(unicode.Han, r):
			counts["zh"]++
		case unicode.Is(unicode.Latin, r):
			latin++
		}
	}
	if letters < 2 {
		return "", false
	}
	if kana { // Japanese mixes kanji with kana
		counts["ja"] += counts["zh"]
		delete(counts, "zh")
	}
	best, n := "", 0
	for c, k := range counts {
		if k > n || (k == n && c < best) {
			best, n = c, k
		}
	}
	if n > latin {
		if best == "ru" && hasAny(text, "іїєґІЇЄҐ") {
			best = "uk"
		}
		return best, n*2 >= letters // most of the letters
	}
	return detectLatin(text)
}

// detectLatin scores the Latin-script languages by their common words
// (shared words count less) and tell-tale letters.
func detectLatin(text string) (string, bool) {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && r != '\''
	})
	scores := map[string]float64{}
	for _, w := range words {
		w = strings.Trim(w, "'")
		if langs := stopwords[w]; len(langs) > 0 {
			for _, l := range langs {
				scores[l] += 1 / float64(len(langs))
			}
		}
	}
	for l, letters := range hints {
		if hasAny(text, letters) {
			scores[l] += 1.5
		}
	}
	best, second := "", 0.0
	top := 0.0
	for l, s := range scores {
		switch {
		case s > top || (s == top && l < best):
			second, top, best = top, s, l
		case s > second:
			second = s
		}
	}
	if best == "" {
		return "", false
	}
	return best, top >= minLatinScore && top >= 1.5*second && len(words) >= minLatinWords
}

// alphabets are the languages with their own writing system.
var alphabets = map[string][]*unicode.RangeTable{
	"he": {unicode.Hebrew}, "ar": {unicode.Arabic}, "ru": {unicode.Cyrillic}, "uk": {unicode.Cyrillic},
	"el": {unicode.Greek}, "hi": {unicode.Devanagari}, "th": {unicode.Thai}, "ko": {unicode.Hangul},
	"ja": {unicode.Hiragana, unicode.Katakana, unicode.Han}, "zh": {unicode.Han},
}

// OffAlphabet reports whether text is written outside code's alphabet: under
// half its letters are in it. It catches what Detect can't place ("sure
// thing mate" when the rule is Russian). Single words like "ok" pass, and so
// does any text for a language written in Latin letters.
func OffAlphabet(text, code string) bool {
	tables := alphabets[code]
	if tables == nil || len(strings.Fields(text)) < 2 {
		return false
	}
	in, letters := 0, 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsOneOf(tables, r) {
				in++
			}
		}
	}
	return letters >= 2 && in*2 < letters
}

// UsesAlphabet reports whether text has any letter of code's alphabet (always
// true for a language written in Latin letters).
func UsesAlphabet(text, code string) bool {
	tables := alphabets[code]
	return tables == nil || strings.ContainsFunc(text, func(r rune) bool { return unicode.IsOneOf(tables, r) })
}

func hasAny(s, chars string) bool { return strings.ContainsAny(s, chars) }

// Main is the conversation's language: the most frequent sure language among
// texts ("" when none is sure). Ties go to the latest text.
func Main(texts []string) string {
	count := map[string]int{}
	best, n := "", 0
	for _, t := range texts {
		if c, sure := Detect(t); sure {
			count[c]++
			if count[c] >= n {
				best, n = c, count[c]
			}
		}
	}
	return best
}

// hints are letters that point to one Latin-script language.
var hints = map[string]string{
	"es": "ñ¿¡",
	"pt": "ãõ",
	"de": "ß",
	"tr": "ışğİ",
	"fr": "œ",
}

// stopwords maps common short words to the languages that use them.
var stopwords = func() map[string][]string {
	lists := map[string]string{
		"en": "a come the and you is are to of it that what my me this for with have was not but so do how we your just be on in at can will yes hi hey thanks good yeah im i'm dont don't i it's what's there they he she would about know like",
		"es": "el la los las de que y en un una es por para con pero como qué sí hola gracias está muy yo tu mi también estoy eres",
		"fr": "le la les de des et est un une je tu il que qui pas pour avec mais oui merci bonjour c'est ça vous nous suis très",
		"de": "der die das und ist ich du nicht ein eine zu mit ja nein danke auch was wie bin hast sie wir aber",
		"it": "il lo la che e è di un una non per con sono ciao grazie sì come cosa ma anche sei",
		"pt": "o a os as de que e é um uma não para com você obrigado olá sim mas eu está tudo muito",
		"nl": "de het een en is ik je niet dat van wat hoe ja nee dank ook met jij zijn",
		"tr": "ve bir bu ne mi değil ben sen var yok evet hayır merhaba teşekkür için çok nasıl",
	}
	out := map[string][]string{}
	for l, ws := range lists {
		for _, w := range strings.Fields(ws) {
			out[w] = append(out[w], l)
		}
	}
	return out
}()
