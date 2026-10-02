package behavior

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"whatsappdoppel/internal/model"
)

// QuickTypingCap bounds per-bubble typing and the gaps between bubbles for
// trigger-prefix replies and approved replies (a human already waited for them).
const QuickTypingCap = 3 * time.Second

// Splitting limits (runes).
const (
	splitMinText = 60
	splitMinPart = 12
)

// PlanInput describes one reply to plan.
type PlanInput struct {
	Text      string
	Immediate bool            // trigger prefix: no notice/wait/think, quick typing
	Approval  bool            // approval mode: no think/distraction, quick typing, never quoted
	Group     bool            // quoting only happens in groups
	Quote     *model.QuoteRef // candidate message to quote (rolled with quoteReplyPercent)
}

// Bubble is one WhatsApp message of a (possibly split) reply.
type Bubble struct {
	Text      string
	Typing    time.Duration
	GapBefore time.Duration // pause after the previous bubble was sent (0 for the first)
	Quote     *model.QuoteRef
}

// Plan is the timing of one reply cycle.
type Plan struct {
	Notice     time.Duration // before the chat is "opened" (read receipts)
	Wait       time.Duration // wait for more messages after reading
	Think      time.Duration
	Distracted time.Duration // extra delay added to Think
	Bubbles    []Bubble
	TypingOn   bool
}

// Total is the planned time from the incoming message to the last bubble.
func (p Plan) Total() time.Duration {
	t := p.Notice + p.Wait + p.Think + p.Distracted
	for _, b := range p.Bubbles {
		t += b.GapBefore + b.Typing
	}
	return t
}

// PlanNotice draws how long until the persona looks at the chat.
func PlanNotice(p model.BehaviorProfile, s Sampler) time.Duration {
	return s.Between(p.NoticeMinSec, p.NoticeMaxSec)
}

// PlanThink draws the think time and, when the distraction roll hits, the extra delay.
func PlanThink(p model.BehaviorProfile, s Sampler) (think, distracted time.Duration) {
	think = s.Between(p.ThinkMinSec, p.ThinkMaxSec)
	if s.Hit(p.DistractedPercent) {
		distracted = s.Between(p.DistractedMinSec, p.DistractedMaxSec)
	}
	return think, distracted
}

// PlanBubbles splits text (when the split roll hits) and assigns typing
// durations and gaps. quote, if non-nil, is attached to the first bubble.
func PlanBubbles(p model.BehaviorProfile, s Sampler, text string, quote *model.QuoteRef, quick bool) []Bubble {
	parts := []string{strings.TrimSpace(text)}
	if s.Hit(p.SplitPercent) {
		parts = SplitText(text, p.SplitMaxParts)
	}
	out := make([]Bubble, len(parts))
	for i, part := range parts {
		b := Bubble{Text: part, Typing: TypingDuration(p, s, utf8.RuneCountInString(part))}
		if quick {
			b.Typing = min(b.Typing, QuickTypingCap)
		}
		if i > 0 {
			b.GapBefore = s.Between(p.BubbleGapMinSec, p.BubbleGapMaxSec)
			if quick {
				b.GapBefore = min(b.GapBefore, QuickTypingCap)
			}
		} else if quote != nil {
			q := *quote
			b.Quote = &q
		}
		out[i] = b
	}
	return out
}

// PlanDelivery plans a whole reply cycle. It is pure apart from the sampler,
// so the API preview (POST /api/behavior/sample) shows exactly what the engine does.
func PlanDelivery(eff Effective, s Sampler, in PlanInput) Plan {
	p := eff.Profile
	pl := Plan{TypingOn: p.TypingIndicator}
	if !in.Immediate {
		pl.Notice = PlanNotice(p, s)
		pl.Wait = time.Duration(p.WaitForMoreSec) * time.Second
		if !in.Approval {
			pl.Think, pl.Distracted = PlanThink(p, s)
		}
	}
	pl.Bubbles = PlanBubbles(p, s, in.Text, PickQuote(eff, s, in), in.Immediate || in.Approval)
	return pl
}

// PickQuote rolls quoteReplyPercent for in.Quote: only in groups, never for
// trigger-prefix or approved replies.
func PickQuote(eff Effective, s Sampler, in PlanInput) *model.QuoteRef {
	if in.Group && in.Quote != nil && !in.Immediate && !in.Approval && s.Hit(eff.Profile.QuoteReplyPercent) {
		return in.Quote
	}
	return nil
}

// TypingDuration is how long typing runes takes: runes / typingCharsPerSec,
// jittered by typingJitterPercent and clamped to [typingMinSec, typingMaxSec].
func TypingDuration(p model.BehaviorProfile, s Sampler, runes int) time.Duration {
	cps := max(p.TypingCharsPerSec, 1)
	base := time.Duration(float64(max(runes, 0)) / float64(cps) * float64(time.Second))
	d := s.Jitter(base, p.TypingJitterPercent)
	lo := time.Duration(p.TypingMinSec) * time.Second
	hi := max(time.Duration(p.TypingMaxSec)*time.Second, lo)
	return min(max(d, lo), hi).Round(100 * time.Millisecond)
}

// ─── splitting ───────────────────────────────────────────────

type segment struct {
	text string
	sep  string // separator before this segment when merged with the previous one
}

// SplitText breaks a reply into at most maxParts chat bubbles at line breaks
// and sentence ends. Texts under 60 runes are never split, no part is shorter
// than 12 runes, and words (hence URLs) are never cut.
func SplitText(text string, maxParts int) []string {
	text = strings.TrimSpace(text)
	if maxParts < 2 || utf8.RuneCountInString(text) < splitMinText {
		return []string{text}
	}
	var segs []segment
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sep := " "
		if i > 0 && len(segs) > 0 {
			sep = "\n"
		}
		for j, sent := range sentences(line) {
			sg := segment{text: sent, sep: " "}
			if j == 0 {
				sg.sep = sep
			}
			segs = append(segs, sg)
		}
	}
	if len(segs) <= 1 {
		return []string{text}
	}
	runes := func(s segment) int { return utf8.RuneCountInString(s.text) }
	merge := func(i int) { // merge segs[i] and segs[i+1]
		segs[i].text = segs[i].text + segs[i+1].sep + segs[i+1].text
		segs = append(segs[:i+1], segs[i+2:]...)
	}
	for len(segs) > 1 {
		short := -1
		for i, sg := range segs {
			if runes(sg) < splitMinPart {
				short = i
				break
			}
		}
		switch {
		case short >= 0:
			switch {
			case short == 0:
				merge(0)
			case short == len(segs)-1:
				merge(short - 1)
			case runes(segs[short-1]) <= runes(segs[short+1]):
				merge(short - 1)
			default:
				merge(short)
			}
		case len(segs) > maxParts:
			best, bestLen := 0, -1
			for i := 0; i+1 < len(segs); i++ {
				if l := runes(segs[i]) + runes(segs[i+1]); bestLen < 0 || l < bestLen {
					best, bestLen = i, l
				}
			}
			merge(best)
		default:
			out := make([]string, len(segs))
			for i, sg := range segs {
				out[i] = sg.text
			}
			return out
		}
	}
	return []string{segs[0].text}
}

// sentences splits a line after runs of . ! ? … (plus closing quotes or
// brackets) that are followed by whitespace.
func sentences(line string) []string {
	var out []string
	rs := []rune(line)
	start := 0
	for i := 0; i < len(rs); i++ {
		if !isTerminal(rs[i]) {
			continue
		}
		j := i + 1
		for j < len(rs) && (isTerminal(rs[j]) || strings.ContainsRune(`"')]”’»`, rs[j])) {
			j++
		}
		if j < len(rs) && unicode.IsSpace(rs[j]) {
			if s := strings.TrimSpace(string(rs[start:j])); s != "" {
				out = append(out, s)
			}
			start = j
		}
		i = j - 1
	}
	if s := strings.TrimSpace(string(rs[start:])); s != "" {
		out = append(out, s)
	}
	return out
}

func isTerminal(r rune) bool { return r == '.' || r == '!' || r == '?' || r == '…' }

// ─── reactions ───────────────────────────────────────────────

// DefaultReactions are used when a persona has no favourite emoji.
var DefaultReactions = []string{"👍", "😂", "❤️", "😮", "🙏"}

// PickReaction chooses the emoji a persona reacts with: one of its favourites,
// else a default (a thumbs-up for personas that never use emoji).
func PickReaction(p model.Persona, s Sampler) string {
	var favs []string
	for _, f := range p.Emoji.Favorites {
		if f = strings.TrimSpace(f); f != "" {
			favs = append(favs, f)
		}
	}
	if len(favs) > 0 {
		return favs[s.Index(len(favs))]
	}
	if p.Emoji.Usage == "none" {
		return "👍"
	}
	return DefaultReactions[s.Index(len(DefaultReactions))]
}
