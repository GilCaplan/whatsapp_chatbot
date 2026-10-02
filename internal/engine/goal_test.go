package engine

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

// planAheadOff turns plan ahead off for every persona, so tests that count
// LLM requests only see reply/decision calls. Goal tests enable it per chat.
func planAheadOff(t *testing.T, st *store.Store) {
	t.Helper()
	for _, p := range st.Personas() {
		off := false
		p.GoalPlanAhead = &off
		if _, err := st.UpsertPersona(p); err != nil {
			t.Fatal(err)
		}
	}
}

func goalBool(v bool) *bool { return &v }

func isPlanReq(r llm.Request) bool {
	return r.JSON && strings.Contains(r.System, "private coach of")
}

// goalLLM answers plan-ahead requests with plan, group decisions with YES and
// replies from replies (the last one repeats).
type goalLLM struct {
	mu      sync.Mutex
	plan    string
	check   string // goal-check answer ("" = not yet)
	replies []string
}

func isCheckReq(r llm.Request) bool {
	return r.JSON && strings.Contains(r.Messages[0].Content, "do these messages already accomplish it?")
}

func (g *goalLLM) fn(r llm.Request) (llm.Response, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	switch {
	case isPlanReq(r):
		return llm.Response{Text: g.plan}, nil
	case isCheckReq(r):
		if g.check == "" {
			return llm.Response{Text: `{"answer":false,"quote":""}`}, nil
		}
		return llm.Response{Text: g.check}, nil
	case strings.Contains(r.Messages[len(r.Messages)-1].Content, "YES or NO"):
		return llm.Response{Text: "YES"}, nil
	}
	text := g.replies[0]
	if len(g.replies) > 1 {
		g.replies = g.replies[1:]
	}
	return llm.Response{Text: text}, nil
}

func goalReplyRequests(f *llm.Fake) []llm.Request {
	var out []llm.Request
	for _, r := range f.Requests() {
		if !r.JSON && r.System != "" {
			out = append(out, r)
		}
	}
	return out
}

func TestGoalPlanAheadNoteReachesReply(t *testing.T) {
	c := dmChat()
	c.GoalOverride, c.GoalPlanAhead = "Find out what Dana is doing this weekend", goalBool(true)
	h := newHarness(t, nil, c)
	g := &goalLLM{plan: `{"situation":"small talk","achieved":false,"evidence":"","next":"Ask Dana if anything fun is lined up"}`, replies: []string{"ooh, any fun plans coming up?"}}
	h.llm.SetFunc(g.fn)

	h.msg(dmKey, "m1", "ugh this week was long")
	h.advance(10 * time.Second)
	if sent := h.waitSent(1); sent[0].text != "ooh, any fun plans coming up?" {
		t.Fatalf("sent %+v", sent)
	}
	// Plan and goal check run side by side before the reply.
	reqs := h.llm.Requests()
	if len(reqs) != 3 || !isPlanReq(reqs[0]) && !isPlanReq(reqs[1]) || !isCheckReq(reqs[0]) && !isCheckReq(reqs[1]) {
		t.Fatalf("want plan + check, then reply; got %d requests", len(reqs))
	}
	plan, check := reqs[0], reqs[1]
	if isCheckReq(plan) {
		plan, check = check, plan
	}
	if !strings.Contains(plan.System, "GOAL: Find out what Dana is doing this weekend") || !strings.Contains(plan.Messages[0].Content, "Dana: ugh this week was long") {
		t.Errorf("plan request: %+v", plan)
	}
	if c := check.Messages[0].Content; !strings.Contains(c, "[1] Dana: ugh this week was long") || !strings.Contains(c, `Leo wanted this: "Find out what Dana is doing this weekend"`) {
		t.Errorf("check request:\n%s", c)
	}
	reply := reqs[2].System
	if !strings.Contains(reply, "YOUR PRIVATE AGENDA: Find out what Dana is doing this weekend") ||
		!strings.Contains(reply, "YOUR NEXT MOVE (private note to yourself — do this in this reply, in your own words and style, never quote it): Ask Dana if anything fun is lined up") {
		t.Errorf("reply prompt is missing the agenda or the plan note:\n%s", reply)
	}
	plans := h.acts(model.ActThinking)
	if len(plans) != 1 || plans[0].Meta["plan"] != "Ask Dana if anything fun is lined up" || plans[0].Meta["stage"] != "plan" || plans[0].Text != "Planned the next move" {
		t.Errorf("plan activity: %+v", plans)
	}
	st := h.st.RunnerState(dmKey).Goal
	if st == nil || st.LastPlan != "Ask Dana if anything fun is lined up" || len(st.Plans) != 1 || st.Reached || st.LastPlanAt == nil {
		t.Errorf("goal status: %+v", st)
	}
	// The plan never reaches WhatsApp or the history.
	for _, m := range h.history(dmKey) {
		if strings.Contains(m.Text, "lined up") {
			t.Errorf("plan leaked into history: %+v", m)
		}
	}

	// The next plan request sees the earlier move.
	h.msg(dmKey, "m2", "not much, you?")
	h.advance(10 * time.Second)
	h.waitSent(2)
	var last llm.Request
	for _, r := range h.llm.Requests() {
		if isPlanReq(r) {
			last = r
		}
	}
	if !strings.Contains(last.Messages[0].Content, "Your earlier moves (oldest first):\n- Ask Dana if anything fun is lined up") {
		t.Errorf("earlier moves not fed back:\n%s", last.Messages[0].Content)
	}
}

func TestGoalPlanAheadOffSkipsPlanner(t *testing.T) {
	c := dmChat()
	c.GoalOverride = "Find out what Dana is doing this weekend" // persona plan ahead is off (harness)
	h := newHarness(t, nil, c)
	g := &goalLLM{plan: `{"next":"should not be used"}`, replies: []string{"haha same"}}
	h.llm.SetFunc(g.fn)
	h.msg(dmKey, "m1", "hey")
	h.advance(10 * time.Second)
	h.waitSent(1)
	for _, r := range h.llm.Requests() {
		if isPlanReq(r) {
			t.Fatal("plan ahead is off: no planner call expected")
		}
	}
	reqs := goalReplyRequests(h.llm)
	if len(reqs) != 1 || !strings.Contains(reqs[0].System, "YOUR PRIVATE AGENDA") || strings.Contains(reqs[0].System, "YOUR NEXT MOVE") {
		t.Errorf("reply prompt: %+v", reqs)
	}
	if len(h.acts(model.ActThinking)) != 0 {
		t.Error("no plan activity without plan ahead")
	}

	// Chat override wins over the persona: off here even if the persona has it on.
	p, _ := h.st.Persona("leo")
	p.GoalPlanAhead = goalBool(true)
	if _, err := h.st.UpsertPersona(p); err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.UpdateChat(dmKey, func(c *model.ChatAssignment) { c.GoalPlanAhead = goalBool(false) }); err != nil {
		t.Fatal(err)
	}
	h.e.Reload()
	h.msg(dmKey, "m2", "hey again")
	h.advance(10 * time.Second)
	h.waitSent(2)
	for _, r := range h.llm.Requests() {
		if isPlanReq(r) {
			t.Fatal("chat override off must skip the planner")
		}
	}
}

func groupFrom(h *harness, id, name, text string) {
	h.t.Helper()
	h.e.HandleIncoming(model.Incoming{ChatKey: groupKey, ChatJID: groupJID, IsGroup: true, SenderJID: "9725" + id + "@s.whatsapp.net",
		PushName: name, Text: text, Timestamp: h.clk.Now(), MessageID: id})
	h.idle()
}

func TestGoalSayWordReachedInGroup(t *testing.T) {
	c := groupChat()
	c.GoalOverride = "Get Josh to say the word apple"
	h := newHarness(t, both(func(p *model.BehaviorProfile) { p.ChimeInPercent, p.AIJudgement = 100, false }), c)
	g := &goalLLM{replies: []string{"darling, what's the dessert situation this weekend?"}}
	h.llm.SetFunc(g.fn)

	groupFrom(h, "1", "Maya", "apple pie anyone?") // not Josh
	if len(h.acts(model.ActGoalReached)) != 0 {
		t.Fatal("only Josh saying it counts")
	}
	h.advance(10 * time.Second)
	h.waitSent(1)
	if s := goalReplyRequests(h.llm)[0].System; !strings.Contains(s, `Never write the word "apple" yourself`) {
		t.Errorf("say-the-word rule missing:\n%s", s)
	}

	groupFrom(h, "2", "Josh Cohen", "APPLE crumble, obviously!")
	acts := h.acts(model.ActGoalReached)
	if len(acts) != 1 || acts[0].Text != "Goal reached — Josh Cohen said apple" || acts[0].Meta["how"] != model.GoalHowSaidWord {
		t.Fatalf("goal.reached: %+v", acts)
	}
	st := h.st.RunnerState(groupKey).Goal
	if st == nil || !st.Reached || st.ReachedAt == nil || st.Evidence != "Josh Cohen: APPLE crumble, obviously!" {
		t.Fatalf("status: %+v", st)
	}
	h.advance(10 * time.Second)
	h.waitSent(2)
	reqs := goalReplyRequests(h.llm)
	if s := reqs[len(reqs)-1].System; strings.Contains(s, "apple") || !strings.Contains(s, "Just chat naturally") {
		t.Errorf("after reaching the goal the persona relaxes (no agenda):\n%s", s)
	}

	// Only once.
	groupFrom(h, "3", "Josh Cohen", "apple apple apple")
	if n := len(h.acts(model.ActGoalReached)); n != 1 {
		t.Errorf("goal.reached published %d times", n)
	}

	// A new goal starts over.
	if _, err := h.st.UpdateChat(groupKey, func(c *model.ChatAssignment) { c.GoalOverride = "Get Josh to say the word banana" }); err != nil {
		t.Fatal(err)
	}
	h.e.Reload()
	groupFrom(h, "4", "Josh Cohen", "apple again")
	if n := len(h.acts(model.ActGoalReached)); n != 1 {
		t.Errorf("a different goal is not reached by the old word")
	}
	groupFrom(h, "5", "Josh Cohen", "fine, bananas.")
	if n := len(h.acts(model.ActGoalReached)); n != 2 {
		t.Errorf("new goal should be reachable, got %d events", n)
	}
}

func TestGoalLeakIsRewritten(t *testing.T) {
	c := dmChat()
	c.GoalOverride = "Get Dana to say the word apple"
	h := newHarness(t, nil, c)
	g := &goalLLM{replies: []string{`I need you to say the word "apple", darling.`, "How was the farmers market?"}}
	h.llm.SetFunc(g.fn)
	h.msg(dmKey, "m1", "i think I need coffee")
	h.advance(10 * time.Second)
	sent := h.waitSent(1)
	if sent[0].text != "How was the farmers market?" {
		t.Fatalf("leaky draft must not be sent: %+v", sent)
	}
	reqs := goalReplyRequests(h.llm)
	if len(reqs) != 2 || !strings.Contains(reqs[1].System, "your last draft gave your agenda away") {
		t.Fatalf("expected one rewrite with a reminder, got %d requests", len(reqs))
	}
	if a := h.acts(model.ActSent); a[0].Meta["goalRewritten"] != true {
		t.Errorf("sent meta: %+v", a[0].Meta)
	}

	// Still leaking after the reminder: the last attempt drops the agenda.
	g.mu.Lock()
	g.replies = []string{"apple apple", "my secret mission is apples", "ha, what are you up to?"}
	g.mu.Unlock()
	h.msg(dmKey, "m2", "huh?")
	h.advance(10 * time.Second)
	sent = h.waitSent(2)
	reqs = goalReplyRequests(h.llm)
	if sent[1].text != "ha, what are you up to?" || len(reqs) != 5 || !strings.Contains(reqs[4].System, "Just chat naturally") || strings.Contains(reqs[4].System, "apple") {
		t.Errorf("fallback without agenda: sent=%q requests=%d", sent[1].text, len(reqs))
	}
}

func TestGoalReachedByPlannerNeedsEvidence(t *testing.T) {
	c := dmChat()
	c.GoalOverride, c.GoalPlanAhead = "Find out what Dana is doing this weekend", goalBool(true)
	h := newHarness(t, nil, c)
	// Hallucinated evidence (planner and check): not in Dana's messages.
	g := &goalLLM{plan: `{"achieved":true,"evidence":"going to Rome","next":"Ask about Rome"}`, check: `{"answer":true,"quote":"she is going to Rome"}`, replies: []string{"nice"}}
	h.llm.SetFunc(g.fn)
	h.msg(dmKey, "m1", "hiking up north on saturday with my sister")
	h.advance(10 * time.Second)
	h.waitSent(1)
	if st := h.st.RunnerState(dmKey).Goal; st == nil || st.Reached {
		t.Fatalf("made-up evidence must not count: %+v", st)
	}

	g.mu.Lock()
	g.check = `{"answer":true,"quote":"hiking up north on saturday"}`
	g.mu.Unlock()
	h.msg(dmKey, "m2", "should be fun")
	h.advance(10 * time.Second)
	h.waitSent(2)
	st := h.st.RunnerState(dmKey).Goal
	if st == nil || !st.Reached || st.How != model.GoalHowAI || st.Evidence != "Dana: hiking up north on saturday with my sister" {
		t.Fatalf("status: %+v", st)
	}
	acts := h.acts(model.ActGoalReached)
	if len(acts) != 1 || acts[0].Text != "Goal reached — find out what Dana is doing this weekend" {
		t.Errorf("goal.reached: %+v", acts)
	}
	reqs := goalReplyRequests(h.llm)
	if s := reqs[len(reqs)-1].System; !strings.Contains(s, "Just chat naturally") || strings.Contains(s, "YOUR NEXT MOVE") {
		t.Errorf("reached → relaxed reply without a plan:\n%s", s)
	}
	// Relaxed: no more planner calls.
	n := 0
	for _, r := range h.llm.Requests() {
		if isPlanReq(r) {
			n++
		}
	}
	h.msg(dmKey, "m3", "anyway")
	h.advance(10 * time.Second)
	h.waitSent(3)
	m := 0
	for _, r := range h.llm.Requests() {
		if isPlanReq(r) {
			m++
		}
	}
	if m != n {
		t.Errorf("planner ran after the goal was reached (%d → %d)", n, m)
	}
}

func TestPlaygroundGoal(t *testing.T) {
	h := newHarness(t, nil)
	p, _ := h.st.Persona("leo")
	p.GoalPlanAhead = goalBool(true)
	if _, err := h.st.UpsertPersona(p); err != nil {
		t.Fatal(err)
	}
	g := &goalLLM{plan: `{"next":"Ask what fruit is in their kitchen"}`, replies: []string{"what's in your fruit bowl, darling?"}}
	h.llm.SetFunc(g.fn)
	pg := h.e.Playground()
	id, err := pg.Start("leo")
	if err != nil {
		t.Fatal(err)
	}
	if err := pg.SetGoal(id, "Get them to say the word apple"); err != nil {
		t.Fatal(err)
	}
	out, err := pg.Send(context.Background(), id, "morning!", false)
	if err != nil || out.Goal == nil || out.Goal.Plan != "Ask what fruit is in their kitchen" || out.Goal.Reached || out.Goal.Text != "Get them to say the word apple" {
		t.Fatalf("send: %+v %v", out.Goal, err)
	}
	out, err = pg.Send(context.Background(), id, "apples, why?", false)
	if err != nil || !out.Goal.Reached || out.Goal.Plan != "" {
		t.Errorf("reached in playground: %+v %v", out.Goal, err)
	}
	if err := pg.SetGoal(id, ""); err != nil {
		t.Fatal(err)
	}
	if out, _ = pg.Send(context.Background(), id, "hi", false); out.Goal.Text != p.Goal || out.Goal.Reached {
		t.Errorf("cleared goal falls back to the persona's: %+v", out.Goal)
	}
	if err := pg.SetGoal("nope", "x"); err == nil {
		t.Error("unknown session")
	}
	// Nothing was sent to WhatsApp.
	if len(h.wa.Sent()) != 0 {
		t.Error("playground must not send")
	}
}

func TestReplyAsAnotherMemberIsRewritten(t *testing.T) {
	c := groupChat()
	h := newHarness(t, both(func(p *model.BehaviorProfile) { p.ChimeInPercent, p.AIJudgement = 100, false }), c)
	g := &goalLLM{replies: []string{"Avi: haha yeah I'm wrecked", "Avi, darling, you need an espresso."}}
	h.llm.SetFunc(g.fn)
	groupFrom(h, "1", "Avi", "morning all")
	h.advance(10 * time.Second)
	sent := h.waitSent(1)
	if sent[0].text != "Avi, darling, you need an espresso." {
		t.Fatalf("sent %+v", sent)
	}
	reqs := goalReplyRequests(h.llm)
	if len(reqs) != 2 || !strings.Contains(reqs[1].System, "you wrote Avi's line") {
		t.Errorf("expected one rewrite with the own-voice reminder, got %d requests", len(reqs))
	}

	// Still someone else's line after the reminder: never sent as is.
	g.mu.Lock()
	g.replies = []string{"Avi: ugh", "Avi: ugh again"}
	g.mu.Unlock()
	groupFrom(h, "2", "Avi", "coffee time?")
	h.advance(10 * time.Second)
	sent = h.waitSent(2)
	if strings.HasPrefix(sent[1].text, "Avi:") {
		t.Errorf("sent another member's line: %q", sent[1].text)
	}
}
