// Package crossctx decides what a persona may carry from one of its chats
// into another ("cross-chat context"): which mode a chat uses, which
// memories may cross (sensitive topics never do on their own), which items
// to show for the people in the conversation, and whether a generated reply
// gives away something it should keep to itself (Leak). It is a leaf package
// (model, memory, handoff + stdlib).
package crossctx

import (
	"math"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/handoff"
	"whatsappdoppel/internal/memory"
	"whatsappdoppel/internal/model"
)

// Rules are the app-wide cross-chat settings (config.CrossSettings.Rules).
type Rules struct {
	Enabled   bool
	GroupMode string          // how groups use private chats
	DMMode    string          // how private chats use shared groups
	Sensitive map[string]bool // categories that never cross on their own
	FreshDays int             // older unpinned items and briefs are not carried
	MaxPeople int             // people per reply whose context is used
	MaxItems  int             // items per reply
}

// Defaults: on, discreet in groups, open in private chats, every sensitive
// category kept local, 14 days, 4 people, 8 items.
func Defaults() Rules {
	s := map[string]bool{}
	for _, c := range model.SensitiveCategories {
		s[c] = true
	}
	return Rules{Enabled: true, GroupMode: model.CrossDiscreet, DMMode: model.CrossOpen, Sensitive: s, FreshDays: 14, MaxPeople: 4, MaxItems: 8}
}

// Sources of a mode or share value.
const (
	SourceChat    = "chat"
	SourceDefault = "default"
)

// ValidMode reports whether m is a cross-chat mode.
func ValidMode(m string) bool { return slices.Contains(model.CrossModes, m) }

// ModeFor is how chat c uses the persona's other chats, and where that came
// from (the chat or the app default for its kind). Off when the feature is off.
func ModeFor(r Rules, c model.ChatAssignment) (mode, source string) {
	if !r.Enabled {
		return model.CrossOff, SourceDefault
	}
	if ValidMode(c.Cross.Mode) {
		return c.Cross.Mode, SourceChat
	}
	if c.Kind == "group" {
		return orMode(r.GroupMode, model.CrossDiscreet), SourceDefault
	}
	return orMode(r.DMMode, model.CrossOpen), SourceDefault
}

func orMode(m, d string) string {
	if ValidMode(m) {
		return m
	}
	return d
}

// ShareFor reports whether other chats may use what chat c learned (and the
// source). Always false when the feature is off.
func ShareFor(r Rules, c model.ChatAssignment) (bool, string) {
	if c.Cross.Share != nil {
		return r.Enabled && *c.Cross.Share, SourceChat
	}
	return r.Enabled, SourceDefault
}

// UserOf is the user part of a JID ("972501234567@s.whatsapp.net" →
// "972501234567"), the same person's id across addressing forms.
func UserOf(jid string) string {
	u, _, _ := strings.Cut(jid, "@")
	u, _, _ = strings.Cut(u, ":")
	return strings.TrimSpace(u)
}

// ─── sensitivity ─────────────────────────────────────────────

var (
	romanceRe = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}_])(boyfriend|girlfriend|my ex|her ex|his ex|their ex|an ex|ex-boyfriend|ex-girlfriend|ex husband|ex wife|dating|first date|a date with|crush on|has a crush|broke up|breakup|break up|split up|divorce[ds]?|divorcing|cheat(?:ed|ing) on|affair|pregnan(?:t|cy)|tinder|bumble|hinge|engaged)(?:$|[^\p{L}\p{N}_])`)
	romanceHE = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(?:[והבלמשכ]{1,2})?(חבר שלי|חברה שלי|האקס|האקסית|דייט|נפרדנו|נפרד(?:ה|ו)?|גירושין|מתגרש(?:ת|ים)?|בהריון|הריון|בגד(?:ה)? ב)(?:$|[^\p{L}\p{N}_])`)
	secretRe  = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}_])(secret|secretly|don'?t tell|do not tell|between us|between you and me|keep it quiet|keep this quiet|keep it to yourself|not tell anyone|nobody knows|no one knows|confidential|off the record)(?:$|[^\p{L}\p{N}_])`)
	// Extra words for cross-chat sensitivity only (the hand-off lexicon is
	// tuned for "pause the chat now", this one for "keep it to this chat").
	moneyRe    = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}_])(rent|mortgage|salary|paycheck|pay ?check|debts?|in debt|loans?|overdraft|overdrawn|bills|afford|can'?t pay|cannot pay|behind on|bankrupt(?:cy)?|evicted|eviction|unemployed|laid off|lost (?:his|her|their|my) job|got fired|savings|budget|credit score|taxes|inheritance|owes?)(?:$|[^\p{L}\p{N}_])`)
	moneyHE    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(?:[והבלמשכ]{1,2})?(שכירות|משכנתא|משכורת|מינוס|חובות|הלוואה|פוטר(?:ה)?|מובטל(?:ת)?|לא מצליח(?:ה)? לשלם)(?:$|[^\p{L}\p{N}_])`)
	healthRe   = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}_])(therapy|therapist|psychologist|psychiatrist|diagnos(?:is|ed)|medication|antidepressants?|illness|disease|surgery|operation|treatment|chronic|ivf|fertility|miscarriage|rehab|addiction|sober|eating disorder|adhd|allerg(?:y|ic) (?:attack|reaction)|mental health|biopsy|scan results|test results)(?:$|[^\p{L}\p{N}_])`)
	healthHE   = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(?:[והבלמשכ]{1,2})?(טיפול|פסיכולוג(?:ית)?|פסיכיאטר|אבחנה|אובחנ(?:ה|תי)|תרופות|מחלה|בדיקות)(?:$|[^\p{L}\p{N}_])`)
	distressRe = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}_])(depress(?:ed|ion)|anxiety|panic|burn(?:ed|t)? ?out|struggling|grieving|grief|lonely|self.?harm|crisis|breakdown)(?:$|[^\p{L}\p{N}_])`)
	secretHE   = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(?:[והבלמשכ]{1,2})?(סוד|בסוד|אל תספר(?:י)?|בינינו|שלא תגיד(?:י)?|אף אחד לא יודע)(?:$|[^\p{L}\p{N}_])`)
)

// Classify returns the sensitive category a text touches, or "". It is strict
// on purpose (weak hand-off hits count too): a false "sensitive" only keeps a
// memory in its own chat.
func Classify(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	h, hit := handoff.Detect(text, handoff.All())
	if hit && h.Strength == handoff.Strong {
		return h.Category
	}
	s := handoff.Normalize(text)
	switch {
	case romanceRe.MatchString(s) || romanceHE.MatchString(s):
		return model.SensitiveRomance
	case secretRe.MatchString(s) || secretHE.MatchString(s):
		return model.SensitiveSecret
	case distressRe.MatchString(s):
		return model.HandoffDistress
	case healthRe.MatchString(s) || healthHE.MatchString(s):
		return model.HandoffHealth
	case moneyRe.MatchString(s) || moneyHE.MatchString(s):
		return model.HandoffMoney
	case hit:
		return h.Category // a weak hit: strict on purpose
	}
	return ""
}

// ValidSensitive reports whether c is "" or a sensitive category.
func ValidSensitive(c string) bool { return c == "" || slices.Contains(model.SensitiveCategories, c) }

// EffectiveSensitive is a memory's stored category, else what Classify finds
// in its text and evidence (so memories from before the feature are covered).
func EffectiveSensitive(m model.Memory) string {
	if m.Sensitive != "" {
		return m.Sensitive
	}
	return Classify(m.Text + " . " + m.Evidence)
}

// Why a memory does not cross (Carries).
const (
	WhyLocked    = "locked"
	WhySensitive = "sensitive"
	WhyStale     = "stale"
	WhyExpired   = "expired"
)

// Carries reports whether memory m may be used in another chat: a lock keeps
// it home, your unlock lets it go (even when sensitive), a sensitive topic
// switched on in r stays home, and old unpinned memories and past events are
// not carried.
func Carries(m model.Memory, r Rules, now time.Time) (bool, string) {
	switch m.Scope {
	case model.MemoryScopeLocal:
		return false, WhyLocked
	case model.MemoryScopeShared:
		return true, ""
	}
	if cat := EffectiveSensitive(m); cat != "" && sensitiveOn(r, cat) {
		return false, WhySensitive
	}
	if memory.Expired(m, now) || (m.ExpiresAt != nil && now.After(m.ExpiresAt.Add(48*time.Hour))) {
		return false, WhyExpired
	}
	if !m.Pinned && r.FreshDays > 0 && now.Sub(m.UpdatedAt) > time.Duration(r.FreshDays)*24*time.Hour {
		return false, WhyStale
	}
	return true, ""
}

// sensitiveOn: a nil map (zero Rules) keeps every category home.
func sensitiveOn(r Rules, cat string) bool {
	if r.Sensitive == nil {
		return true
	}
	on, known := r.Sensitive[cat]
	return on || !known
}

// SensitiveOn reports whether category cat never crosses under r.
func SensitiveOn(r Rules, cat string) bool { return sensitiveOn(r, cat) }

// ─── selection ───────────────────────────────────────────────

// Item kinds.
const (
	KindMemory     = "memory"
	KindCommitment = "commitment" // something the persona promised there
	KindTopic      = "topic"      // "lately talking about: …"
	KindTone       = "tone"
	KindGroupNote  = "group_note" // what the person said or did in a group
)

// Item is one thing that may be carried into the current chat.
type Item struct {
	Kind       string
	Person     string // display name in the source chat
	PersonJID  string
	Text       string
	Evidence   string // the words it was learned from (memories), for Leak
	Source     string // source chat key
	SourceName string
	SourceKind string // dm|group
	At         time.Time
	Pinned     bool
}

// Person is someone active in the current conversation.
type Person struct {
	Name string
	JID  string
	IDs  []string // every id of this person (phone and lid forms)
	Rank int      // 0 = the person answered now; lower = more relevant
}

const recencyHalfLife = 7 * 24 * time.Hour

// Select picks the items for a reply: only items about the first MaxPeople
// people (by rank), scored by kind (promises first), pin, overlap with what
// is being talked about (recent) and recency; at most 3 per person (more
// when only one or two people count; always keeping their best commitment)
// and MaxItems in all, ordered by person rank then score.
func Select(items []Item, people []Person, recent []string, now time.Time, r Rules) []Item {
	if len(items) == 0 || len(people) == 0 {
		return nil
	}
	ps := slices.Clone(people)
	slices.SortStableFunc(ps, func(a, b Person) int { return a.Rank - b.Rank })
	if r.MaxPeople > 0 && len(ps) > r.MaxPeople {
		ps = ps[:r.MaxPeople]
	}
	maxItems := r.MaxItems
	if maxItems <= 0 {
		maxItems = 8
	}
	// At most 3 things per person, more when only one or two people count
	// (a private chat).
	perPerson := max(3, maxItems/len(ps))
	topic := map[string]bool{}
	for _, t := range recent {
		for _, w := range memory.Tokens(t) {
			topic[w] = true
		}
	}
	type scored struct {
		it    Item
		score float64
	}
	var out []Item
	for _, p := range ps {
		var mine []scored
		for _, it := range items {
			if !Matches(it, p) {
				continue
			}
			s := 0.0
			switch it.Kind {
			case KindCommitment:
				s += 3
			case KindTone:
				s += 0.2
			}
			if it.Pinned {
				s += 2
			}
			hits := 0
			for _, w := range memory.Tokens(it.Text) {
				if topic[w] {
					hits++
				}
			}
			s += 1.2 * float64(min(hits, 2))
			if !it.At.IsZero() {
				age := max(now.Sub(it.At), 0)
				s += math.Pow(0.5, float64(age)/float64(recencyHalfLife))
			}
			mine = append(mine, scored{it, s})
		}
		slices.SortStableFunc(mine, func(a, b scored) int {
			switch {
			case a.score > b.score:
				return -1
			case a.score < b.score:
				return 1
			}
			return 0
		})
		if len(mine) > perPerson {
			// Keep the best commitment even when others score higher.
			top := mine[:perPerson]
			if !slices.ContainsFunc(top, func(s scored) bool { return s.it.Kind == KindCommitment }) {
				if i := slices.IndexFunc(mine, func(s scored) bool { return s.it.Kind == KindCommitment }); i >= 0 {
					top = append(slices.Clone(mine[:perPerson-1]), mine[i])
				}
			}
			mine = top
		}
		for _, s := range mine {
			if len(out) >= maxItems {
				return out
			}
			out = append(out, s.it)
		}
	}
	return out
}

// Matches reports whether an item is about person p: by id when both have
// one, else by name (whole name or first word, case-insensitive).
func Matches(it Item, p Person) bool {
	if u := UserOf(it.PersonJID); u != "" {
		for _, id := range append([]string{p.JID}, p.IDs...) {
			if UserOf(id) == u {
				return true
			}
		}
		if p.JID != "" || len(p.IDs) > 0 {
			return false
		}
	}
	a, b := strings.ToLower(strings.TrimSpace(it.Person)), strings.ToLower(strings.TrimSpace(p.Name))
	if a == "" || b == "" {
		return false
	}
	return a == b || firstWord(a) == firstWord(b)
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return s
}

// ─── leak guard ──────────────────────────────────────────────

// genericWords never count as a distinctive overlap with a reply.
var genericWords = map[string]bool{
	"really": true, "thing": true, "things": true, "going": true, "today": true, "tomorrow": true, "week": true,
	"weekend": true, "time": true, "good": true, "great": true, "nice": true, "little": true, "people": true,
	"lately": true, "talking": true, "about": true, "always": true, "never": true, "likes": true, "loves": true,
	"just": true, "very": true, "much": true, "some": true, "this": true, "that": true, "what": true, "when": true,
	"there": true, "their": true, "them": true, "they": true, "with": true, "your": true, "you're": true, "would": true,
	"could": true, "should": true, "doing": true, "said": true, "says": true, "told": true, "want": true, "wants": true,
	"been": true, "being": true, "more": true, "than": true, "into": true, "back": true, "make": true, "made": true,
	"still": true, "well": true, "also": true, "like": true, "know": true, "think": true, "feel": true, "feels": true,
	"promised": true, "agreed": true, "bring": true, "friday": true, "saturday": true, "sunday": true, "monday": true,
	"tuesday": true, "wednesday": true, "thursday": true, "tonight": true, "next": true, "last": true, "recent": true,
	"topics": true, "hope": true, "work": true, "home": true, "here": true, "over": true, "after": true, "before": true,
}

// Distinct are the words of an item that would give it away: its tokens
// (text and evidence) minus anything said here, names, short and generic words.
func Distinct(it Item, here map[string]bool, names map[string]bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range memory.Tokens(it.Text + " " + it.Evidence) {
		if seen[w] || here[w] || names[w] || genericWords[w] || utf8.RuneCountInString(w) < 4 || isNumber(w) {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}

func isNumber(w string) bool {
	for _, r := range w {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

var (
	// told: "as you told me", "you mentioned", "when we talked" — pointing at
	// an earlier private conversation.
	toldRe = regexp.MustCompile(`(?i)\b(?:(?:like|as) you (?:told|said|mentioned|wrote|texted)(?: to)?(?: me)?|you (?:told|texted) me|you (?:said|mentioned) to me|you mentioned|when we (?:talked|spoke|chatted|texted)|you were telling me|you said the other day|you told me|you messaged me)\b`)
	// privateRe: "our private chat", "in the DMs", "privately" — saying a private chat exists.
	privateRe = regexp.MustCompile(`(?i)\b(?:(?:our|your|my|a|the) (?:private|direct|personal) (?:chat|messages?|conversation|convo|texts?)|in (?:the |our |my |your )?dms?|in private|privately|between (?:us|you and me)|our (?:little )?(?:chat|convo|conversation)|dm(?:'d|ed) me|texted me (?:about|that))\b`)
	toldHE    = regexp.MustCompile(`(אמרת לי|סיפרת לי|כתבת לי|בפרטי|בינינו|בשיחה שלנו|כשדיברנו)`)
)

// LeakInput is what Leak checks a reply against.
type LeakInput struct {
	Items   []Item          // the items shown in the prompt
	Reply   string          // the generated reply
	Here    []string        // texts of this chat (history, its own memories)
	Mode    string          // model.CrossDiscreet | CrossOpen
	IsGroup bool            // target is a group (private → group direction)
	Present map[string]bool // users (UserOf) and lower-case first names active in the last messages
}

// Leak lists what in a reply gives away something from another chat that it
// should not ("" items = clean). Discreet: any distinctive overlap with an
// item, or any phrase pointing at a private conversation. Open: only things
// about people who aren't part of the conversation right now, plus saying in
// a group that a private chat exists. Matches are for tests and logs only.
func Leak(in LeakInput) []string {
	if len(in.Items) == 0 || strings.TrimSpace(in.Reply) == "" || in.Mode == model.CrossOff {
		return nil
	}
	here := map[string]bool{}
	for _, t := range in.Here {
		for _, w := range memory.Tokens(t) {
			here[w] = true
		}
	}
	names := map[string]bool{}
	for _, it := range in.Items {
		for _, w := range memory.Tokens(it.Person + " " + it.SourceName) {
			names[w] = true
		}
	}
	replyToks := map[string]bool{}
	for _, w := range memory.Tokens(in.Reply) {
		replyToks[w] = true
	}
	replyLow := strings.ToLower(in.Reply)
	norm := normWords(in.Reply)
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	anyAbsent := false
	for _, it := range in.Items {
		present := in.Mode == model.CrossOpen && isPresent(it, in.Present)
		if !present {
			anyAbsent = true
		}
		if present {
			continue // Open: their own things may be mentioned with them
		}
		distinct := Distinct(it, here, names)
		var hits []string
		for _, w := range distinct {
			if replyToks[w] || stemHit(w, replyToks) {
				hits = append(hits, w)
			}
		}
		if len(hits) >= 2 || (len(hits) == 1 && len(distinct) <= 2) {
			for _, h := range hits {
				add(h)
			}
			continue
		}
		for _, g := range trigrams(it.Text, here) {
			if strings.Contains(norm, g) {
				add(g)
			}
		}
	}
	// Pointing at a private conversation.
	if in.Mode == model.CrossDiscreet || anyAbsent {
		if m := toldRe.FindString(replyLow); m != "" && !inHere(m, in.Here) {
			add(m)
		}
		if m := toldHE.FindString(in.Reply); m != "" && !inHere(m, in.Here) {
			add(m)
		}
	}
	if in.IsGroup || in.Mode == model.CrossDiscreet {
		if m := privateRe.FindString(replyLow); m != "" && !inHere(m, in.Here) {
			add(m)
		}
	}
	return out
}

// stemHit: "shifts" vs "shift", "marathons" vs "marathon", "training" vs "trained".
func stemHit(w string, toks map[string]bool) bool {
	st := stem(w)
	if utf8.RuneCountInString(st) < 4 {
		return false
	}
	for t := range toks {
		if utf8.RuneCountInString(t) >= 4 && stem(t) == st {
			return true
		}
	}
	return false
}

func stem(w string) string {
	for _, suf := range []string{"ing", "ed", "es", "s"} {
		if strings.HasSuffix(w, suf) && utf8.RuneCountInString(w)-len(suf) >= 4 {
			return strings.TrimSuffix(w, suf)
		}
	}
	return w
}

func isPresent(it Item, present map[string]bool) bool {
	if len(present) == 0 {
		return false
	}
	if u := UserOf(it.PersonJID); u != "" && present[u] {
		return true
	}
	if n := strings.ToLower(firstWord(strings.TrimSpace(it.Person))); n != "" && present[n] {
		return true
	}
	return false
}

// inHere: the phrase was already said in this chat (then it's no giveaway).
func inHere(phrase string, here []string) bool {
	phrase = strings.ToLower(phrase)
	for _, t := range here {
		if strings.Contains(strings.ToLower(t), phrase) {
			return true
		}
	}
	return false
}

// normWords lower-cases s to single-spaced words (letters/digits only).
func normWords(s string) string {
	return " " + strings.Join(memory.Tokens(s), " ") + " "
}

// trigrams are the item's three-word runs (after stop words) that contain at
// least one word not said here.
func trigrams(text string, here map[string]bool) []string {
	toks := memory.Tokens(text)
	var out []string
	for i := 0; i+3 <= len(toks); i++ {
		g := toks[i : i+3]
		fresh := false
		for _, w := range g {
			if !here[w] && !genericWords[w] && utf8.RuneCountInString(w) >= 4 {
				fresh = true
			}
		}
		if fresh {
			out = append(out, " "+strings.Join(g, " ")+" ")
		}
	}
	return out
}
