package guard

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// The content filter runs in code on every generated reply, before and after
// translation, so what it stops never depends on a model. Slurs and anything
// about self-harm stop the whole reply; curse words are taken out of it.

// Why a reply was stopped (Content.Block).
const (
	BlockSlur     = "slur"
	BlockSelfHarm = "self-harm"
	BlockEmpty    = "empty" // nothing was left after taking out the curse words
)

// Content is what the filter made of a reply.
type Content struct {
	Text  string // the reply without its curse words
	Block string // why it must not be sent ("" = it may)
}

// wordRe compiles whole-token alternatives. Letter runs are written with + so
// stretched spellings match too.
func wordRe(alts ...string) *regexp.Regexp {
	return regexp.MustCompile(`^(?:` + strings.Join(alts, "|") + `)$`)
}

// hebrewPrefix: Hebrew attaches "the", "and", "to" and so on to the word.
const hebrewPrefix = `[הובלמשכ]?`

var slurRe = wordRe(
	`n+i+g{2,}(a|ah|as|az|e+r+|e+r+s|ur|uh|uz)`, // two g's: Niger is a country
	`f+a+g+(s|ot|ots|got|gots)?`, `k+i+k+e+s?`, `c+h+i+n+k+s?`, `s+p+i+c+s?`,
	`t+r+a+n+n+(y|ie|ies)`, `r+e+t+a+r+d+(s|ed)?`, `c+o{2,}n+s?`, `w+e+t+b+a+c+k+s?`, `p+a+k+i+s?`,
	`н[иеі]гг?[еа]р\p{L}*`, `негритос\p{L}*`, `жид(ы|ов|ам|овск\p{L}*)?`, `хач(и|ей|ам)?`, `чурк\p{L}*`, `пидор\p{L}*`, `пидар\p{L}*`, `педик\p{L}*`,
	hebrewPrefix+`כוש(י|ים|ית|יות)`, hebrewPrefix+`ערבוש\p{L}*`, hebrewPrefix+`מפגר(ים|ת|ות)?`,
)

var curseRe = wordRe(
	`(mother)?f+u+c+k+(s|ed|er|ers|ing|in|ity|head|wit)?`, `b+u+l+s+h+i+t+`, `s+h+i+t+(s|ty|head|hole|ting)?`,
	`b+i+t+c+h+(es|y|ing)?`, `a+s+h+o+l+e+s?`, `a+s+s+h+o+l+e+s?`, `b+a+s+t+a+r+d+s?`, `c+u+n+t+s?`, `d+i+c+k+(s|head|heads)?`,
	`c+o+c+k+s?`, `p+u+s+y+`, `p+u+s+s+y+`, `s+l+u+t+s?`, `w+h+o+r+e+s?`, `w+a+n+k+e+r+s?`, `t+w+a+t+s?`, `w+t+f+`, `s+t+f+u+`, `g+o+d+a+m+n+(it)?`, `g+o+d+d+a+m+n+(it)?`,
	`бл[яе]\p{L}*`, `ху[йеёя]\p{L}*`, `пизд\p{L}*`, `[её]б(а|у|ы|и|ё|о|л)\p{L}*`, `сук(а|и|е|у|ой)`, `мудак\p{L}*`, `мудил\p{L}*`, `гандон\p{L}*`, `залуп\p{L}*`,
	hebrewPrefix+`זונ(ה|ות)`, hebrewPrefix+`שרמוט\p{L}*`, hebrewPrefix+`מניאק\p{L}*`, hebrewPrefix+`כוס(אמא|אמק|אמך|עמק|ומו)\p{L}*`,
	`לזיין`, `תזדיי?ן`, `מזדיי?ן\p{L}*`, hebrewPrefix+`זיון\p{L}*`, hebrewPrefix+`חרא`,
	`كس\p{L}*`, `شرموط\p{L}*`, `خرا`,
	`put(a|as|o|os)`, `mierda`, `cabr[oó]n(es)?`, `merde`, `putain`, `salope`, `schei(ß|ss)e`, `arschloch`, `fotze`,
)

var selfHarmRe = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`\bsuicid\w*`, `\bkys\b`, `\bself[- ]?harm\w*`,
	`\bkill(ing|ed)? (my|your|him|her|them|our)sel(f|ves)\b`, `\bend(ing)? (my|your|it all|our)\s*(life|lives|all)?\b.{0,12}\b(life|lives|all)\b`,
	`\bhang(ing|ed)? (my|your)self\b`, `\bcut(ting)? (my|your)self\b`, `\boff(ing)? (my|your)self\b`,
	`\b(want|wanna|going|gonna|wish|ready|about) (to )?die\b`, `\bi wish i (was|were) dead\b`, `\bdon'?t want to (live|be alive)\b`,
	`התאבד\p{L}*`, `מתאבד\p{L}*`, `להתאבד`, `אתאבד`, `תתאבד\p{L}*`, `להרוג את עצמ\p{L}*`, `רוצה למות`, `בא לי למות`, `לפגוע בעצמ\p{L}*`,
	`суицид\p{L}*`, `самоубийств\p{L}*`, `поконч\p{L}* с собой`, `хочу умереть`, `убить себя`, `убей себя`, `убью себя`, `повешусь`, `повеситься`,
	`انتحر\p{L}*`, `انتحار`, `suicid\p{L}*`, `selbstmord`, `umbringen`,
}, "|"))

// tokenRe finds words, with the characters people swap letters for.
var tokenRe = regexp.MustCompile(`[\p{L}\p{M}\p{N}*$@!'’-]+`)

// leet maps the usual letter stand-ins.
var leet = strings.NewReplacer("0", "o", "1", "i", "3", "e", "4", "a", "5", "s", "7", "t", "$", "s", "@", "a", "!", "i", "*", "u", "’", "'")

// normalToken is a token as the word lists see it.
func normalToken(tok string) string {
	tok = strings.Trim(tok, "'’-!*")
	return leet.Replace(strings.ToLower(tok))
}

// singleLetters returns words spelled out letter by letter ("n i g g e r").
func singleLetters(tokens []string) []string {
	var out []string
	run := ""
	flush := func() {
		if len([]rune(run)) >= 4 {
			out = append(out, run)
		}
		run = ""
	}
	for _, t := range tokens {
		if len([]rune(t)) == 1 {
			run += t
		} else {
			flush()
		}
	}
	flush()
	return out
}

// Filter checks a reply. extra are your own words, taken out like curse words
// (a whole word, or a phrase anywhere in the text).
func Filter(text string, extra []string) Content {
	if selfHarmRe.MatchString(strings.ReplaceAll(text, "’", "'")) {
		return Content{Text: text, Block: BlockSelfHarm}
	}
	spans := tokenRe.FindAllStringIndex(text, -1)
	var norm []string
	var out strings.Builder
	last := 0
	for _, s := range spans {
		tok := text[s[0]:s[1]]
		if strings.HasPrefix(tok, "@") { // a tag is someone's name
			norm = append(norm, "")
			continue
		}
		n := normalToken(tok)
		norm = append(norm, n)
		if slurRe.MatchString(n) {
			return Content{Text: text, Block: BlockSlur}
		}
		if curseRe.MatchString(n) || slices.ContainsFunc(extra, func(w string) bool { return strings.EqualFold(strings.TrimSpace(w), n) }) {
			out.WriteString(text[last:s[0]])
			last = s[1]
		}
	}
	for _, w := range singleLetters(norm) {
		if slurRe.MatchString(w) {
			return Content{Text: text, Block: BlockSlur}
		}
	}
	out.WriteString(text[last:])
	clean := out.String()
	for _, w := range extra {
		if w = strings.TrimSpace(w); strings.Contains(w, " ") {
			clean = regexp.MustCompile(`(?i)`+regexp.QuoteMeta(w)).ReplaceAllString(clean, "")
		}
	}
	if clean == text {
		return Content{Text: text}
	}
	clean = tidy(clean)
	if !strings.ContainsFunc(clean, unicode.IsLetter) {
		return Content{Text: text, Block: BlockEmpty}
	}
	return Content{Text: clean}
}

var (
	spacesRe     = regexp.MustCompile(`[ \t]{2,}`)
	spacePunctRe = regexp.MustCompile(` +([,.!?;:…])`)
	repeatPunct  = regexp.MustCompile(`([,;:]) *[,;:]+`)
)

// tidy closes the gaps a removed word leaves.
func tidy(s string) string {
	s = spacesRe.ReplaceAllString(s, " ")
	s = spacePunctRe.ReplaceAllString(s, "$1")
	s = repeatPunct.ReplaceAllString(s, "$1")
	s = strings.TrimLeft(strings.TrimSpace(s), ",;:.-– ")
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

var firstPersonRe = regexp.MustCompile(`(?i)(^|[^\p{L}'])(i|i'm|i've|i'd|i'll|my|mine|we|we're|our)($|[^\p{L}'])`)

// minClaimWords: shorter replies say too little about who is talking.
const minClaimWords = 5

// FirstPersonClaim reports whether a reply says something about the persona
// itself, so it can be held against who the persona is. Short replies and
// remarks about the world never count.
func FirstPersonClaim(text string) bool {
	return len(strings.Fields(text)) >= minClaimWords && firstPersonRe.MatchString(strings.ReplaceAll(text, "’", "'"))
}
