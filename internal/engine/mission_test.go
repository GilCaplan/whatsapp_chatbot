package engine

import (
	"testing"
	"time"

	"whatsappdoppel/internal/mission"
	"whatsappdoppel/internal/model"
)

func TestMissionRecordedOnGoalReached(t *testing.T) {
	c := groupChat()
	tpl, _ := mission.Template("say-word")
	goal, _ := mission.Fill(tpl, map[string]string{"name": "Josh", "word": "apple"})
	c.GoalOverride, c.MissionID = goal, "say-word"
	h := newHarness(t, both(func(p *model.BehaviorProfile) { p.ChimeInPercent, p.AIJudgement = 100, false }), c)
	h.llm.SetFunc((&goalLLM{replies: []string{"so what's for dessert?"}}).fn)
	if _, err := h.st.StartMission(model.MissionRecord{ChatKey: groupKey, Goal: goal, TemplateID: "say-word", StartedAt: h.clk.Now()}); err != nil {
		t.Fatal(err)
	}

	groupFrom(h, "1", "Josh", "apple pie, obviously")
	acts := h.acts(model.ActGoalReached)
	if len(acts) != 1 || acts[0].Meta["missionId"] != "say-word" {
		t.Fatalf("goal.reached = %+v", acts)
	}
	ms := h.st.Missions()
	if len(ms) != 1 || ms[0].ReachedAt == nil || ms[0].How != model.GoalHowSaidWord || ms[0].Evidence != "Josh: apple pie, obviously" ||
		ms[0].TemplateID != "say-word" || ms[0].ChatName != "Friends" || ms[0].PersonaID != "leo" {
		t.Fatalf("missions = %+v", ms)
	}
	h.advance(10 * time.Second)
}

func TestPhotoMissionReached(t *testing.T) {
	c := dmChat()
	tpl, _ := mission.Template("get-photo")
	goal, _ := mission.Fill(tpl, map[string]string{"name": "Dana"})
	c.GoalOverride, c.MissionID = goal, "get-photo"
	h := newHarness(t, nil, c)
	h.llm.SetFunc((&goalLLM{replies: []string{"cute!"}}).fn)

	// Text alone doesn't count; a photo does — once.
	h.e.HandleIncoming(model.Incoming{ChatKey: dmKey, ChatJID: dmJID, SenderJID: dmJID, PushName: "Dana", Text: "I'll send one later", Timestamp: h.clk.Now(), MessageID: "m1"})
	h.idle()
	if n := len(h.acts(model.ActGoalReached)); n != 0 {
		t.Fatalf("reached by text: %d", n)
	}
	for _, id := range []string{"m2", "m3"} {
		h.e.HandleIncoming(model.Incoming{ChatKey: dmKey, ChatJID: dmJID, SenderJID: dmJID, PushName: "Dana", Text: "[photo]", Media: model.MediaImage, Timestamp: h.clk.Now(), MessageID: id})
		h.idle()
	}
	acts := h.acts(model.ActGoalReached)
	if len(acts) != 1 || acts[0].Text != "Goal reached — Dana sent a photo" || acts[0].Meta["how"] != mission.HowMedia {
		t.Fatalf("goal.reached = %+v", acts)
	}
	if st := h.st.RunnerState(dmKey).Goal; st == nil || !st.Reached || st.How != mission.HowMedia {
		t.Fatalf("status = %+v", st)
	}
	// No open record existed: the completion is recorded anyway.
	if ms := h.st.Missions(); len(ms) != 1 || ms[0].ReachedAt == nil || ms[0].TemplateID != "get-photo" {
		t.Fatalf("missions = %+v", ms)
	}
	// Another chat's photo mission isn't triggered by a free-form goal.
	h.advance(30 * time.Second)
}
