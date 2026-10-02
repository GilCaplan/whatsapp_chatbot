package server

import (
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

func TestMissionsStartAbandon(t *testing.T) {
	e := newEnv(t)
	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo", "name": "Dana"}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	key := strings.ReplaceAll(c.Key, ":", "%3A")

	var mv model.MissionsView
	e.do("GET", "/api/missions", nil, &mv)
	if len(mv.Templates) < 12 || len(mv.Achievements) < 8 || len(mv.History) != 0 {
		t.Fatalf("view = %d templates %d achievements %d history", len(mv.Templates), len(mv.Achievements), len(mv.History))
	}

	// Errors.
	var apiErr apiError
	for _, tc := range []struct {
		body any
		code int
		err  string
	}{
		{map[string]any{"chatKey": "dm:nope", "templateId": "say-word"}, 404, "not_found"},
		{map[string]any{"chatKey": c.Key, "templateId": "fly"}, 400, "unknown_mission"},
		{map[string]any{"chatKey": c.Key, "templateId": "say-word", "blanks": map[string]string{"name": "Dana"}}, 400, "invalid_blanks"},
		{map[string]any{"chatKey": c.Key}, 400, "invalid_goal"},
		{map[string]any{"chatKey": c.Key, "goal": strings.Repeat("x", 501)}, 400, "invalid_goal"},
	} {
		if code := e.do("POST", "/api/missions/start", tc.body, &apiErr); code != tc.code || apiErr.Code != tc.err {
			t.Errorf("start %v = %d %+v", tc.body, code, apiErr)
		}
	}

	// A goal reached earlier is started over.
	if _, err := e.st.UpdateRunnerState(c.Key, func(st *store.RunnerState) {
		now := time.Now()
		st.Goal = &model.GoalStatus{Goal: `Get Dana to say the word "pineapple"`, Reached: true, ReachedAt: &now}
	}); err != nil {
		t.Fatal(err)
	}
	if code := e.do("POST", "/api/missions/start", map[string]any{"chatKey": c.Key, "templateId": "say-word",
		"blanks": map[string]string{"name": "Dana", "word": "pineapple"}}, &c); code != 200 {
		t.Fatalf("start = %d", code)
	}
	if c.GoalOverride != `Get Dana to say the word "pineapple"` || c.MissionID != "say-word" {
		t.Fatalf("chat = %q %q", c.GoalOverride, c.MissionID)
	}
	if st := e.st.RunnerState(c.Key); st.Goal != nil {
		t.Errorf("goal status not reset: %+v", st.Goal)
	}
	e.do("GET", "/api/missions", nil, &mv)
	if len(mv.Active) != 1 || mv.Active[0].MissionID != "say-word" || mv.Active[0].Goal.State != "working" || mv.Active[0].ChatName != "Dana" {
		t.Fatalf("active = %+v", mv.Active)
	}
	if len(mv.History) != 1 || mv.History[0].ReachedAt != nil || mv.History[0].PersonaName == "" || mv.History[0].Style == "" {
		t.Fatalf("history = %+v", mv.History)
	}

	// Editing the goal by hand drops the mission id.
	e.do("PATCH", "/api/chats/"+key, map[string]any{"goalOverride": "Get Dana to say the word mango"}, &c)
	if c.MissionID != "" {
		t.Errorf("missionId kept after a hand edit: %q", c.MissionID)
	}

	// A free-form mission, then reached → completed record + achievement.
	e.do("POST", "/api/missions/start", map[string]any{"chatKey": c.Key, "goal": "  Make   Dana laugh "}, &c)
	if c.GoalOverride != "Make Dana laugh" || c.MissionID != "" {
		t.Fatalf("free-form = %q %q", c.GoalOverride, c.MissionID)
	}
	now := time.Now()
	if _, ok, err := e.st.CompleteMission(c.Key, c.GoalOverride, model.GoalStatus{Goal: c.GoalOverride, Reached: true, ReachedAt: &now, How: model.GoalHowAI, Evidence: "Dana: hahaha"}); !ok || err != nil {
		t.Fatalf("complete = %v %v", ok, err)
	}
	e.do("GET", "/api/missions", nil, &mv)
	if len(mv.History) != 1 || mv.History[0].ReachedAt == nil || mv.History[0].Evidence != "Dana: hahaha" {
		t.Fatalf("history = %+v", mv.History)
	}
	unlocked := 0
	for _, a := range mv.Achievements {
		if a.Unlocked {
			unlocked++
			if a.ID != "first-mission" && a.ID != "smooth-operator" {
				t.Errorf("unexpected badge %s", a.ID)
			}
		}
	}
	if unlocked == 0 {
		t.Error("no achievement unlocked")
	}

	// Abandon clears the goal and the open record.
	e.do("POST", "/api/missions/start", map[string]any{"chatKey": c.Key, "templateId": "get-photo", "blanks": map[string]string{"name": "Dana"}}, &c)
	if code := e.do("POST", "/api/missions/"+key+"/abandon", nil, &c); code != 200 || c.GoalOverride != "" || c.MissionID != "" {
		t.Fatalf("abandon = %d %+v", code, c)
	}
	e.do("GET", "/api/missions", nil, &mv)
	// (the chat may still pursue the persona's own goal, which is no mission)
	if (len(mv.Active) > 0 && (mv.Active[0].MissionID != "" || mv.Active[0].Goal.Source == "chat")) || len(mv.History) != 1 {
		t.Errorf("after abandon: active %d history %d", len(mv.Active), len(mv.History))
	}
	if code := e.do("POST", "/api/missions/dm%3Anope/abandon", nil, nil); code != 404 {
		t.Errorf("abandon unknown = %d", code)
	}
}
