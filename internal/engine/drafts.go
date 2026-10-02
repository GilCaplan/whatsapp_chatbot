package engine

import (
	"context"
	"time"

	"whatsappdoppel/internal/goals"
	"whatsappdoppel/internal/guard"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/mention"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
)

// Co-pilot mode (wave 3): one JSON call writes three alternatives (brief,
// warm, playful); each goes through the same checks and expression rules as
// a normal reply, and they wait in Approvals (never auto-sent) until you pick
// one. When fewer than two drafts survive, a normal single reply is written
// instead. See WAVE3_PLAN §3.7.

// maxDraftsTokens caps the co-pilot call's output.
const maxDraftsTokens = 900

// replyFor writes a cycle's reply: co-pilot drafts (copilot and not an
// opener) or one reply (expressiveReply).
func (e *Engine) replyFor(ctx context.Context, copilot bool, p model.Persona, bp model.BehaviorProfile, hist []model.Message, opening bool,
	cfg goals.Config, turn prompt.GoalTurn, cross *crossUse, build buildFn) (genResult, error) {
	if copilot && !opening {
		return e.goalDrafts(ctx, p, bp, hist, cfg, turn, cross, build)
	}
	return e.expressiveReply(ctx, p, bp, hist, opening, cfg, turn, cross, build)
}

// goalDrafts writes the co-pilot alternatives in one call. Drafts that break
// character, give the goal away, give away something from another chat or
// speak as someone else are dropped; the rest get the expression rules
// (word budget, emoji level).
func (e *Engine) goalDrafts(ctx context.Context, p model.Persona, bp model.BehaviorProfile, hist []model.Message,
	cfg goals.Config, turn prompt.GoalTurn, cross *crossUse, build buildFn) (genResult, error) {
	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err != nil {
		return genResult{}, err
	}
	s := e.cfg.Get().LLM
	base := build(turn, prompt.CrossTurn{})
	req := prompt.Drafts(base)
	req.Model, req.Temperature = mdl, s.Temperature
	req.MaxTokens = min(max(3*s.ReplyMaxTokens, 240), maxDraftsTokens)
	res := genResult{Provider: prov.Name(), Model: mdl}
	start := time.Now()
	cctx, cancel := context.WithTimeout(ctx, llm.TimeoutReply)
	resp, err := prov.Chat(cctx, req)
	cancel()
	res.Latency = time.Since(start)
	if resp.Model != "" {
		res.Model = resp.Model
	}
	var drafts []model.Draft
	if err == nil {
		speakers := speakersOf(base)
		for _, d := range prompt.ParseDrafts(resp.Text) {
			d.Text = cleanReply(d.Text, p.Name)
			if d.Text == "" || guard.BrokeCharacter(d.Text, p.Name) || len(goals.Leak(cfg, d.Text)) > 0 ||
				len(cross.leaks(d.Text)) > 0 || prompt.SpeaksAs(d.Text, p.Name, speakers) != "" {
				continue
			}
			d.Text = e.expressDraft(ctx, p, bp, hist, false, genResult{Text: d.Text}, nil).Text
			drafts = append(drafts, d)
		}
	}
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	if len(drafts) < 2 {
		// Not enough good ideas: one normal reply, shown on its own.
		single, err := e.expressiveReply(ctx, p, bp, hist, false, cfg, turn, cross, build)
		single.Latency += res.Latency
		return single, err
	}
	res.Text, res.Drafts = drafts[0].Text, drafts
	if cross.active() {
		res.Cross = cross
	}
	return res, nil
}

// speakersOf lists the other people of a group reply request ("Name: …"
// user turns), for prompt.SpeaksAs.
func speakersOf(req llm.Request) []model.Message {
	var out []model.Message
	for _, m := range req.Messages {
		if m.Role != llm.RoleUser {
			continue
		}
		for i := 0; i < len(m.Content) && i < 40; i++ {
			if m.Content[i] == '\n' {
				break
			}
			if m.Content[i] == ':' && i+1 < len(m.Content) && m.Content[i+1] == ' ' {
				out = append(out, model.Message{Speaker: "them", Name: m.Content[:i]})
				break
			}
		}
	}
	return out
}

// applyMentionsAll applies the @mention rules to the reply and, in co-pilot
// mode, to every draft (Text = the first draft). It returns the people the
// reply tags.
func (e *Engine) applyMentionsAll(res *genResult, d *mention.Directory, bp model.BehaviorProfile, answered pendingMsg, hist []model.Message, proactive bool) []mention.Entry {
	if len(res.Drafts) == 0 {
		var tagged []mention.Entry
		res.Text, tagged = e.applyMentions(res.Text, d, bp, answered, hist, proactive)
		return tagged
	}
	var first []mention.Entry
	for i := range res.Drafts {
		text, tagged := e.applyMentions(res.Drafts[i].Text, d, bp, answered, hist, proactive)
		res.Drafts[i].Text, res.Drafts[i].Mentions = text, mention.Names(tagged)
		if i == 0 {
			first = tagged
		}
	}
	res.Text = res.Drafts[0].Text
	return first
}
