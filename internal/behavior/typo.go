package behavior

import (
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
)

// typoKinds is the order Sampler.Index picks from.
var typoKinds = []string{TypoSwap, TypoDrop, TypoDouble, TypoNear}

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
	Kind  string // swap|drop|double|neighbour
}

type wordSpan struct{ start, end int } // byte offsets of an ASCII word

// typoCandidates lists the words that may get a typo.
func typoCandidates(text string) []wordSpan {
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
			if len(word) >= 4 && !glued && strings.ToUpper(word) != word {
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
func FixBubble(t Typo) string { return "*" + t.Word }
