package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"whatsappdoppel/internal/core/events"
	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/core/store"
	"whatsappdoppel/internal/logic/ctxsys"
	"whatsappdoppel/internal/logic/goals"
	"whatsappdoppel/internal/runtime/llm"
	"whatsappdoppel/internal/runtime/prompt"
)

// The context-system sandbox: a chat's last few messages from them are
// answered again with each chosen system — joining in, planning, the reply
// and its check, exactly as the chat would — and every AI call is counted
// (llm.Meter). Nothing is sent, and the chat's history, goal progress and
// runtime state are left alone; only a missing persona card is made (and
// kept) when a system needs one.

const (
	replayMaxMessages = 20
	replayKeep        = 5 // finished runs kept for the page
	replayLimit       = 30 * time.Minute
)

type replayRegistry struct {
	mu   sync.Mutex
	runs []*replayRun
}

type replayRun struct {
	mu     sync.Mutex
	r      model.Replay
	cancel context.CancelFunc
}

func (rr *replayRun) view() model.Replay {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	v := rr.r
	v.Results = make([]model.ReplayResult, len(rr.r.Results))
	for i, res := range rr.r.Results {
		res.Steps = append([]model.ReplayStep{}, res.Steps...) // [] not null while it starts
		by := make(map[string]int, len(res.CallsByStep))
		for k, n := range res.CallsByStep {
			by[k] = n
		}
		res.CallsByStep = by
		v.Results[i] = res
	}
	return v
}

// StartReplay starts a sandbox run in the background.
func (e *Engine) StartReplay(req model.ReplayRequest) (model.Replay, error) {
	c, ok := e.store.Chat(req.ChatKey)
	if !ok {
		return model.Replay{}, fmt.Errorf("chat %q: %w", req.ChatKey, ErrNotFound)
	}
	if _, ok := e.store.Persona(c.PersonaID); !ok {
		return model.Replay{}, fmt.Errorf("the chat has no persona")
	}
	var systems []model.ContextSystem
	for _, id := range req.Systems {
		s, ok := e.store.ContextSystem(id)
		if !ok {
			return model.Replay{}, fmt.Errorf("context system %q: %w", id, ErrNotFound)
		}
		systems = append(systems, s)
	}
	if len(systems) == 0 || len(systems) > 3 {
		return model.Replay{}, errors.New("pick one to three systems")
	}
	n := req.Messages
	if n <= 0 {
		n = 8
	}
	n = min(n, replayMaxMessages)
	all, err := e.store.History(c.Key, 0)
	if err != nil {
		return model.Replay{}, err
	}
	all = fromContext(all, c.ContextFrom)
	var targets []int
	for i := len(all) - 1; i >= 0 && len(targets) < n; i-- {
		if all[i].Speaker == "them" && all[i].Kind != model.MsgKindFix && strings.TrimSpace(all[i].Text) != "" {
			targets = append([]int{i}, targets...)
		}
	}
	if len(targets) == 0 {
		return model.Replay{}, errors.New("this chat has no messages from them to replay yet")
	}
	ctx, cancel := context.WithTimeout(e.ctx, replayLimit)
	run := &replayRun{cancel: cancel, r: model.Replay{ID: "rp-" + store.NewID(), ChatKey: c.Key, ChatName: c.Name, Status: "running",
		Total: len(targets) * len(systems), StartedAt: e.clock.Now(), Results: []model.ReplayResult{}}}
	e.replays.mu.Lock()
	e.replays.runs = append(e.replays.runs, run)
	if len(e.replays.runs) > replayKeep {
		e.replays.runs = e.replays.runs[len(e.replays.runs)-replayKeep:]
	}
	e.replays.mu.Unlock()
	go func() { // not counted in Engine.busy: it may run for minutes
		defer cancel()
		e.runReplay(ctx, run, c, systems, all, targets)
	}()
	return run.view(), nil
}

// Replay returns a sandbox run.
func (e *Engine) Replay(id string) (model.Replay, bool) {
	e.replays.mu.Lock()
	defer e.replays.mu.Unlock()
	for _, r := range e.replays.runs {
		if r.r.ID == id {
			return r.view(), true
		}
	}
	return model.Replay{}, false
}

// CancelReplay stops a running sandbox run.
func (e *Engine) CancelReplay(id string) bool {
	e.replays.mu.Lock()
	defer e.replays.mu.Unlock()
	for _, r := range e.replays.runs {
		if r.r.ID == id {
			r.cancel()
			return true
		}
	}
	return false
}

func (e *Engine) replayChanged(id string) {
	if e.hub != nil {
		e.hub.Publish(events.TypeContextChanged, map[string]string{"replayId": id})
	}
}

func (e *Engine) runReplay(ctx context.Context, run *replayRun, c model.ChatAssignment, systems []model.ContextSystem, all []model.Message, targets []int) {
	finish := func(status, msg string) {
		now := e.clock.Now()
		run.mu.Lock()
		run.r.Status, run.r.Error, run.r.EndedAt = status, msg, &now
		run.mu.Unlock()
		e.replayChanged(run.r.ID)
	}
	for _, sys := range systems {
		cp := ctxsys.Clamp(sys.Params, sys.Base)
		res := model.ReplayResult{SystemID: sys.ID, SystemName: sys.Name, CallsByStep: map[string]int{}, Steps: []model.ReplayStep{}}
		run.mu.Lock()
		run.r.Results = append(run.r.Results, res)
		idx := len(run.r.Results) - 1
		run.mu.Unlock()
		goal := e.goalStatus(c.Key, goals.Resolve(e.personaOr(c), c).Text) // a copy: progress stays local
		goal.Plans = slices.Clone(goal.Plans)
		// The replayed messages are older than now: plan from the first one
		// and let all of them count as evidence.
		goal.LastPlanAt = nil
		if first := all[targets[0]].TS; !first.IsZero() && goal.Since.After(first) {
			goal.Since = first
		}
		prompts, replies := 0, 0
		for _, t := range targets {
			if ctx.Err() != nil {
				status := "cancelled"
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					status = "failed"
				}
				finish(status, "")
				return
			}
			step, chars := e.replayStep(ctx, c, cp, &goal, all, t)
			if chars > 0 {
				prompts += chars
				replies++
			}
			run.mu.Lock()
			r := &run.r.Results[idx]
			r.Steps = append(r.Steps, step.ReplayStep)
			r.Calls += step.Calls
			r.TotalMs += step.Ms
			for k, v := range step.by {
				r.CallsByStep[k] += v
			}
			if step.Decision == "reply" {
				r.Replied++
			} else {
				r.Skipped++
			}
			if replies > 0 {
				r.PromptChars = prompts / replies
			}
			run.r.Done++
			run.mu.Unlock()
			e.replayChanged(run.r.ID)
		}
		run.mu.Lock()
		run.r.Results[idx].Background = replayBackground(cp, len(targets))
		run.mu.Unlock()
	}
	finish("done", "")
}

// replayBackground is how many background calls this many messages from
// them add with the system's knobs (memories, brief, summary).
func replayBackground(cp model.ContextParams, n int) int {
	calls := float64(n) / 70 // folding into the summary
	if cp.Background == model.BackgroundDigest {
		calls += float64(n) / float64(max(1, cp.DigestEvery))
	} else {
		calls += float64(n)/memoryEvery + float64(n)/briefEvery
	}
	return int(calls + 0.5)
}

func (e *Engine) personaOr(c model.ChatAssignment) model.Persona {
	p, _ := e.store.Persona(c.PersonaID)
	return p
}

type replayOut struct {
	model.ReplayStep
	by map[string]int
}

// replayStep answers all[t] again with knobs cp; it returns the step and the
// size of its reply prompt (0 = no reply written).
func (e *Engine) replayStep(ctx context.Context, c model.ChatAssignment, cp model.ContextParams, goal *model.GoalStatus, all []model.Message, t int) (out replayOut, chars int) {
	p := e.personaOr(c)
	msg := all[t]
	hist := all[:t]
	out = replayOut{ReplayStep: model.ReplayStep{From: orName(msg.Name), Text: msg.Text}}
	for _, m := range all[t+1:] {
		if m.Speaker == "them" {
			break
		}
		if m.Speaker == "me" && m.FromBot {
			out.Real = m.Text
			break
		}
	}
	meter := &llm.Meter{}
	mctx := llm.WithMeter(ctx, meter)
	start := time.Now()
	defer func() {
		out.Ms = time.Since(start).Milliseconds()
		out.Calls, out.by = meter.Snapshot()
	}()
	bp := e.effective(c).Profile
	isGroup := isGroupChat(c)
	at := msg.TS // the message's own time, not now: rules and planning compare with history
	if at.IsZero() {
		at = e.clock.Now()
	}
	if isGroup {
		meter.Step("join")
		d := e.decide(mctx, p, bp, cp, msg.Text, msg.Name, nil, hist, at, false)
		out.Why = decisionText(d.Reason, p.Name)
		if !d.Reply {
			out.Decision = "skip"
			return out, 0
		}
	}
	full := all[:t+1]
	meter.Step("plan")
	cfg := goals.Resolve(p, c)
	if goal.Goal != cfg.Text {
		*goal = freshGoalStatus(cfg.Text, at)
	}
	gp := e.planAhead(mctx, p, cfg, cp, *goal, full, isGroup, false)
	if gp.planned || gp.reachedNow {
		*goal = gp.status
		if gp.planned {
			goal.LastPlanAt = &at
		}
	}
	out.Plan = gp.turn.Plan
	dir := e.directory(mctx, c, full)
	opts, cross := e.promptOptions(c, bp, dir, full)
	if cp.PromptStyle == model.PromptCard && strings.TrimSpace(p.Card) == "" {
		meter.Step("card")
		if np, err := e.MakeCard(mctx, p.ID, cp.CardChars); err == nil {
			p = np
		}
	}
	ph := e.shapePrompt(&opts, cp, p, full)
	build := func(gt prompt.GoalTurn, x prompt.CrossTurn) llm.Request {
		opts.Goal, opts.CrossTurn = gt, x
		return prompt.Compose(p, c, ph, isGroup, opts)
	}
	meter.Step("reply")
	res, err := e.replyFor(mctx, false, p, bp, ph, false, cfg, gp.turn, cross, build)
	chars = meter.Last
	if err != nil {
		out.Decision, out.Why = "skip", "Couldn't write a reply: "+err.Error()
		return out, 0
	}
	if cp.ReplyCheck != model.CheckOff || opts.LanguageFixed {
		meter.Step("check")
		o := opts
		o.Goal = gp.turn
		ck := replyCheck{cp: cp, persona: p, plan: gp.turn.Plan, lang: opts.ReplyLanguage, langFixed: opts.LanguageFixed, hist: full,
			rules: checkRules(prompt.ComposeSections(p, c, ph, isGroup, o), p)}
		res = e.checkedReply(mctx, ck, res, func(note string) (genResult, error) {
			meter.Step("rewrite")
			defer meter.Step("check")
			return e.replyFor(mctx, false, p, bp, ph, false, cfg, gp.turn, cross, func(gt prompt.GoalTurn, x prompt.CrossTurn) llm.Request {
				r := build(gt, x)
				r.System += note
				return r
			})
		})
		out.Checks = res.Checks
	}
	if !res.Quiet {
		res = e.finishReply(mctx, p, e.replyLanguage(c, full), full, res, nil)
		out.Checks = res.Checks
	}
	if res.Quiet {
		out.Decision, out.Why = "skip", "Stayed quiet: the reply didn't pass the check"
		return out, chars
	}
	out.Decision, out.Reply = "reply", res.Text
	if out.Why == "" {
		out.Why = "Answered"
	}
	return out, chars
}
