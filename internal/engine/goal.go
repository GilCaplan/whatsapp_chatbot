package engine

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/goals"
	"whatsappdoppel/internal/guard"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/mission"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/persona"
	"whatsappdoppel/internal/prompt"
	"whatsappdoppel/internal/store"
)

// Goal pursuit: before a reply, an optional private "plan ahead" call reads the
// chat and suggests the next move (and whether the goal already happened);
// the move goes into the reply prompt as a private note. Replies that give the
// goal away are regenerated. Progress is kept per chat in runtime.json.

// timeoutPlan bounds the plan-ahead call; on timeout the reply goes ahead without a plan.
const timeoutPlan = 30 * time.Second

// maxGoalPlans is how many earlier moves are fed back to the planner.
const maxGoalPlans = 4

// goalEvidenceSlack lets a message that arrived just before the goal status
// was created still count as evidence.
const goalEvidenceSlack = 5 * time.Minute

// freshGoalStatus starts a status for goal at now.
func freshGoalStatus(goal string, now time.Time) model.GoalStatus {
	return model.GoalStatus{Goal: goal, Since: now}
}

// ensureGoal makes st a status for goal (a different or missing goal starts over).
func ensureGoal(st *store.RunnerState, goal string, now time.Time) *model.GoalStatus {
	if st.Goal == nil || st.Goal.Goal != goal {
		g := freshGoalStatus(goal, now)
		st.Goal = &g
	}
	return st.Goal
}

// goalStatus returns a chat's status for goal (fresh when none or for another goal).
func (e *Engine) goalStatus(key, goal string) model.GoalStatus {
	st := e.store.RunnerState(key)
	if st.Goal == nil || st.Goal.Goal != goal {
		return freshGoalStatus(goal, e.clock.Now())
	}
	return *st.Goal
}

// updateGoal changes a chat's goal status atomically (started over when the goal changed).
func (e *Engine) updateGoal(key, goal string, fn func(g *model.GoalStatus)) model.GoalStatus {
	now := e.clock.Now()
	var out model.GoalStatus
	e.updateState(key, func(st *store.RunnerState) {
		g := ensureGoal(st, goal, now)
		fn(g)
		out = g.Clone()
	})
	return out
}

// goalPlan is the outcome of the plan-ahead step for one reply.
type goalPlan struct {
	cfg        goals.Config
	turn       prompt.GoalTurn
	planned    bool // the planner ran and returned a move
	latency    time.Duration
	reachedNow bool
	status     model.GoalStatus
}

// planAhead runs the private strategist for a reply when the goal settings
// ask for it. st is the current status (read-only); the caller persists the
// result with applyGoalPlan. Failures are silent: the reply goes ahead
// without a plan.
func (e *Engine) planAhead(ctx context.Context, p model.Persona, cfg goals.Config, st model.GoalStatus, hist []model.Message, isGroup, opening bool) goalPlan {
	gp := goalPlan{cfg: cfg, status: st, turn: prompt.GoalTurn{Reached: st.Reached}}
	if !cfg.PlanAhead || strings.TrimSpace(cfg.Text) == "" || cfg.Relaxed(gp.turn.Reached) {
		return gp
	}
	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err != nil {
		return gp
	}
	// Only messages since the goal was set can accomplish it.
	var recent []model.Message
	theirs := false
	for _, m := range hist {
		if m.TS.IsZero() || !m.TS.Before(st.Since.Add(-goalEvidenceSlack)) {
			recent = append(recent, m)
			theirs = theirs || m.Speaker != "me"
		}
	}
	sw, isSay := goals.ParseSayWord(cfg.Text)

	// "Has it already happened?" is a separate, narrow question (small models
	// answer it much more reliably than inside the planning request); it runs
	// alongside the planner. Say-the-word goals use the exact check instead.
	var (
		wg         sync.WaitGroup
		checkYes   bool
		checkQuote string
	)
	if !isSay && !st.Reached && theirs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := prompt.GoalCheck(p, cfg, recent)
			req.Model = mdl
			cctx, cancel := context.WithTimeout(ctx, timeoutPlan)
			defer cancel()
			if resp, err := prov.Chat(cctx, req); err == nil {
				checkYes, checkQuote, _ = prompt.ParseGoalCheck(resp.Text)
			}
		}()
	}

	req := prompt.Plan(p, cfg, hist, isGroup, st.Plans, prompt.PlanOptions{Opening: opening})
	req.Model = mdl
	cctx, cancel := context.WithTimeout(ctx, timeoutPlan)
	start := time.Now()
	resp, err := prov.Chat(cctx, req)
	cancel()
	wg.Wait()
	gp.latency = time.Since(start)
	var res prompt.PlanResult
	if err == nil {
		res, _ = prompt.ParsePlan(resp.Text)
	}

	// Reached only with evidence that is really in their recent messages.
	if !isSay && !st.Reached {
		for _, c := range []struct {
			yes   bool
			quote string
		}{{checkYes, checkQuote}, {res.Achieved, res.Evidence}} {
			if !c.yes {
				continue
			}
			if m, ok := goals.EvidenceFound(c.quote, recent); ok {
				gp.reachedNow = true
				gp.status.Reached = true
				now := e.clock.Now()
				gp.status.ReachedAt, gp.status.How = &now, model.GoalHowAI
				gp.status.Evidence = evidenceLine(m.Name, m.Text)
				gp.turn.Reached = true
				break
			}
		}
	}
	if isSay && goals.SaysWord(res.Next, sw.Word) && cfg.Style != model.GoalStyleDirect {
		res.Next = "" // never feed the target word into the reply prompt
	}
	if cfg.Relaxed(gp.turn.Reached) {
		return gp
	}
	if res.Next != "" {
		gp.planned = true
		gp.turn.Plan = res.Next
		now := e.clock.Now()
		gp.status.LastPlan, gp.status.LastPlanAt = res.Next, &now
		gp.status.Plans = append(gp.status.Plans, res.Next)
		if n := len(gp.status.Plans); n > maxGoalPlans {
			gp.status.Plans = gp.status.Plans[n-maxGoalPlans:]
		}
	}
	return gp
}

func evidenceLine(name, text string) string {
	text = guard.Truncate(strings.Join(strings.Fields(text), " "), 160)
	if name == "" {
		return text
	}
	return name + ": " + text
}

// chatGoalTurn runs the goal step for a real chat: plan ahead, persist the
// move/achievement and publish activity. It returns the turn for the prompt.
func (e *Engine) chatGoalTurn(ctx context.Context, c model.ChatAssignment, p model.Persona, hist []model.Message, isGroup, opening bool) (goals.Config, prompt.GoalTurn) {
	cfg := goals.Resolve(p, c)
	st := e.goalStatus(c.Key, cfg.Text)
	gp := e.planAhead(ctx, p, cfg, st, hist, isGroup, opening)
	if ctx.Err() != nil {
		return cfg, gp.turn
	}
	if gp.planned || gp.reachedNow {
		var final model.GoalStatus
		reachedNow := false
		final = e.updateGoal(c.Key, cfg.Text, func(g *model.GoalStatus) {
			if gp.planned {
				g.LastPlan, g.LastPlanAt, g.Plans = gp.status.LastPlan, gp.status.LastPlanAt, gp.status.Plans
			}
			if gp.reachedNow && !g.Reached {
				g.Reached, g.ReachedAt, g.How, g.Evidence = true, gp.status.ReachedAt, gp.status.How, gp.status.Evidence
				reachedNow = true
			}
		})
		if reachedNow {
			e.goalReached(c, p, cfg, final)
		}
	}
	if gp.planned {
		e.act(model.ActThinking, c, p.Name, "Planned the next move", map[string]any{
			"plan": gp.turn.Plan, "stage": "plan", "latencyMs": gp.latency.Milliseconds(),
		})
	}
	return cfg, gp.turn
}

// goalIncoming checks a message from them against a say-the-word goal
// ("get Josh to say apple"): the exact word from the right person reaches it.
func (e *Engine) goalIncoming(c model.ChatAssignment, p model.Persona, isGroup bool, name, text, media string) {
	cfg := goals.Resolve(p, c)
	if e.goalMedia(c, p, cfg, name, text, media) {
		return
	}
	sw, ok := goals.ParseSayWord(cfg.Text)
	if !ok {
		return
	}
	who := sw.Who
	if !isGroup {
		who = "" // a DM has only one other person
	}
	if !goals.SpeakerMatches(name, who) || !goals.SaysWord(text, sw.Word) {
		return
	}
	if st := e.goalStatus(c.Key, cfg.Text); st.Reached {
		return
	}
	now := e.clock.Now()
	reachedNow := false
	final := e.updateGoal(c.Key, cfg.Text, func(g *model.GoalStatus) {
		if g.Reached {
			return
		}
		g.Reached, g.ReachedAt, g.How, g.Evidence = true, &now, model.GoalHowSaidWord, evidenceLine(name, text)
		reachedNow = true
	})
	if reachedNow {
		who := name
		if who == "" {
			who = "They"
		}
		e.publishGoalReached(c, p, cfg, final, "Goal reached — "+who+" said "+sw.Word)
	}
}

// goalReached publishes the activity for a goal the planner saw happen.
func (e *Engine) goalReached(c model.ChatAssignment, p model.Persona, cfg goals.Config, st model.GoalStatus) {
	e.publishGoalReached(c, p, cfg, st, "Goal reached — "+lowerFirst(strings.TrimRight(cfg.Text, ".!")))
}

func (e *Engine) publishGoalReached(c model.ChatAssignment, p model.Persona, cfg goals.Config, st model.GoalStatus, text string) {
	meta := map[string]any{"goal": cfg.Text, "how": st.How, "evidence": st.Evidence, "afterReached": cfg.AfterReached}
	if c.MissionID != "" && c.GoalOverride == cfg.Text {
		meta["missionId"] = c.MissionID
	}
	e.act(model.ActGoalReached, c, p.Name, text, meta)
	// Missions page: every reached goal completes (or records) a mission.
	_, completed, _ := e.store.CompleteMission(c.Key, cfg.Text, st)
	if e.hub != nil {
		e.hub.Publish(events.TypeChatsChanged, struct{}{})
		if completed {
			e.hub.Publish(events.TypeMissionsChanged, struct{}{})
		}
	}
}

// goalMedia reaches a media mission ("get Dana to send you a photo") when
// they send that kind of media while the mission is the chat's goal.
func (e *Engine) goalMedia(c model.ChatAssignment, p model.Persona, cfg goals.Config, name, text, media string) bool {
	if c.MissionID == "" || c.GoalOverride != cfg.Text || !mission.MediaDetected(c.MissionID, media) {
		return false
	}
	if st := e.goalStatus(c.Key, cfg.Text); st.Reached {
		return true
	}
	now := e.clock.Now()
	reachedNow := false
	final := e.updateGoal(c.Key, cfg.Text, func(g *model.GoalStatus) {
		if g.Reached {
			return
		}
		g.Reached, g.ReachedAt, g.How, g.Evidence = true, &now, mission.HowMedia, evidenceLine(name, text)
		reachedNow = true
	})
	if reachedNow {
		who := name
		if who == "" {
			who = "They"
		}
		what := map[string]string{model.MediaImage: "a photo", model.MediaAudio: "a voice note", model.MediaVideo: "a video", model.MediaSticker: "a sticker"}[media]
		e.publishGoalReached(c, p, cfg, final, "Goal reached — "+who+" sent "+what)
	}
	return true
}

func lowerFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 || !unicode.IsUpper(r) {
		return s
	}
	// Keep names and acronyms ("Josh", "NYC") as they are.
	if n2 := len(s); n < n2 {
		next, _ := utf8.DecodeRuneInString(s[n:])
		if unicode.IsUpper(next) {
			return s
		}
	}
	return string(unicode.ToLower(r)) + s[n:]
}

// goalReply generates a reply with the goal turn and guards against leaks: a
// draft that gives the goal away is rewritten once with a reminder, then — if
// it still leaks — written without the agenda at all. build renders the
// request for a given turn (prompt.Compose / prompt.Initiate).
func (e *Engine) goalReply(ctx context.Context, p model.Persona, cfg goals.Config, turn prompt.GoalTurn, build func(prompt.GoalTurn) llm.Request) (genResult, error) {
	res, err := e.generateReply(ctx, p, build(turn))
	if err == nil && !res.Fallback {
		res = e.ownVoice(ctx, p, res, build(turn))
	}
	if err != nil || res.Fallback || len(goals.Leak(cfg, res.Text)) == 0 {
		return res, err
	}
	first := res
	retry := turn
	retry.Retry = true
	res, err = e.generateReply(ctx, p, build(retry))
	if err == nil && !res.Fallback && len(goals.Leak(cfg, res.Text)) == 0 {
		res.GoalRewritten = true
		return res, nil
	}
	if ctx.Err() != nil {
		return first, ctx.Err()
	}
	// Last resort: no agenda this time. If even that fails, send nothing
	// rather than the draft that gave the goal away.
	res, err = e.generateReply(ctx, p, build(prompt.GoalTurn{Off: true}))
	if err != nil {
		return first, err
	}
	res.GoalRewritten = true
	return res, nil
}

// senderName is who wrote an incoming message (push name; the chat name in DMs).
func senderName(in model.Incoming, c model.ChatAssignment, isGroup bool) string {
	if in.PushName != "" || isGroup {
		return in.PushName
	}
	return c.Name
}

// playgroundGoalTurn is the goal step of a playground turn: st is the
// session's in-memory status, lastText the tester's message (who plays
// everyone, so any speaker counts for say-the-word goals).
func (e *Engine) playgroundGoalTurn(ctx context.Context, p model.Persona, chat model.ChatAssignment, st *model.GoalStatus, hist []model.Message, isGroup bool, lastText string, opening bool) (goals.Config, prompt.GoalTurn) {
	cfg := goals.Resolve(p, chat)
	now := e.clock.Now()
	if st.Goal != cfg.Text {
		*st = freshGoalStatus(cfg.Text, now)
	}
	if sw, ok := goals.ParseSayWord(cfg.Text); ok && !st.Reached && lastText != "" && goals.SaysWord(lastText, sw.Word) {
		st.Reached, st.ReachedAt, st.How, st.Evidence = true, &now, model.GoalHowSaidWord, evidenceLine("", lastText)
	}
	gp := e.planAhead(ctx, p, cfg, *st, hist, isGroup, opening)
	if gp.planned || gp.reachedNow {
		*st = gp.status
	}
	return cfg, gp.turn
}

// playgroundGoalView reports the goal side of a playground reply.
func playgroundGoalView(cfg goals.Config, turn prompt.GoalTurn, st model.GoalStatus, res genResult) *model.PlaygroundGoal {
	return &model.PlaygroundGoal{Text: cfg.Text, Style: cfg.Style, Plan: turn.Plan, Reached: st.Reached, Evidence: st.Evidence, Rewritten: res.GoalRewritten}
}

// SetGoal sets (or with "" clears) a playground session's test goal.
func (pg *playground) SetGoal(id, goal string) error {
	s, err := pg.session(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.goal = strings.TrimSpace(guard.Truncate(goal, 500))
	s.mu.Unlock()
	return nil
}

// ownVoice catches a reply written as another chat member ("Josh: haha …"):
// it is regenerated once with a reminder; if that fails too, the other
// person's prefix line is dropped (or, when nothing is left, the persona's
// fallback line is used).
func (e *Engine) ownVoice(ctx context.Context, p model.Persona, res genResult, req llm.Request) genResult {
	hist := make([]model.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == llm.RoleUser {
			if name, _, ok := strings.Cut(m.Content, ": "); ok && !strings.ContainsAny(name, "\n") && len(name) < 40 {
				hist = append(hist, model.Message{Speaker: "them", Name: name})
			}
		}
	}
	other := prompt.SpeaksAs(res.Text, p.Name, hist)
	if other == "" {
		return res
	}
	req.System += prompt.OwnVoiceReminder(p.Name, other)
	again, err := e.generateReply(ctx, p, req)
	if err == nil && !again.Fallback && prompt.SpeaksAs(again.Text, p.Name, hist) == "" {
		again.Retried = true
		return again
	}
	// Still someone else's line: keep only what follows it, if anything.
	if _, rest, ok := strings.Cut(res.Text, "\n"); ok && strings.TrimSpace(rest) != "" && prompt.SpeaksAs(rest, p.Name, hist) == "" {
		res.Text = strings.TrimSpace(rest)
		return res
	}
	res.Text = strings.TrimSpace(p.FallbackReply)
	if res.Text == "" {
		res.Text = persona.DefaultFallbackReply
	}
	res.Fallback = true
	return res
}
