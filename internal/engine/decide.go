package engine

import (
	"context"
	"errors"
	"strings"
	"time"

	"whatsappdoppel/internal/guard"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/persona"
	"whatsappdoppel/internal/prompt"
)

// Decision reasons (activity meta "reason").
const (
	ReasonNameMentioned  = "name_mentioned"
	ReasonMentionedMe    = "mentioned_me"
	ReasonMentionsOther  = "mentions_other"
	ReasonAlwaysReply    = "always_reply" // chimeInPercent 100 without AI judgement
	ReasonLLMYes         = "llm_yes"
	ReasonLLMNo          = "llm_no"
	ReasonChimeIn        = "chime_in"  // random chime-in (v1: random_override)
	ReasonChimeOut       = "chime_out" // random roll said no (AI judgement off)
	ReasonLLMErrorRandom = "llm_error_random"
	ReasonLLMError       = "llm_error"
)

type decision struct {
	Reply  bool
	Reason string
	Meta   map[string]any
}

// decisionText is the plain-English activity text for a group decision
// (the reason code stays in meta.reason).
func decisionText(reason, name string) string {
	switch reason {
	case ReasonNameMentioned:
		return "Joining in — someone said " + name + "'s name"
	case ReasonMentionedMe:
		return "Joining in — you were @mentioned"
	case ReasonAlwaysReply:
		return "Joining in — set to reply to everything"
	case ReasonLLMYes:
		return "Joining in — the AI thinks " + name + " would jump in here"
	case ReasonChimeIn:
		return "Chiming in by chance"
	case ReasonLLMErrorRandom:
		return "Chiming in by chance — the AI check failed"
	case ReasonMentionsOther:
		return "Staying quiet — the message @mentions someone else"
	case ReasonLLMNo:
		return "Staying quiet — the AI thinks " + name + " wouldn't jump in here"
	case ReasonChimeOut:
		return "Staying quiet — didn't feel like chiming in"
	case ReasonLLMError:
		return "Staying quiet — the AI check failed"
	}
	return "Staying quiet"
}

// decide runs the group "should the persona jump in?" logic, driven by the
// behaviour profile: name mention → reply; @mention of me → reply; @mention of
// someone else → skip; otherwise ask the LLM (aiJudgement) and/or roll
// chimeInPercent (a lucky roll overrides an LLM "no").
func (e *Engine) decide(ctx context.Context, p model.Persona, bp model.BehaviorProfile, text string, mentions []string) decision {
	if bp.ReplyWhenNameMentioned && guard.ContainsName(text, p.Name) {
		return decision{true, ReasonNameMentioned, map[string]any{"reason": ReasonNameMentioned}}
	}
	own := map[string]bool{}
	var ownJIDs []string
	if e.wa != nil {
		ownJIDs = e.wa.OwnJIDs()
	}
	for _, j := range ownJIDs {
		if u := jidUser(j); u != "" {
			own[u] = true
		}
	}
	mentionedMe, mentionedOther := false, false
	for _, m := range mentions {
		switch u := jidUser(m); {
		case u == "":
		case own[u]:
			mentionedMe = true
		default:
			mentionedOther = true
		}
	}
	if mentionedMe && bp.ReplyWhenAtMentioned {
		return decision{true, ReasonMentionedMe, map[string]any{"reason": ReasonMentionedMe}}
	}
	if mentionedOther && !mentionedMe && bp.SkipWhenOthersMentioned {
		return decision{false, ReasonMentionsOther, map[string]any{"reason": ReasonMentionsOther}}
	}
	pct := bp.ChimeInPercent
	if pct >= 100 && !bp.AIJudgement {
		return decision{true, ReasonAlwaysReply, map[string]any{"reason": ReasonAlwaysReply, "percent": pct}}
	}
	lucky := e.rng.Hit(pct)
	meta := map[string]any{"percent": pct}
	if !bp.AIJudgement {
		if lucky {
			meta["reason"] = ReasonChimeIn
			return decision{true, ReasonChimeIn, meta}
		}
		meta["reason"] = ReasonChimeOut
		return decision{false, ReasonChimeOut, meta}
	}

	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err == nil {
		req := prompt.Decision(p, text)
		req.Model = mdl
		meta["provider"], meta["model"] = prov.Name(), mdl
		cctx, cancel := context.WithTimeout(ctx, llm.TimeoutDecide)
		start := time.Now()
		var resp llm.Response
		resp, err = prov.Chat(cctx, req)
		cancel()
		meta["latencyMs"] = time.Since(start).Milliseconds()
		if err == nil {
			meta["answer"] = guard.Truncate(resp.Text, 20)
			if prompt.ParseDecision(resp.Text) {
				meta["reason"] = ReasonLLMYes
				return decision{true, ReasonLLMYes, meta}
			}
		}
	}
	if err != nil {
		meta["error"] = err.Error()
		reason := ReasonLLMError
		if lucky {
			reason = ReasonLLMErrorRandom
		}
		meta["reason"] = reason
		return decision{lucky, reason, meta}
	}
	if lucky {
		meta["reason"] = ReasonChimeIn
		return decision{true, ReasonChimeIn, meta}
	}
	meta["reason"] = ReasonLLMNo
	return decision{false, ReasonLLMNo, meta}
}

// jidUser returns the user part of a JID ("972501234567:12@s.whatsapp.net" → "972501234567").
func jidUser(jid string) string {
	u, _, _ := strings.Cut(jid, "@")
	u, _, _ = strings.Cut(u, ":")
	return u
}

// genResult is the outcome of one reply generation.
type genResult struct {
	Text string
	// Drafts: co-pilot alternatives (Text == Drafts[0].Text); see drafts.go.
	Drafts   []model.Draft
	Provider string
	Model    string
	Latency  time.Duration
	Retried  bool
	Fallback bool
	// GoalRewritten: the first draft gave the goal away and was rewritten (goal.go).
	GoalRewritten bool
	// Cross-chat context (cross.go): what informed the reply; CrossRewritten
	// = a draft gave away something from another chat and was rewritten;
	// CrossDropped = in the end it was written without the other chats.
	Cross          *crossUse
	CrossRewritten bool
	CrossDropped   bool
	// Opener: written by prompt.Initiate (check-in / manual start), see opener.go.
	Opener bool
	// Expression rules (expression.go): asked again because far too long,
	// trimmed to the word budget, emoji removed / added for the emoji level.
	LengthRetried bool
	Trimmed       bool
	EmojiRemoved  int
	EmojiAdded    string
}

// crossUsed names the people whose other-chat context the reply was written
// with (nil when none, or when it was written without it in the end).
func (g genResult) crossUsed() []string {
	if !g.Cross.active() || g.CrossDropped {
		return nil
	}
	return g.Cross.People
}

func (g genResult) meta() map[string]any {
	m := map[string]any{"provider": g.Provider, "model": g.Model, "latencyMs": g.Latency.Milliseconds()}
	if g.Retried {
		m["retried"] = true
	}
	if g.Fallback {
		m["fallback"] = true
		m["reason"] = "character_break"
	}
	if g.GoalRewritten {
		m["goalRewritten"] = true
	}
	if g.Cross.active() {
		m["crossContext"] = g.Cross.meta()
	}
	if g.CrossRewritten {
		m["crossRewritten"] = true
	}
	if g.CrossDropped {
		m["crossDropped"] = true
	}
	if g.LengthRetried {
		m["lengthRetried"] = true
	}
	if g.Trimmed {
		m["trimmed"] = true
	}
	if g.EmojiRemoved > 0 {
		m["emojiRemoved"] = g.EmojiRemoved
	}
	if g.EmojiAdded != "" {
		m["emojiAdded"] = true
	}
	return m
}

// generateReply calls the persona's provider with req (from prompt.Compose or
// prompt.Initiate) and enforces character: a broken reply is retried once with
// a reminder, then replaced by the persona's fallback line.
func (e *Engine) generateReply(ctx context.Context, p model.Persona, req llm.Request) (genResult, error) {
	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err != nil {
		return genResult{}, err
	}
	s := e.cfg.Get().LLM
	req.Model, req.MaxTokens, req.Temperature = mdl, s.ReplyMaxTokens, s.Temperature
	res := genResult{Provider: prov.Name(), Model: mdl}
	start := time.Now()

	text, used, err := chatOnce(ctx, prov, req, p.Name)
	if used != "" {
		res.Model = used
	}
	if err != nil && !errors.Is(err, llm.ErrEmptyReply) {
		res.Latency = time.Since(start)
		return res, err
	}
	if err != nil || guard.BrokeCharacter(text, p.Name) {
		res.Retried = true
		req.System += prompt.CharacterReminder(p.Name)
		text, _, err = chatOnce(ctx, prov, req, p.Name)
		if err != nil && ctx.Err() != nil {
			res.Latency = time.Since(start)
			return res, err
		}
		if err != nil || guard.BrokeCharacter(text, p.Name) {
			text = strings.TrimSpace(p.FallbackReply)
			if text == "" {
				text = persona.DefaultFallbackReply
			}
			res.Fallback = true
		}
	}
	res.Text = text
	res.Latency = time.Since(start)
	return res, nil
}

func chatOnce(ctx context.Context, prov llm.Provider, req llm.Request, name string) (text, usedModel string, err error) {
	cctx, cancel := context.WithTimeout(ctx, llm.TimeoutReply)
	defer cancel()
	resp, err := prov.Chat(cctx, req)
	if err != nil {
		return "", "", err
	}
	text = cleanReply(resp.Text, name)
	if text == "" {
		return "", resp.Model, llm.ErrEmptyReply
	}
	return text, resp.Model, nil
}

// cleanReply strips wrapping quotes, markdown bold and a leading "Name:" label.
func cleanReply(s, name string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "**", ""))
	if name != "" {
		for _, n := range []string{name, firstWord(name)} {
			pre := n + ":"
			if len(s) >= len(pre) && strings.EqualFold(s[:len(pre)], pre) {
				s = strings.TrimSpace(s[len(pre):])
				break
			}
		}
	}
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && strings.Count(s, `"`) == 2 {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return s
}
