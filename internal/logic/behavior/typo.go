package behavior

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Typos (wave 3): now and then one bubble of a reply goes out with a small
// slip — two letters swapped, one dropped or doubled, or a neighbouring key —
// and, depending on typoFixStyle, is corrected with a "*word" bubble or an
// edit. Only plain Latin words of 4+ letters are touched: never @tags, links,
// numbers, ALL-CAPS words or non-Latin text.

// Typo kinds (for tests and activity details).
const (
	TypoSwap   = "swap"
	TypoDrop   = "drop"
	TypoDouble = "double"
	TypoNear   = "neighbour"
	// VariedTypo only: two words run together ("getsome"), the wrong word
	// of a commonly mixed-up pair ("your" for "you're") and a word typed
	// twice ("the the"). Run-together and doubled words are left uncorrected,
	// like people do.
	TypoSpace  = "space"
	TypoWord   = "word"
	TypoRepeat = "repeat"
)

// typoKinds is the order Sampler.Index picks from (MakeTypo: letter slips).
var typoKinds = []string{TypoSwap, TypoDrop, TypoDouble, TypoNear}

// TypoKinds are all the kinds VariedTypo can make.
var TypoKinds = []string{TypoSwap, TypoDrop, TypoDouble, TypoNear, TypoSpace, TypoWord, TypoRepeat}

// FixForms are the shapes of a correction bubble ("*work", "work*", "**work").
var FixForms = []string{"*%s", "%s*", "**%s"}

// mixUps maps a word to the one people type by mistake instead.
var mixUps = map[string]string{
	"you're": "your", "they're": "their", "there": "their", "their": "there", "than": "then", "then": "than",
	"it's": "its", "its": "it's", "where": "were", "were": "where", "too": "to", "lose": "loose", "who's": "whose",
}

var wordWithApostrophe = regexp.MustCompile(`[A-Za-z]+(?:'[A-Za-z]+)?`)

// qwertyNear lists the keys next to each letter on a QWERTY keyboard.
var qwertyNear = map[rune]string{
	'q': "wa", 'w': "qes", 'e': "wrd", 'r': "etf", 't': "ryg", 'y': "tuh", 'u': "yij", 'i': "uok", 'o': "ipl", 'p': "ol",
	'a': "qsz", 's': "adw", 'd': "sfe", 'f': "dgr", 'g': "fht", 'h': "gjy", 'j': "hku", 'k': "jli", 'l': "ko",
	'z': "xa", 'x': "zcs", 'c': "xvd", 'v': "cbf", 'b': "vng", 'n': "bmh", 'm': "nj",
}

// Typo is one slip made by MakeTypo.
type Typo struct {
	Typed string // the whole text as sent (with the slip)
	Word  string // the intended word, for the "*word" correction
	Wrong string // the word as typed
	Kind  string // swap|drop|double|neighbour|space|word|repeat
	NoFix bool   // not worth a correction (run-together or doubled words)
	Form  int    // the correction's shape (FixForms), chosen by the engine
}

type wordSpan struct{ start, end int } // byte offsets of an ASCII word

// typoCandidates lists the words that may get a letter slip.
func typoCandidates(text string) []wordSpan { return wordSpans(text, 4) }

// wordSpans lists the plain ASCII words of at least minLen letters (not in
// links or tags, not ALL CAPS, not glued to digits or other scripts).
func wordSpans(text string, minLen int) []wordSpan {
	var out []wordSpan
	// Skip whole tokens that are links or tags.
	pos := 0
	for _, tok := range strings.Fields(text) {
		i := strings.Index(text[pos:], tok)
		if i < 0 {
			break
		}
		start := pos + i
		pos = start + len(tok)
		low := strings.ToLower(tok)
		if strings.HasPrefix(tok, "@") || strings.Contains(low, "://") || strings.HasPrefix(low, "www.") ||
			strings.Contains(low, ".com") || strings.Contains(low, "wa.me") || strings.Contains(tok, "@") {
			continue
		}
		// ASCII letter runs inside the token.
		j := 0
		for j < len(tok) {
			if !isASCIILetter(tok[j]) {
				j++
				continue
			}
			k := j
			for k < len(tok) && isASCIILetter(tok[k]) {
				k++
			}
			word := tok[j:k]
			// A letter run glued to digits or non-ASCII letters is not a plain word.
			glued := (j > 0 && !isBoundary(tok[j-1])) || (k < len(tok) && !isBoundary(tok[k]))
			if len(word) >= minLen && !glued && strings.ToUpper(word) != word {
				out = append(out, wordSpan{start + j, start + k})
			}
			j = k
		}
	}
	return out
}

func isASCIILetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }

// isBoundary: punctuation and apostrophes end a word; digits and non-ASCII
// bytes (Hebrew, accents) glue to it.
func isBoundary(b byte) bool {
	if b >= 0x80 || (b >= '0' && b <= '9') || b == '_' {
		return false
	}
	return true
}

// MakeTypo puts one realistic slip into text: s picks the word and the kind
// of slip. ok is false when text has no word that can take a typo.
func MakeTypo(text string, s Sampler) (Typo, bool) {
	cands := typoCandidates(text)
	if len(cands) == 0 {
		return Typo{}, false
	}
	w := cands[s.Index(len(cands))]
	word := text[w.start:w.end]
	start := s.Index(len(typoKinds))
	for n := 0; n < len(typoKinds); n++ {
		kind := typoKinds[(start+n)%len(typoKinds)]
		if wrong, ok := slip(word, kind, s); ok && wrong != word {
			return Typo{Typed: text[:w.start] + wrong + text[w.end:], Word: word, Wrong: wrong, Kind: kind}, true
		}
	}
	return Typo{}, false
}

// slip applies one kind of typo to an ASCII word (never the first letter,
// so the word stays recognisable).
func slip(word, kind string, s Sampler) (string, bool) {
	b := []byte(word)
	n := len(b)
	switch kind {
	case TypoSwap:
		// positions 1..n-2 swap with the next letter, when different
		var idx []int
		for i := 1; i < n-1; i++ {
			if lower(b[i]) != lower(b[i+1]) {
				idx = append(idx, i)
			}
		}
		if len(idx) == 0 {
			return "", false
		}
		i := idx[s.Index(len(idx))]
		b[i], b[i+1] = b[i+1], b[i]
		return string(b), true
	case TypoDrop:
		i := 1 + s.Index(n-1)
		return string(b[:i]) + string(b[i+1:]), true
	case TypoDouble:
		var idx []int
		for i := 1; i < n; i++ {
			if lower(b[i]) != lower(b[i-1]) {
				idx = append(idx, i)
			}
		}
		if len(idx) == 0 {
			return "", false
		}
		i := idx[s.Index(len(idx))]
		return string(b[:i+1]) + string(b[i:]), true
	case TypoNear:
		i := 1 + s.Index(n-1)
		near := qwertyNear[rune(lower(b[i]))]
		if near == "" {
			return "", false
		}
		c := near[s.Index(len(near))]
		if unicode.IsUpper(rune(b[i])) {
			c = byte(unicode.ToUpper(rune(c)))
		}
		b[i] = c
		return string(b), true
	}
	return "", false
}

func lower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 'a' - 'A'
	}
	return b
}

// FixBubble is the correction bubble for a typo ("*tomorrow").
func FixBubble(t Typo) string { return FixText(t, 0) }

// FixText is the correction bubble in one of FixForms.
func FixText(t Typo, form int) string {
	if form < 0 || form >= len(FixForms) {
		form = 0
	}
	return fmt.Sprintf(FixForms[form], t.Word)
}

// VariedTypo is MakeTypo over every kind (TypoKinds), skipping the kinds
// avoid reports (nil = none): the engine uses it so one chat doesn't see the
// same kind of slip again and again.
func VariedTypo(text string, s Sampler, avoid func(kind string) bool) (Typo, bool) {
	start := s.Index(len(TypoKinds))
	for n := range TypoKinds {
		kind := TypoKinds[(start+n)%len(TypoKinds)]
		if avoid != nil && avoid(kind) {
			continue
		}
		if t, ok := typoOfKind(text, kind, s); ok {
			return t, true
		}
	}
	return Typo{}, false
}

func typoOfKind(text, kind string, s Sampler) (Typo, bool) {
	switch kind {
	case TypoSpace:
		ws := wordSpans(text, 2)
		var gaps []int // index of the space between two words
		for i := 1; i < len(ws); i++ {
			if ws[i].start == ws[i-1].end+1 && text[ws[i-1].end] == ' ' {
				gaps = append(gaps, i)
			}
		}
		if len(gaps) == 0 {
			return Typo{}, false
		}
		i := gaps[s.Index(len(gaps))]
		a, b := ws[i-1], ws[i]
		word := text[a.start:b.end]
		wrong := text[a.start:a.end] + text[b.start:b.end]
		return Typo{Typed: text[:a.start] + wrong + text[b.end:], Word: word, Wrong: wrong, Kind: kind, NoFix: true}, true
	case TypoRepeat:
		ws := wordSpans(text, 2)
		var short []wordSpan
		for _, w := range ws {
			if w.end-w.start <= 5 {
				short = append(short, w)
			}
		}
		if len(short) == 0 {
			return Typo{}, false
		}
		w := short[s.Index(len(short))]
		word := text[w.start:w.end]
		wrong := word + " " + word
		return Typo{Typed: text[:w.start] + wrong + text[w.end:], Word: word, Wrong: wrong, Kind: kind, NoFix: true}, true
	case TypoWord:
		var spans [][2]int
		for _, loc := range wordWithApostrophe.FindAllStringIndex(text, -1) {
			if _, ok := mixUps[strings.ToLower(text[loc[0]:loc[1]])]; ok && plainToken(text, loc[0]) {
				spans = append(spans, [2]int{loc[0], loc[1]})
			}
		}
		if len(spans) == 0 {
			return Typo{}, false
		}
		sp := spans[s.Index(len(spans))]
		word := text[sp[0]:sp[1]]
		wrong := mixUps[strings.ToLower(word)]
		if unicode.IsUpper(rune(word[0])) {
			wrong = strings.ToUpper(wrong[:1]) + wrong[1:]
		}
		return Typo{Typed: text[:sp[0]] + wrong + text[sp[1]:], Word: word, Wrong: wrong, Kind: kind}, true
	}
	cands := typoCandidates(text)
	if len(cands) == 0 {
		return Typo{}, false
	}
	w := cands[s.Index(len(cands))]
	word := text[w.start:w.end]
	if wrong, ok := slip(word, kind, s); ok && wrong != word {
		return Typo{Typed: text[:w.start] + wrong + text[w.end:], Word: word, Wrong: wrong, Kind: kind}, true
	}
	return Typo{}, false
}

// plainToken reports whether the whitespace-separated token around byte i
// is ordinary text (not a link or a tag).
func plainToken(text string, i int) bool {
	start := strings.LastIndexAny(text[:i], " \t\n") + 1
	end := len(text)
	if j := strings.IndexAny(text[i:], " \t\n"); j >= 0 {
		end = i + j
	}
	tok := strings.ToLower(text[start:end])
	return !strings.Contains(tok, "@") && !strings.Contains(tok, "://") && !strings.HasPrefix(tok, "www.")
}
