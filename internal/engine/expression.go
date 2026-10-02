package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"whatsappdoppel/internal/goals"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
)

// Expression: the persona's emoji level and word budget are prompt
// instructions first, then enforced on the generated text here, so the
// settings mean what they say even when a small model ignores them. Every
// generated reply takes this path: auto replies, approval drafts and
// regenerations, check-ins / "start the conversation", the playground and
// the persona editor's expression preview. Texts you write or edit are never
// touched.

// lengthRetryMaxLatency: a far-too-long draft is asked for again only when
// the first one came back faster than this (and the context has time left).
const lengthRetryMaxLatency = 20 * time.Second

// expressiveReply is goalReply plus the expression rules. hist is the
// history the request was built from; opening is true for check-ins and
// openers (nothing to answer: neutral tone, base word budget).
func (e *Engine) expressiveReply(ctx context.Context, p model.Persona, bp model.BehaviorProfile, hist []model.Message, opening bool,
	cfg goals.Config, turn prompt.GoalTurn, cross *crossUse, build buildFn) (genResult, error) {
	res, err := e.guardedReply(ctx, p, cfg, turn, cross, build)
	if err != nil {
		return res, err
	}
	return e.expressDraft(ctx, p, bp, hist, opening, res, func(note string) (genResult, error) {
		return e.guardedReply(ctx, p, cfg, turn, cross, func(t prompt.GoalTurn, x prompt.CrossTurn) llm.Request {
			r := build(t, x)
			r.System += note
			return r
		})
	}), nil
}

// expressDraft applies the expression rules to a generated draft: a draft
// far over the word budget is asked for again once (regen appends note to
// the system prompt) when time allows, then trimmed; then the emoji level is
// enforced. The owner's fallback line is left as written.
func (e *Engine) expressDraft(ctx context.Context, p model.Persona, bp model.BehaviorProfile, hist []model.Message, opening bool,
	res genResult, regen func(note string) (genResult, error)) genResult {
	if res.Fallback {
		return res
	}
	last, incoming := "", ""
	if !opening {
		incoming = prompt.IncomingText(hist)
		if n := len(hist); n > 0 {
			last = hist[n-1].Text
		}
	}
	budget := prompt.WordBudget(p.MessageLength, bp.LengthBias, last)
	limit := prompt.HardLimit(budget)
	if words := prompt.WordCount(res.Text); words > limit && regen != nil && retryAllowed(ctx, res.Latency) {
		again, err := regen(prompt.LengthRetryNote(budget, words))
		if err == nil && !again.Fallback && prompt.WordCount(again.Text) < words {
			again.Latency += res.Latency
			again.GoalRewritten = again.GoalRewritten || res.GoalRewritten
			again.CrossRewritten = again.CrossRewritten || res.CrossRewritten
			res = again
		}
		res.LengthRetried = true
	}
	if t, ok := prompt.TrimToWords(res.Text, limit); ok {
		res.Text, res.Trimmed = t, true
	}
	tone := prompt.ToneNeutral
	if !opening {
		tone = prompt.ClassifyTone(incoming)
	}
	er := prompt.EnforceEmoji(res.Text, p.Emoji, tone, myRecentTexts(hist, prompt.RareGap), e.rng)
	res.Text, res.EmojiRemoved, res.EmojiAdded = er.Text, er.Removed, er.Added
	if strings.TrimSpace(res.Text) == "" { // it was nothing but emoji
		res.Text = prompt.WordsFor(tone)
	}
	return res
}

// retryAllowed reports whether there is time for one more generation.
func retryAllowed(ctx context.Context, took time.Duration) bool {
	if took > lengthRetryMaxLatency || ctx.Err() != nil {
		return false
	}
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) < 2*took+2*time.Second {
		return false
	}
	return true
}

// myRecentTexts are the persona's last n messages in hist, oldest first.
func myRecentTexts(hist []model.Message, n int) []string {
	var out []string
	for i := len(hist) - 1; i >= 0 && len(out) < n; i-- {
		if hist[i].Speaker == "me" {
			out = append([]string{hist[i].Text}, out...)
		}
	}
	return out
}

// ─── reactions ───────────────────────────────────────────────

// reactionFor picks the reaction to a message by its meaning (see
// prompt.ReactionFor) and describes it in words for the activity feed
// ("Reacted to Dana's joke with a laugh instead of replying").
func (e *Engine) reactionFor(p model.Persona, name, text string) (emoji, activity string, tone prompt.Tone) {
	emoji, tone = prompt.ReactionFor(p.Emoji, text, e.rng)
	who := "their"
	if name != "" {
		who = name + "'s"
	}
	return emoji, fmt.Sprintf("Reacted to %s %s with %s instead of replying", who, toneNoun(tone), emojiWords(emoji)), tone
}

func toneNoun(t prompt.Tone) string {
	switch t {
	case prompt.ToneFunny:
		return "joke"
	case prompt.ToneGood:
		return "good news"
	case prompt.ToneLove:
		return "kind words"
	case prompt.ToneSurprise:
		return "news"
	case prompt.TonePlan:
		return "plan"
	case prompt.ToneQuestion:
		return "question"
	case prompt.ToneGreeting:
		return "hello"
	}
	return "message"
}

// emojiNames describes reaction emoji in words (activity texts show no emoji).
var emojiNames = map[string]string{
	"😂": "a laugh", "🤣": "a laugh", "😆": "a laugh", "💀": "a laugh", "😅": "a nervous laugh",
	"❤️": "a heart", "❤": "a heart", "🤍": "a heart", "💕": "hearts", "🥰": "a loving face", "😍": "heart eyes",
	"🎉": "a party popper", "🥳": "a party face", "🔥": "a fire", "🙌": "raised hands", "👏": "applause",
	"🍾": "champagne", "🥂": "a toast", "🍸": "a cocktail", "🍻": "a toast", "💪": "a flexed arm", "🚀": "a rocket",
	"✨": "sparkles", "💅": "a manicure", "📈": "a chart going up", "💎": "a diamond", "💸": "money", "💯": "a hundred",
	"🙏": "praying hands", "😢": "a sad face", "😔": "a sad face", "🫂": "a hug", "🤗": "a hug", "😤": "a huff",
	"😮": "a surprised face", "😱": "a shocked face", "👀": "eyes", "🤯": "a mind blown", "🍵": "tea",
	"👍": "a thumbs-up", "👌": "an OK hand", "🤝": "a handshake", "🤔": "a thinking face", "👋": "a wave",
	"🧿": "an evil eye", "🔮": "a crystal ball", "🧠": "a brain",
}

func emojiWords(emoji string) string {
	if w, ok := emojiNames[emoji]; ok {
		return w
	}
	if w, ok := emojiNames[strings.TrimSuffix(emoji, "️")]; ok {
		return w
	}
	return "an emoji"
}

// ─── expression preview (persona editor) ─────────────────────

// previewPool are the incoming messages of the expression preview: light,
// everyday and serious ones, so emoji and length settings show.
var previewPool = []struct{ group, text string }{
	{"light", "haha my cat just knocked my coffee onto my laptop and then stared at me"},
	{"light", "I GOT THE JOB!!! starting next month"},
	{"light", "we're engaged!! he proposed last night"},
	{"light", "thank you so much for yesterday, you're the best"},
	{"everyday", "drinks on friday at 8?"},
	{"everyday", "what are you up to this weekend?"},
	{"everyday", "do you know any good restaurants around here?"},
	{"everyday", "hey"},
	{"deep", "ok I need to vent. my manager presented my work as his own idea in front of the whole team today and then asked me why the numbers were late. I just sat there smiling. should I say something?"},
	{"deep", "I got laid off today. not really sure what to do now"},
	{"deep", "my grandmother passed away this morning"},
	{"deep", "what's the best advice you've ever gotten?"},
}

// ExpressionPreviewMax caps the samples of one preview.
const ExpressionPreviewMax = 3

// ExpressionPreview writes sample replies of p (possibly unsaved) to a few
// incoming messages at its emoji and length settings, as a DM with the
// private behaviour profile (lengthBias overridden when not ""). roll picks
// different messages (re-roll).
func (e *Engine) ExpressionPreview(ctx context.Context, p model.Persona, lengthBias string, count, roll int) (model.ExpressionPreview, error) {
	count = min(max(count, 1), ExpressionPreviewMax)
	bp := e.cfg.Get().Behavior.Private
	if lengthBias != "" {
		bp.LengthBias = lengthBias
	}
	out := model.ExpressionPreview{Samples: make([]model.ExpressionSample, count)}
	if prov, mdl, err := e.llm.Resolve(p.LLM); err == nil {
		out.Provider, out.Model = prov.Name(), mdl
	} else {
		return out, err
	}
	groups := []string{"light", "everyday", "deep"}
	start := time.Now()
	var wg sync.WaitGroup
	errs := make([]error, count)
	for i := range count {
		var pool []string
		for _, m := range previewPool {
			if m.group == groups[i%len(groups)] {
				pool = append(pool, m.text)
			}
		}
		incoming := pool[((roll%len(pool))+len(pool)+i/len(groups))%len(pool)]
		wg.Add(1)
		go func() {
			defer wg.Done()
			chat := model.ChatAssignment{Key: "preview:expression", Kind: "dm", Name: "Friend", PersonaID: p.ID, Enabled: true}
			hist := []model.Message{{ID: "preview", TS: time.Now(), Speaker: "them", Name: "Friend", Text: incoming}}
			opts := prompt.Options{LengthBias: bp.LengthBias}
			res, err := e.expressiveReply(ctx, p, bp, hist, false, goals.Resolve(p, chat), prompt.GoalTurn{Off: true}, nil, func(t prompt.GoalTurn, _ prompt.CrossTurn) llm.Request {
				opts.Goal = t
				return prompt.Compose(p, chat, hist, false, opts)
			})
			if err != nil {
				errs[i] = err
				return
			}
			out.Samples[i] = model.ExpressionSample{
				Incoming: incoming, Reply: res.Text, Words: prompt.WordCount(res.Text), Emoji: prompt.CountEmoji(res.Text),
				Budget: prompt.WordBudget(p.MessageLength, bp.LengthBias, incoming), Tone: string(prompt.ClassifyTone(incoming)),
				Adjusted: res.Trimmed || res.LengthRetried || res.EmojiRemoved > 0 || res.EmojiAdded != "",
			}
		}()
	}
	wg.Wait()
	out.LatencyMs = time.Since(start).Milliseconds()
	for _, err := range errs {
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
