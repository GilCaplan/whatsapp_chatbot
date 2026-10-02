package prompt

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"whatsappdoppel/internal/model"
)

// ─── emoji detection ─────────────────────────────────────────
//
// An emoji is a cluster: a base (pictograph, a text-style symbol followed by
// U+FE0F, a keycap, a regional-indicator pair or a tag-sequence flag) plus
// variation selectors, skin-tone modifiers, keycap marks, tag characters and
// ZWJ-joined further pictographs ("👩🏽‍💻", "🏳️‍🌈", "❤️‍🔥", "🇮🇱", "1️⃣").

const (
	zwj       = '‍'
	vs15      = '︎'
	vs16      = '️'
	keycap    = '⃣'
	tagCancel = '\U000E007F'
)

func isSkinTone(r rune) bool   { return r >= 0x1F3FB && r <= 0x1F3FF }
func isRegional(r rune) bool   { return r >= 0x1F1E6 && r <= 0x1F1FF }
func isTagChar(r rune) bool    { return r >= 0xE0020 && r <= 0xE007F }
func isKeycapBase(r rune) bool { return (r >= '0' && r <= '9') || r == '#' || r == '*' }

// bmpPictographic lists the BMP Extended_Pictographic code points that are
// shown as emoji on their own (no U+FE0F needed in chat apps).
var bmpPictographic = [][2]rune{
	{0x231A, 0x231B}, {0x2328, 0x2328}, {0x2388, 0x2388}, {0x23CF, 0x23CF}, {0x23E9, 0x23F3}, {0x23F8, 0x23FA},
	{0x2600, 0x2604}, {0x2607, 0x2612}, {0x2614, 0x2685}, {0x2690, 0x2705}, {0x2708, 0x2712}, {0x2714, 0x2714},
	{0x2716, 0x2716}, {0x271D, 0x271D}, {0x2721, 0x2721}, {0x2728, 0x2728}, {0x2733, 0x2734}, {0x2744, 0x2744},
	{0x2747, 0x2747}, {0x274C, 0x274C}, {0x274E, 0x274E}, {0x2753, 0x2755}, {0x2757, 0x2757}, {0x2763, 0x2767},
	{0x2795, 0x2797}, {0x27A1, 0x27A1}, {0x27B0, 0x27B0}, {0x27BF, 0x27BF}, {0x2B1B, 0x2B1C}, {0x2B50, 0x2B50},
	{0x2B55, 0x2B55},
}

// textStyle lists symbols that are ordinary text unless followed by U+FE0F
// (© ® ‼ ⁉ ™ ℹ arrows, Ⓜ, small squares/triangles, ★, 〰, 〽, ㊗, ㊙).
var textStyle = [][2]rune{
	{0x00A9, 0x00A9}, {0x00AE, 0x00AE}, {0x203C, 0x203C}, {0x2049, 0x2049}, {0x2122, 0x2122}, {0x2139, 0x2139},
	{0x2194, 0x2199}, {0x21A9, 0x21AA}, {0x24C2, 0x24C2}, {0x25AA, 0x25AB}, {0x25B6, 0x25B6}, {0x25C0, 0x25C0},
	{0x25FB, 0x25FE}, {0x2605, 0x2606}, {0x2934, 0x2935}, {0x2B05, 0x2B07}, {0x3030, 0x3030}, {0x303D, 0x303D},
	{0x3297, 0x3297}, {0x3299, 0x3299},
}

func inRanges(r rune, rs [][2]rune) bool {
	for _, x := range rs {
		if r >= x[0] && r <= x[1] {
			return true
		}
	}
	return false
}

// isPictograph reports whether r is an emoji on its own.
func isPictograph(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF:
		return !isSkinTone(r) && !isRegional(r)
	case r >= 0x1FC00 && r <= 0x1FFFD:
		return true
	case r >= 0x2300 && r <= 0x2BFF:
		return inRanges(r, bmpPictographic)
	}
	return false
}

// emojiSpan is the byte range [start, end) of one emoji cluster.
type emojiSpan struct{ start, end int }

// emojiAt returns the length in bytes of the emoji cluster starting at s[0]
// (0 when s does not start with an emoji).
func emojiAt(s string) int {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return 0
	}
	next := func(i int) (rune, int) { return utf8.DecodeRuneInString(s[i:]) }
	i := 0
	switch {
	case isRegional(r):
		i = n
		if r2, n2 := next(i); isRegional(r2) {
			i += n2
		}
		return i
	case isKeycapBase(r):
		i = n
		r2, n2 := next(i)
		if r2 == vs16 {
			i += n2
			r2, n2 = next(i)
		}
		if r2 == keycap {
			return i + n2
		}
		return 0
	case isPictograph(r):
		i = n
	case inRanges(r, textStyle):
		if r2, n2 := next(n); r2 == vs16 {
			i = n + n2
		} else {
			return 0
		}
	case isSkinTone(r):
		i = n // a stray modifier counts as emoji too
	default:
		return 0
	}
	// Extend: selectors, modifiers, keycap marks, tags, ZWJ + pictograph.
	for i < len(s) {
		r2, n2 := next(i)
		switch {
		case r2 == vs16 || r2 == vs15 || isSkinTone(r2) || r2 == keycap || isTagChar(r2):
			i += n2
		case r2 == zwj:
			r3, n3 := next(i + n2)
			if isPictograph(r3) || inRanges(r3, textStyle) {
				i += n2 + n3
			} else {
				return i
			}
		default:
			return i
		}
	}
	return i
}

func emojiSpans(s string) []emojiSpan {
	var out []emojiSpan
	for i := 0; i < len(s); {
		if n := emojiAt(s[i:]); n > 0 {
			out = append(out, emojiSpan{i, i + n})
			i += n
			continue
		}
		_, n := utf8.DecodeRuneInString(s[i:])
		i += n
	}
	return out
}

// Emojis lists the emoji clusters in s, in order.
func Emojis(s string) []string {
	spans := emojiSpans(s)
	out := make([]string, len(spans))
	for i, sp := range spans {
		out[i] = s[sp.start:sp.end]
	}
	return out
}

// CountEmoji is the number of emoji clusters in s.
func CountEmoji(s string) int { return len(emojiSpans(s)) }

// KeepEmoji removes every emoji cluster of s except the first keep ones.
// Only the whitespace around a removed emoji is tidied (no double space, no
// space before closing punctuation or a line end, no empty line left by an
// emoji-only line); the rest of the text is untouched. keep <= 0 strips all.
func KeepEmoji(s string, keep int) string {
	keep = max(keep, 0)
	spans := emojiSpans(s)
	if len(spans) <= keep {
		return s
	}
	return removeSpans(s, spans[keep:])
}

// removeSpans deletes the given (ordered, non-overlapping) emoji spans and
// tidies the whitespace around each.
func removeSpans(s string, spans []emojiSpan) string {
	if len(spans) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, sp := range spans {
		b.WriteString(s[last:sp.start])
		last = sp.end
		out := b.String()
		trimmed := strings.TrimRight(out, " \t")
		leftSpace := len(trimmed) < len(out)
		rest := s[last:]
		restTrim := strings.TrimLeft(rest, " \t")
		rightSpace := len(restTrim) < len(rest)
		atLineStart := trimmed == "" || strings.HasSuffix(trimmed, "\n")
		next, _ := utf8.DecodeRuneInString(restTrim)
		closing := restTrim == "" || strings.ContainsRune(".,!?;:)…\n", next) || emojiAt(restTrim) > 0
		switch {
		case atLineStart && strings.HasPrefix(restTrim, "\n") && trimmed != "":
			// The line held only emoji: drop it.
			b.Reset()
			b.WriteString(trimmed)
			last += len(rest) - len(restTrim) + 1
		case atLineStart:
			b.Reset()
			b.WriteString(trimmed)
			last += len(rest) - len(restTrim)
		case leftSpace && (rightSpace || closing):
			b.Reset()
			b.WriteString(trimmed)
			if !closing {
				b.WriteByte(' ')
			}
			last += len(rest) - len(restTrim)
		}
	}
	b.WriteString(s[last:])
	return strings.TrimSpace(b.String())
}

// StripEmoji removes every emoji cluster from s.
func StripEmoji(s string) string { return KeepEmoji(s, 0) }

// ─── tone ────────────────────────────────────────────────────

// Tone is the rough intent of an incoming message, from a cheap keyword
// heuristic (English + Hebrew). It decides whether emoji fit a reply and
// which emoji a reaction uses.
type Tone string

const (
	ToneNeutral  Tone = "neutral"
	ToneSerious  Tone = "serious"  // bad news, grief, illness, distress: no emoji, no laughing
	ToneVent     Tone = "vent"     // annoyed / frustrated: no laughing
	ToneGood     Tone = "good"     // good news, celebration
	ToneLove     Tone = "love"     // thanks, affection
	ToneFunny    Tone = "funny"    // jokes, laughter
	ToneSurprise Tone = "surprise" // gossip, shock
	TonePlan     Tone = "plan"     // plans, invitations, agreement
	ToneQuestion Tone = "question"
	ToneGreeting Tone = "greeting"
)

// Keyword lists are matched against the lower-cased text: entries starting
// with a space must start a word; Hebrew entries match anywhere (prefixes
// like ו/ש/ה attach to words).
var toneWords = []struct {
	tone  Tone
	words []string
}{
	{ToneSerious, []string{
		"passed away", " died", "is dying", " dead.", "funeral", "cancer", "hospital", " icu", "diagnos", "depress", "suicid",
		"kill myself", "self harm", "anxiety", "panic attack", "laid off", "got fired", "been fired", "lost my job",
		"divorce", "miscarriage", "accident", "chemo", "grief", "grieving", " rip ", "condolence",
		"broke up with me", "we broke up", "dumped me", "can't get out of bed", "cant get out of bed", "not okay",
		"not ok", "falling apart", "so sad", "heartbroken", "crying", "lonely", "bad news", "i'm sick", "im sick",
		"feeling sick", "got sick", "been sick", "is sick", "very sick",
		"in pain", "surgery", "emergency", "pray for", "mourning", "was attacked", "assault", "abuse", "rockets", "rocket fire",
		"siren", " war ", "missing since",
		"נפטר", "נפטרה", "הלוויה", "לוויה", "שבעה", "בית חולים", "בית החולים", "סרטן", "פוטרתי", "פיטרו אותי", "דיכאון",
		"חרדה", "התקף חרדה", "תאונה", "גירושים", "התגרשנו", "נפרדנו", "נפרדה ממני", "נפרד ממני", "עצוב לי", "עצובה",
		"הרוס", "הרוסה", "קשה לי", "בוכה", "בכיתי", "ניתוח", "אזעקה", "אזעקות", "טילים", "מלחמה", "חולה", "ז״ל", "ז\"ל",
		"משתתף בצערך", "משתתפת בצערך", "בודד", "בודדה",
	}},
	{ToneVent, []string{
		" vent", "so done", "nightmare", "annoying", "annoyed", "frustrat", "pissed", "i hate", " ugh", "worst",
		"sick of", "fed up", "can't stand", "cant stand", "furious", " rage", "unbelievable", "ridiculous",
		"נמאס", "עצבים", "מעצבן", "מעצבנת", "מבאס", "מבאסת", "סיוט", "שונא", "שונאת", "חרא", "די כבר", "התעצבנתי",
	}},
	{ToneGood, []string{
		"got the job", "new job", "engaged", "promot", " passed", "i won", "we won", "we did it", "accepted",
		"graduat", "pregnant", "it's a girl", "it's a boy", "got in", "finally", "good news", "great news",
		"best day", "nailed it", "birthday", "congrat", "hired", "got the apartment", "got the flat", "bought a",
		"mazal tov", "mazel tov",
		"קיבלתי", "התקבלתי", "מאורס", "התארסנו", "סוף סוף", "סוף-סוף", "עברתי", "ניצחנו", "מזל טוב", "יש!!", "בהריון",
		"סיימתי את התואר", "יום הולדת", "חדשות טובות", "הצלחתי",
	}},
	{ToneLove, []string{
		"thank", "thx", " ty ", " ty!", "appreciate", "love you", "luv you", "you're the best", "youre the best",
		"you are the best", "miss you", "proud of you", "so sweet", "you're amazing",
		"תודה", "אוהב אותך", "אוהבת אותך", "מתגעגע", "מתגעגעת", "אתה הכי", "את הכי", "גאה בך", "מותק",
	}},
	{ToneFunny, []string{
		"lol", "lmao", "lmfao", "rofl", "haha", "hehe", "laughing", "dying", "😂", "🤣", "💀", "joke", "hilarious", "funny", "i'm dead",
		"im dead", "why did the", "what do you call", "knock knock",
		"חחח", "צחוק", "מצחיק", "מת מצחוק", "מתה מצחוק", "בדיחה",
	}},
	{ToneSurprise, []string{
		"no way", "wait what", "wait,", "omg", "oh my god", "wtf", "can't believe", "cant believe", "guess what",
		"seriously?", "really?!", "did you hear", "broke up",
		"אין מצב", "לא נכון", "באמת?", "וואו", "מה??", "שמעת", "לא תאמין", "לא תאמיני",
	}},
	{TonePlan, []string{
		"let's", "lets ", "drinks", "dinner", "lunch", "brunch", "coffee?", "meet", "tonight", "tomorrow at",
		"friday", "saturday", "weekend", "you in", "are you coming", "join us", "wanna", "want to come", "booked",
		"at 8", "at 7", "at 9", "plans", " trip",
		"ניפגש", "נפגש", "נצא", "בא לך", "באה לך", "בסופ\"ש", "בסופ״ש", "סופש", "מחר ב", "הערב", "ארוחה", "בירה", "קפה?",
	}},
	{ToneGreeting, []string{
		"hey", "hi", "hello", "yo", "sup", "good morning", "morning", "gm",
		"היי", "הי", "שלום", "מה קורה", "מה נשמע", "בוקר טוב", "אהלן",
	}},
}

// ClassifyTone guesses the tone of an incoming message. Serious beats
// everything; a greeting only counts for very short messages; a question
// mark alone means a question.
func ClassifyTone(text string) Tone {
	t := " " + strings.ToLower(strings.Join(strings.Fields(text), " ")) + " "
	if strings.TrimSpace(t) == "" {
		return ToneNeutral
	}
	words := len(strings.Fields(t))
	for _, tw := range toneWords {
		if tw.tone == TonePlan && strings.Contains(t, "?") && whQuestion(t) {
			return ToneQuestion // "what time should we meet?" asks, it doesn't propose
		}
		if tw.tone == ToneGreeting && words > 4 {
			continue
		}
		for _, w := range tw.words {
			if tw.tone == ToneSerious && weakSerious[w] && laughing(t) {
				continue // "I almost died laughing", "rip my wallet lol", "הרוס מצחוק"
			}
			if tw.tone == ToneGreeting {
				if strings.Contains(t, " "+w+" ") || strings.Contains(t, " "+w+"!") || strings.Contains(t, " "+w+",") || strings.Contains(t, " "+w+"?") {
					return tw.tone
				}
				continue
			}
			if strings.Contains(t, w) {
				return tw.tone
			}
		}
	}
	if strings.Contains(t, "?") {
		return ToneQuestion
	}
	if strings.Contains(t, "!!") {
		return ToneGood
	}
	return ToneNeutral
}

// weakSerious are serious keywords that are often jokes when the message
// also laughs.
var weakSerious = map[string]bool{" died": true, "is dying": true, " dead.": true, " rip ": true, "crying": true, "accident": true, "הרוס": true, "הרוסה": true}

func laughing(t string) bool {
	for _, m := range []string{"lol", "lmao", "haha", "laughing", "חחח", "מצחוק", "😂", "🤣", "💀"} {
		if strings.Contains(t, m) {
			return true
		}
	}
	return false
}

// whWords start an open question (EN + HE).
var whWords = []string{"what", "when", "where", "which", "who", "how", "why", "מה", "מתי", "איפה", "איך", "למה", "מי", "כמה", "איזה", "איזו"}

func whQuestion(t string) bool {
	f := strings.Fields(t)
	if len(f) == 0 {
		return false
	}
	first := strings.Trim(f[0], ",.!?\"'")
	return slices.Contains(whWords, first)
}

// IncomingText is what the next reply answers: the "them" messages since the
// persona last spoke (at most the last three), joined by newlines.
func IncomingText(hist []model.Message) string {
	var parts []string
	for i := len(hist) - 1; i >= 0 && len(parts) < 3; i-- {
		if hist[i].Speaker == "me" {
			break
		}
		parts = append([]string{hist[i].Text}, parts...)
	}
	return strings.Join(parts, "\n")
}

// ─── emoji rules ─────────────────────────────────────────────

// Emoji levels.
const (
	EmojiNone = "none"
	EmojiRare = "rare"
	EmojiSome = "some"
	EmojiLots = "lots"
)

// EmojiLevel normalises a persona's emoji usage ("" and unknown = rare).
func EmojiLevel(usage string) string {
	switch usage {
	case EmojiNone, EmojiSome, EmojiLots:
		return usage
	}
	return EmojiRare
}

// Roller is the randomness the expression rules need (behavior.Sampler).
type Roller interface {
	Hit(percent int) bool
	Index(n int) int
}

// Emoji enforcement constants. The add rates bring each level to its
// target share of replies with emoji (measured with llama3.1:8b: rare ≈ 1 in
// 5, some ≈ half, lots ≈ most), because small models write far fewer emoji
// than asked once the prompt says when they fit.
const (
	// RareGap: a "rare" persona's emoji is dropped (and none added) if any of
	// its last RareGap messages had one.
	RareGap = 3
	// RareMax / SomeMax are the most emoji a reply keeps per level.
	RareMax = 1
	SomeMax = 2
	// Chance that an emoji-free reply gets one that suits the message, per level.
	RareAddPercent = 15
	SomeAddPercent = 60
	LotsAddPercent = 85
)

func favorites(e model.EmojiPrefs) []string {
	var out []string
	for _, f := range e.Favorites {
		if f = strings.TrimSpace(f); f != "" && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// emojiRule is the persona's emoji line in COMMUNICATION STYLE.
func emojiRule(e model.EmojiPrefs) string {
	favs := ""
	if f := favorites(e); len(f) > 0 {
		favs = " Favourites: " + strings.Join(f, " ") + "."
	}
	const fit = " Emoji fit jokes, good news, warmth and excitement; never use them on sad, serious or upsetting topics."
	switch EmojiLevel(e.Usage) {
	case EmojiNone:
		return "Never use emoji. Not a single one, in any message."
	case EmojiSome:
		return "Now and then: at most two emoji in a message, only where they fit, and plenty of messages have none." + fit + favs
	case EmojiLots:
		return "Often: most messages get an emoji or two that match the mood." + fit + favs
	default:
		return "Rarely: about one message in five gets a single emoji, the rest have none. Only when it really adds something." + fit + favs
	}
}

// emojiTurn is the per-turn emoji hint for GUIDANCE ("" = nothing to add).
func emojiTurn(e model.EmojiPrefs, tone Tone) string {
	if EmojiLevel(e.Usage) == EmojiNone {
		return " No emoji."
	}
	switch tone {
	case ToneSerious:
		return " They're sharing something serious: be kind and genuine, no emoji and no jokes."
	case ToneVent:
		return " They're upset: no laughing emoji, don't make fun of it."
	}
	return ""
}

// EmojiResult reports what EnforceEmoji changed.
type EmojiResult struct {
	Text    string
	Removed int
	Added   string
}

// EnforceEmoji makes a generated reply follow the persona's emoji level:
// none → no emoji; rare → at most RareMax, none when one of the persona's
// last RareGap messages had one; some → at most SomeMax; lots → any. An
// emoji-free reply gets one that suits the message (AddEmoji) with the
// level's add rate. Replies to serious messages never keep or get emoji;
// replies to venting lose laughing emoji and get none added. recentMine are
// the persona's previous messages (oldest first).
func EnforceEmoji(text string, e model.EmojiPrefs, tone Tone, recentMine []string, s Roller) EmojiResult {
	n := CountEmoji(text)
	keep := n
	level := EmojiLevel(e.Usage)
	gap := level == EmojiRare && usedEmojiRecently(recentMine, RareGap)
	switch {
	case level == EmojiNone || tone == ToneSerious || gap:
		keep = 0
	case level == EmojiRare:
		keep = min(n, RareMax)
	case level == EmojiSome:
		keep = min(n, SomeMax)
	}
	out := KeepEmoji(text, keep)
	if tone == ToneVent {
		out = dropLaughing(out)
	}
	res := EmojiResult{Text: out, Removed: n - CountEmoji(out)}
	add := map[string]int{EmojiRare: RareAddPercent, EmojiSome: SomeAddPercent, EmojiLots: LotsAddPercent}[level]
	if CountEmoji(out) == 0 && n == 0 && !gap && tone != ToneSerious && tone != ToneVent && strings.TrimSpace(out) != "" && s.Hit(add) {
		if a := AddEmoji(favorites(e), tone, s); a != "" {
			res.Text, res.Added = strings.TrimRight(out, " ")+" "+a, a
		}
	}
	return res
}

// AddEmoji picks an emoji to append to a reply answering a message of tone:
// a favourite that suits it; else, for everyday messages, a favourite with
// no conflicting meaning (💅, 🛋️, ✨ …); else a default for good news,
// jokes, thanks and surprises; else "" (questions, serious, venting).
func AddEmoji(favs []string, tone Tone, s Roller) string {
	if f := FittingEmoji(favs, tone, s); f != "" {
		return f
	}
	switch tone {
	case ToneNeutral, ToneQuestion, ToneGreeting, TonePlan:
		var calm []string
		for _, f := range favs {
			if emojiTones(f)&maskOf(ToneFunny, ToneSerious, ToneVent, ToneSurprise) == 0 {
				calm = append(calm, f)
			}
		}
		if len(calm) > 0 {
			return calm[s.Index(len(calm))]
		}
	case ToneGood, ToneFunny, ToneLove, ToneSurprise:
		d := toneDefaults[tone]
		return d[s.Index(len(d))]
	}
	return ""
}

func usedEmojiRecently(mine []string, n int) bool {
	for i := len(mine) - 1; i >= 0 && i >= len(mine)-n; i-- {
		if CountEmoji(mine[i]) > 0 {
			return true
		}
	}
	return false
}

func dropLaughing(s string) string {
	var drop []emojiSpan
	for _, sp := range emojiSpans(s) {
		if m := emojiTones(s[sp.start:sp.end]); m&maskOf(ToneFunny) != 0 && m&maskOf(ToneSerious, ToneVent) == 0 {
			drop = append(drop, sp)
		}
	}
	return removeSpans(s, drop)
}

// ─── emoji meaning ───────────────────────────────────────────

func maskOf(ts ...Tone) uint16 {
	var m uint16
	for _, t := range ts {
		switch t {
		case ToneSerious:
			m |= 1 << 0
		case ToneVent:
			m |= 1 << 1
		case ToneGood:
			m |= 1 << 2
		case ToneLove:
			m |= 1 << 3
		case ToneFunny:
			m |= 1 << 4
		case ToneSurprise:
			m |= 1 << 5
		case TonePlan:
			m |= 1 << 6
		case ToneQuestion:
			m |= 1 << 7
		case ToneGreeting:
			m |= 1 << 8
		case ToneNeutral:
			m |= 1 << 9
		}
	}
	return m
}

// emojiMeaning maps common emoji to the tones they suit (variation selector
// U+FE0F ignored when looking up). Niche emoji (💅 💸 📈 🧠 🔮 …) suit no
// tone on purpose: they stay in the persona's messages but a reaction uses
// a clearer one.
var emojiMeaning = map[string]uint16{
	"😂": maskOf(ToneFunny), "🤣": maskOf(ToneFunny), "😆": maskOf(ToneFunny), "😹": maskOf(ToneFunny),
	"💀": maskOf(ToneFunny), "😅": maskOf(ToneFunny), "😄": maskOf(ToneFunny, ToneGood), "😁": maskOf(ToneFunny, ToneGood),
	"🙈": maskOf(ToneFunny), "😜": maskOf(ToneFunny), "🤪": maskOf(ToneFunny), "😏": maskOf(ToneFunny),
	"😭": maskOf(ToneFunny, ToneSurprise),
	"🎉": maskOf(ToneGood, TonePlan), "🥳": maskOf(ToneGood), "🍾": maskOf(ToneGood), "🥂": maskOf(ToneGood, TonePlan),
	"🔥": maskOf(ToneGood, TonePlan), "👏": maskOf(ToneGood), "🙌": maskOf(ToneGood, TonePlan), "🤩": maskOf(ToneGood, ToneSurprise),
	"💯": maskOf(ToneGood, TonePlan), "🏆": maskOf(ToneGood), "👑": maskOf(ToneGood), "⭐": maskOf(ToneGood),
	"✨": maskOf(ToneGood, ToneLove), "🚀": maskOf(ToneGood),
	"💪": maskOf(ToneGood, TonePlan), "🍸": maskOf(ToneGood, TonePlan), "🍻": maskOf(ToneGood, TonePlan),
	"🍺": maskOf(TonePlan), "🍷": maskOf(TonePlan, ToneGood),
	"❤": maskOf(ToneLove, ToneGood, ToneSerious, ToneVent, ToneGreeting, ToneNeutral), "🤍": maskOf(ToneLove, ToneSerious),
	"💕": maskOf(ToneLove), "💖": maskOf(ToneLove, ToneGood), "🥰": maskOf(ToneLove, ToneGood), "😍": maskOf(ToneLove, ToneGood),
	"😘": maskOf(ToneLove, ToneGreeting), "🤗": maskOf(ToneLove, ToneGreeting, ToneSerious), "🫶": maskOf(ToneLove, ToneSerious),
	"🥹": maskOf(ToneLove, ToneGood), "😊": maskOf(ToneLove, ToneGreeting, ToneNeutral), "🙂": maskOf(ToneNeutral, ToneGreeting),
	"🙏": maskOf(ToneLove, ToneSerious), "🫂": maskOf(ToneSerious, ToneVent), "😢": maskOf(ToneSerious), "😔": maskOf(ToneSerious, ToneVent),
	"💔": maskOf(ToneSerious), "🥺": maskOf(ToneSerious, ToneLove), "😥": maskOf(ToneSerious),
	"😮": maskOf(ToneSurprise), "😱": maskOf(ToneSurprise), "👀": maskOf(ToneSurprise, ToneQuestion), "🤯": maskOf(ToneSurprise),
	"😳": maskOf(ToneSurprise), "😲": maskOf(ToneSurprise), "🫢": maskOf(ToneSurprise), "🍵": maskOf(ToneSurprise),
	"😤": maskOf(ToneVent), "😩": maskOf(ToneVent), "🙄": maskOf(ToneVent), "😡": maskOf(ToneVent), "🤦": maskOf(ToneVent),
	"😬": maskOf(ToneVent, ToneSurprise), "🥲": maskOf(ToneVent),
	"👍": maskOf(TonePlan, ToneNeutral, ToneQuestion), "👌": maskOf(TonePlan, ToneNeutral), "🤝": maskOf(TonePlan),
	"✅": maskOf(TonePlan), "😎": maskOf(TonePlan, ToneGood), "🤙": maskOf(TonePlan, ToneGreeting),
	"👋": maskOf(ToneGreeting), "🤔": maskOf(ToneQuestion), "🧐": maskOf(ToneQuestion),
}

// emojiTones is the tone mask of one emoji cluster (skin tones and
// variation selectors ignored; unknown emoji = 0).
func emojiTones(e string) uint16 {
	e = strings.Map(func(r rune) rune {
		if r == vs16 || r == vs15 || isSkinTone(r) {
			return -1
		}
		return r
	}, e)
	return emojiMeaning[e]
}

// toneDefaults are the fallback emoji per tone (first = WhatsApp's classic reactions).
var toneDefaults = map[Tone][]string{
	ToneFunny:    {"😂", "🤣", "💀"},
	ToneGood:     {"❤️", "🎉", "🔥", "🙌"},
	ToneLove:     {"❤️", "🥰", "🙏"},
	ToneSurprise: {"😮", "😱", "👀"},
	TonePlan:     {"👍", "🙌", "🔥"},
	ToneSerious:  {"❤️", "🙏", "😢"},
	ToneVent:     {"😢", "❤️", "😤"},
	ToneQuestion: {"👍", "🤔"},
	ToneGreeting: {"❤️", "👋"},
	ToneNeutral:  {"👍", "❤️"},
}

// FittingEmoji picks an emoji for tone: one of favs that suits it, else ""
// (callers that need one use ReactionFor).
func FittingEmoji(favs []string, tone Tone, s Roller) string {
	var fit []string
	for _, f := range favs {
		if emojiTones(f)&maskOf(tone) != 0 {
			fit = append(fit, f)
		}
	}
	if len(fit) == 0 {
		return ""
	}
	return fit[s.Index(len(fit))]
}

// ReactionFor chooses the emoji a persona reacts to an incoming message
// with: a favourite that suits the message's tone, else a small default set
// for that tone (only WhatsApp's classic reactions for personas that never
// use emoji). Laughing emoji never go on serious or venting messages.
func ReactionFor(e model.EmojiPrefs, text string, s Roller) (string, Tone) {
	tone := ClassifyTone(text)
	if EmojiLevel(e.Usage) != EmojiNone {
		if f := FittingEmoji(favorites(e), tone, s); f != "" {
			return f, tone
		}
	}
	defs := toneDefaults[tone]
	if len(defs) == 0 {
		defs = toneDefaults[ToneNeutral]
	}
	if EmojiLevel(e.Usage) == EmojiNone {
		return defs[0], tone
	}
	return defs[s.Index(len(defs))], tone
}

// ─── length ──────────────────────────────────────────────────

// Length biases (behaviour profile lengthBias).
const (
	BiasShorter = "shorter"
	BiasNormal  = "normal"
	BiasLonger  = "longer"
	BiasMatch   = "match" // scale to the length of their message
)

// lengthWords is the word budget of each messageLength at bias "normal".
var lengthWords = map[string]int{"short": 12, "medium": 30, "long": 60}

// Bias scales and the "long incoming message" allowance.
const (
	shorterScale   = 0.6
	longerScale    = 1.6
	longIncoming   = 25   // words: a message this long earns a slightly longer answer
	longInScale    = 1.25 // … this much longer (biases shorter/normal/longer)
	matchMinWords  = 4
	matchExtraWord = 3
)

func messageLengthOf(l string) string {
	if _, ok := lengthWords[l]; ok {
		return l
	}
	return "short"
}

// WordBudget is the most words a reply should have for the persona's
// messageLength, the chat's lengthBias and the message it answers:
// short 12 / medium 30 / long 60 words, ×0.6 shorter, ×1.6 longer, ×1.25
// when they wrote more than 25 words. "match" mirrors their length (their
// words + 3, at least 4) up to the "longer" budget.
func WordBudget(messageLength, bias, incoming string) int {
	base := float64(lengthWords[messageLengthOf(messageLength)])
	in := len(strings.Fields(StripEmoji(incoming)))
	if bias == BiasMatch {
		return min(max(in+matchExtraWord, matchMinWords), int(base*longerScale+0.5))
	}
	switch bias {
	case BiasShorter:
		base *= shorterScale
	case BiasLonger:
		base *= longerScale
	}
	if in > longIncoming {
		base *= longInScale
	}
	return max(int(base+0.5), matchMinWords)
}

// HardLimit is the word count above which a reply counts as far over budget
// (it is then re-asked or trimmed).
func HardLimit(budget int) int { return budget + max(3, budget/3) }

// lengthRule is the persona's length line in COMMUNICATION STYLE.
func lengthRule(l string) string {
	switch messageLengthOf(l) {
	case "medium":
		return "Medium: one to three sentences."
	case "long":
		return "Long: a few sentences, up to a short paragraph when there's something to say."
	default:
		return "Short: a few words to one sentence, like most real texts. No paragraphs."
	}
}

// Guidance is the per-turn length hint: a word budget from the persona's
// messageLength, the chat's lengthBias and the message being answered
// (see WordBudget). It wins over lengths mentioned in the persona text.
func Guidance(messageLength, lastMsg, bias string) string {
	n := WordBudget(messageLength, bias, lastMsg)
	var s string
	switch {
	case n <= 6:
		s = "Ultra brief: a few words, at most one short sentence"
	case n <= 14:
		s = "Keep it brief: one sentence, two short ones at most"
	case n <= 25:
		s = "Short: one or two sentences"
	case n <= 45:
		s = "Two or three sentences"
	case n <= 70:
		s = "Up to a short paragraph (three to five sentences)"
	default:
		s = "A full paragraph is fine when there's something to say"
	}
	return fmt.Sprintf("%s — at most %d words. This length rule wins over any length mentioned above.", s, n)
}

// LengthRetryNote is appended to the system prompt when a draft came back
// far over budget and is asked for again.
func LengthRetryNote(budget, got int) string {
	return fmt.Sprintf("\n\nIMPORTANT: your last draft had %d words — far too long. Reply in at most %d words: keep only the main point.", got, budget)
}

// WordCount counts the words of a reply (emoji and lone punctuation are not words).
func WordCount(s string) int {
	n := 0
	for _, f := range strings.Fields(StripEmoji(s)) {
		if strings.IndexFunc(f, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0 {
			n++
		}
	}
	return n
}

// TrimToWords shortens text to about limit words: it keeps whole sentences
// (at least the first). A first sentence of up to 1.5×limit words is kept
// whole; a longer one is cut after a comma, else between words (never after
// a function word) with "…". Words — hence @tags and
// URLs — are never split, and a multi-word "@Name Surname" tag is never cut
// after its first word. ok is false when text already fits.
func TrimToWords(text string, limit int) (out string, ok bool) {
	text = strings.TrimSpace(text)
	if WordCount(text) <= limit {
		return text, false
	}
	sents := sentenceSpans(text)
	end, words := 0, 0
	for _, sp := range sents {
		w := WordCount(text[sp[0]:sp[1]])
		if end > 0 && words+w > limit {
			break
		}
		end, words = sp[1], words+w
		if words > limit {
			break
		}
	}
	if words <= limit && end > 0 {
		return strings.TrimSpace(text[:end]), true
	}
	// The first sentence alone is too long: keep it whole when it is only a
	// little over (run-on sentences without punctuation are common), else
	// cut inside it.
	first := text[:sents[0][1]]
	if WordCount(first) <= limit*3/2 {
		return strings.TrimSpace(first), len(strings.TrimSpace(first)) < len(text)
	}
	fields := fieldSpans(first)
	cut, n := 0, 0
	for i, f := range fields {
		w := first[f[0]:f[1]]
		if WordCount(w) > 0 {
			n++
		}
		if n > limit {
			break
		}
		if i+1 < len(fields) && strings.HasPrefix(w, "@") {
			continue // never end right after "@First" of a longer name
		}
		if danglingWords[strings.ToLower(strings.Trim(w, ",;:"))] {
			continue // never end on "the", "a", "to", "you", "של" …
		}
		cut = f[1]
		if strings.HasSuffix(w, ",") || strings.HasSuffix(w, ";") || strings.HasSuffix(w, ":") {
			if n >= limit/2 {
				return strings.TrimRight(first[:cut], ",;:"), true
			}
		}
	}
	if cut == 0 {
		return text, false
	}
	return strings.TrimRight(first[:cut], ",;:-–— ") + "…", true
}

// danglingWords are function words a cut reply must not end on (EN + HE).
var danglingWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields("a an the to of for in on at by with from and or but so if as that than then " +
		"is are was were be been am i you he she it we they my your his her our their its me him us them " +
		"what which who when where why how this these those do does did not no just like literally really very " +
		"של את עם על אל מה זה זאת כי אבל או גם אני אתה את הוא היא אנחנו הם שלי שלך ו ב ל ה ש") {
		danglingWords[w] = true
	}
}

// sentenceSpans splits text after . ! ? … (and closing quotes/brackets or
// trailing emoji) followed by whitespace, and at line breaks.
func sentenceSpans(text string) [][2]int {
	var out [][2]int
	start := 0
	i := 0
	for i < len(text) {
		r, n := utf8.DecodeRuneInString(text[i:])
		if r == '\n' {
			if strings.TrimSpace(text[start:i]) != "" {
				out = append(out, [2]int{start, i})
			}
			start = i + n
			i += n
			continue
		}
		if r == '.' || r == '!' || r == '?' || r == '…' {
			j := i + n
			for j < len(text) {
				r2, n2 := utf8.DecodeRuneInString(text[j:])
				if r2 == '.' || r2 == '!' || r2 == '?' || r2 == '…' || strings.ContainsRune(`"')]”’»`, r2) {
					j += n2
					continue
				}
				if r2 == ' ' {
					if k := emojiAt(text[j+1:]); k > 0 {
						j += 1 + k
						continue
					}
				}
				if k := emojiAt(text[j:]); k > 0 {
					j += k
					continue
				}
				break
			}
			if j >= len(text) || text[j] == ' ' || text[j] == '\n' || text[j] == '\t' {
				out = append(out, [2]int{start, j})
				start = j
			}
			i = j
			continue
		}
		i += n
	}
	if strings.TrimSpace(text[start:]) != "" {
		out = append(out, [2]int{start, len(text)})
	}
	return out
}

// fieldSpans returns the byte ranges of the whitespace-separated fields of s.
func fieldSpans(s string) [][2]int {
	var out [][2]int
	start := -1
	for i, r := range s {
		if unicode.IsSpace(r) {
			if start >= 0 {
				out = append(out, [2]int{start, i})
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, [2]int{start, len(s)})
	}
	return out
}

// WordsFor is a tiny reply in words for a tone, used when a reply was
// nothing but emoji and the persona's level removed them all.
func WordsFor(t Tone) string {
	switch t {
	case ToneFunny:
		return "haha"
	case ToneGood:
		return "amazing!"
	case ToneLove:
		return "aw"
	case ToneSerious:
		return "I'm so sorry"
	case ToneVent:
		return "ugh"
	case ToneSurprise:
		return "no way"
	case TonePlan:
		return "sounds good"
	case ToneGreeting:
		return "hey"
	case ToneQuestion:
		return "hmm"
	}
	return "ok"
}
