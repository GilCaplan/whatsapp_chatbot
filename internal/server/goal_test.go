package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

// fakePlayground records the goal set before each message.
type fakePlayground struct {
	goals []string
}

func (f *fakePlayground) Start(personaID string) (string, error) { return "s1", nil }
func (f *fakePlayground) Send(ctx context.Context, id, text string, group bool) (model.PlaygroundReply, error) {
	g := ""
	if n := len(f.goals); n > 0 {
		g = f.goals[n-1]
	}
	return model.PlaygroundReply{Reply: "ok", Goal: &model.PlaygroundGoal{Text: g, Plan: "Ask about pie"}}, nil
}
func (f *fakePlayground) SetGoal(id, goal string) error {
	f.goals = append(f.goals, goal)
	return nil
}
func (f *fakePlayground) Initiate(ctx context.Context, id string, group bool, hint string) (model.PlaygroundReply, error) {
	return model.PlaygroundReply{Reply: "opener:" + hint}, nil
}
func (f *fakePlayground) End(string) {}

func TestChatGoalSettingsAndStatus(t *testing.T) {
	e := newEnv(t)
	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo"}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	key := strings.ReplaceAll(c.Key, ":", "%3A")

	var list []chatView
	e.do("GET", "/api/chats", nil, &list)
	g := list[0].Goal
	if g.State != "working" || g.Style != model.GoalStyleSubtle || g.StyleSource != "persona" || !g.PlanAhead || g.PlanAheadSource != "persona" || g.Source == "" || g.Text == "" {
		t.Fatalf("default goal view = %+v", g)
	}

	if code := e.do("PATCH", "/api/chats/"+key, map[string]any{"goalOverride": "Get Dana to say apple", "goalStyle": "Balanced", "goalPlanAhead": false}, &c); code != 200 {
		t.Fatalf("patch = %d", code)
	}
	if c.GoalStyle == nil || *c.GoalStyle != "balanced" || c.GoalPlanAhead == nil || *c.GoalPlanAhead {
		t.Fatalf("overrides not saved: %+v", c)
	}
	e.do("GET", "/api/chats", nil, &list)
	if g = list[0].Goal; g.Text != "Get Dana to say apple" || g.Source != "chat" || g.Style != "balanced" || g.StyleSource != "chat" || g.PlanAhead || g.PlanAheadSource != "chat" {
		t.Fatalf("overridden goal view = %+v", g)
	}

	// Other PATCH fields leave the overrides alone; null goes back to the persona's.
	e.do("PATCH", "/api/chats/"+key, map[string]any{"enabled": true}, &c)
	if c.GoalStyle == nil || c.GoalPlanAhead == nil {
		t.Fatalf("absent fields must not clear overrides: %+v", c)
	}
	e.do("PATCH", "/api/chats/"+key, map[string]any{"goalStyle": nil}, &c)
	if c.GoalStyle != nil || c.GoalPlanAhead == nil {
		t.Fatalf("null clears only goalStyle: %+v", c)
	}

	var apiErr apiError
	for _, body := range []map[string]any{{"goalStyle": "sneaky"}, {"goalStyle": 3}, {"goalPlanAhead": "yes"}} {
		if code := e.do("PATCH", "/api/chats/"+key, body, &apiErr); code != 400 || apiErr.Code != "invalid_goal" {
			t.Errorf("PATCH %v = %d %+v", body, code, apiErr)
		}
	}

	// Progress comes from runtime.json; the behaviour endpoint shows it too.
	at := time.Date(2026, 10, 1, 20, 14, 0, 0, time.UTC)
	if _, err := e.st.UpdateRunnerState(c.Key, func(st *store.RunnerState) {
		st.Goal = &model.GoalStatus{Goal: "Get Dana to say apple", Reached: true, ReachedAt: &at, How: model.GoalHowSaidWord, Evidence: "Dana: apple pie", LastPlan: "Ask about dessert"}
	}); err != nil {
		t.Fatal(err)
	}
	e.do("GET", "/api/chats", nil, &list)
	if g = list[0].Goal; g.State != "reached" || g.ReachedAt == nil || !g.ReachedAt.Equal(at) || g.Evidence != "Dana: apple pie" || g.How != model.GoalHowSaidWord {
		t.Fatalf("reached view = %+v", g)
	}
	var bv chatBehaviorView
	if code := e.do("GET", "/api/chats/"+key+"/behavior", nil, &bv); code != 200 || bv.Goal.State != "reached" || bv.Goal.LastPlan != "Ask about dessert" {
		t.Fatalf("behavior goal = %d %+v", code, bv.Goal)
	}

	var reset model.ChatGoal
	if code := e.do("POST", "/api/chats/"+key+"/goal/reset", nil, &reset); code != 200 || reset.State != "working" || reset.ReachedAt != nil {
		t.Fatalf("reset = %d %+v", code, reset)
	}
	if st := e.st.RunnerState(c.Key); st.Goal != nil {
		t.Fatalf("status not cleared: %+v", st.Goal)
	}
	if code := e.do("POST", "/api/chats/dm%3Anope/goal/reset", nil, &apiErr); code != 404 {
		t.Errorf("reset unknown chat = %d", code)
	}

	// A changed goal text ignores the old status.
	_, _ = e.st.UpdateRunnerState(c.Key, func(st *store.RunnerState) {
		st.Goal = &model.GoalStatus{Goal: "an older goal", Reached: true, ReachedAt: &at}
	})
	e.do("GET", "/api/chats", nil, &list)
	if list[0].Goal.State != "working" {
		t.Errorf("status of another goal shown: %+v", list[0].Goal)
	}
}

func TestPlaygroundGoalParam(t *testing.T) {
	e := newEnv(t)
	pg := &fakePlayground{}
	e.s.d.Playground = pg
	var out model.PlaygroundReply
	if code := e.do("POST", "/api/playground/s1/messages", map[string]any{"text": "hi"}, &out); code != 200 || len(pg.goals) != 0 {
		t.Fatalf("no goal param: %d %v", code, pg.goals)
	}
	if code := e.do("POST", "/api/playground/s1/messages", map[string]any{"text": "hi", "goal": "Get them to say apple"}, &out); code != 200 {
		t.Fatalf("send = %d", code)
	}
	if len(pg.goals) != 1 || pg.goals[0] != "Get them to say apple" || out.Goal == nil || out.Goal.Plan != "Ask about pie" {
		t.Fatalf("goal = %v, reply = %+v", pg.goals, out.Goal)
	}
	e.do("POST", "/api/playground/s1/messages", map[string]any{"text": "hi", "goal": ""}, &out)
	if len(pg.goals) != 2 || pg.goals[1] != "" {
		t.Fatalf("empty goal clears: %v", pg.goals)
	}
}

func TestInitiateEndpoint(t *testing.T) {
	e := newEnv(t)
	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo", "approvalMode": true}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	key := strings.ReplaceAll(c.Key, ":", "%3A")
	var out struct {
		OK       bool `json:"ok"`
		Approval bool `json:"approval"`
	}
	if code := e.do("POST", "/api/chats/"+key+"/initiate", map[string]any{"hint": "the new bakery"}, &out); code != 200 || !out.OK || !out.Approval {
		t.Fatalf("initiate = %d %+v", code, out)
	}
	if code := e.do("POST", "/api/chats/"+key+"/initiate", nil, &out); code != 200 {
		t.Fatalf("initiate without body = %d", code)
	}
	e.eng.mu.Lock()
	got := append([]string(nil), e.eng.initiated...)
	e.eng.mu.Unlock()
	if len(got) != 2 || got[0] != c.Key+"|the new bakery" || got[1] != c.Key+"|" {
		t.Fatalf("engine calls = %v", got)
	}

	var apiErr apiError
	if code := e.do("POST", "/api/chats/"+key+"/initiate", map[string]any{"hint": strings.Repeat("x", 301)}, &apiErr); code != 400 || apiErr.Code != "invalid_hint" {
		t.Errorf("long hint = %d %+v", code, apiErr)
	}
	e.eng.mu.Lock()
	e.eng.initiateErr = errors.New("the persona is already writing in this chat — try again in a moment")
	e.eng.mu.Unlock()
	if code := e.do("POST", "/api/chats/"+key+"/initiate", nil, &apiErr); code != 409 || apiErr.Code != "busy" {
		t.Errorf("busy = %d %+v", code, apiErr)
	}
	e.do("PATCH", "/api/chats/"+key, map[string]any{"enabled": false}, &c)
	if code := e.do("POST", "/api/chats/"+key+"/initiate", nil, &apiErr); code != 409 || apiErr.Code != "chat_disabled" {
		t.Errorf("disabled = %d %+v", code, apiErr)
	}
	if code := e.do("POST", "/api/chats/dm%3Anope/initiate", nil, &apiErr); code != 404 {
		t.Errorf("unknown chat = %d", code)
	}

	// The next automatic check-in is exposed on the chat list.
	due := time.Date(2026, 10, 2, 18, 40, 0, 0, time.UTC)
	_, _ = e.st.UpdateRunnerState(c.Key, func(st *store.RunnerState) { st.ProactiveDueAt = &due })
	var list []chatView
	e.do("GET", "/api/chats", nil, &list)
	if list[0].NextCheckInAt == nil || !list[0].NextCheckInAt.Equal(due) {
		t.Errorf("nextCheckInAt = %v", list[0].NextCheckInAt)
	}
}

func TestPlaygroundInitiateOption(t *testing.T) {
	e := newEnv(t)
	e.s.d.Playground = &fakePlayground{}
	var out model.PlaygroundReply
	if code := e.do("POST", "/api/playground/s1/messages", map[string]any{"initiate": true}, &out); code != 200 || out.Reply != "opener:" {
		t.Fatalf("initiate = %d %+v", code, out)
	}
	if code := e.do("POST", "/api/playground/s1/messages", map[string]any{"initiate": true, "text": "the gym"}, &out); code != 200 || out.Reply != "opener:the gym" {
		t.Fatalf("initiate with topic = %d %+v", code, out)
	}
	var apiErr apiError
	if code := e.do("POST", "/api/playground/s1/messages", map[string]any{"text": ""}, &apiErr); code != 400 {
		t.Errorf("empty text without initiate = %d", code)
	}
}
