package server

import (
	"strings"
	"testing"

	"whatsappdoppel/internal/model"
)

// Phase 0 contract tests for wave 3: chat mode / memory / missionId, co-pilot
// draft selection, v4 settings validation and the frozen endpoints (real
// shapes for reads, 501 not_implemented for features that haven't landed).

func TestChatModeMemoryMissionPatch(t *testing.T) {
	e := newEnv(t)
	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo", "approvalMode": true}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	if c.Mode != model.ChatModeApprove || !c.ApprovalMode || c.Memory != nil {
		t.Fatalf("created = %q %v %v", c.Mode, c.ApprovalMode, c.Memory)
	}
	key := strings.ReplaceAll(c.Key, ":", "%3A")

	e.do("PATCH", "/api/chats/"+key, map[string]any{"mode": "copilot", "memory": false, "missionId": " say-word "}, &c)
	if c.Mode != model.ChatModeCopilot || !c.ApprovalMode || c.Memory == nil || *c.Memory || c.MissionID != "say-word" {
		t.Fatalf("patched = %q %v %v %q", c.Mode, c.ApprovalMode, c.Memory, c.MissionID)
	}
	// approvalMode:true keeps co-pilot; false = auto. memory:null = app setting.
	e.do("PATCH", "/api/chats/"+key, map[string]any{"approvalMode": true}, &c)
	if c.Mode != model.ChatModeCopilot {
		t.Fatalf("approvalMode true changed copilot to %q", c.Mode)
	}
	e.do("PATCH", "/api/chats/"+key, map[string]any{"approvalMode": false, "memory": nil}, &c)
	if c.Mode != model.ChatModeAuto || c.ApprovalMode || c.Memory != nil {
		t.Fatalf("after approvalMode false = %q %v %v", c.Mode, c.ApprovalMode, c.Memory)
	}
	var apiErr apiError
	for _, body := range []map[string]any{{"mode": "yolo"}, {"memory": "yes"}} {
		if code := e.do("PATCH", "/api/chats/"+key, body, &apiErr); code != 400 {
			t.Errorf("PATCH %v = %d %+v", body, code, apiErr)
		}
	}

	var list []chatView
	e.do("GET", "/api/chats", nil, &list)
	if len(list) != 1 || list[0].Mode != model.ChatModeAuto {
		t.Fatalf("list = %+v", list)
	}
}

func TestApproveDraftParam(t *testing.T) {
	e := newEnv(t)
	if err := e.st.UpsertApproval(model.PendingReply{ID: "p1", ChatKey: "dm:15550001", Text: "a",
		Drafts: []model.Draft{{Tone: "brief", Text: "a"}, {Tone: "warm", Text: "b"}}}); err != nil {
		t.Fatal(err)
	}
	if code := e.do("POST", "/api/approvals/p1/approve", map[string]any{"draft": 1}, nil); code != 200 {
		t.Fatalf("approve = %d", code)
	}
	if code := e.do("POST", "/api/approvals/p1/approve", map[string]any{"text": "edited"}, nil); code != 200 {
		t.Fatalf("approve = %d", code)
	}
	var apiErr apiError
	if code := e.do("POST", "/api/approvals/p1/approve", map[string]any{"draft": -2}, &apiErr); code != 400 || apiErr.Code != "invalid_draft" {
		t.Errorf("negative draft = %d %+v", code, apiErr)
	}
	e.eng.mu.Lock()
	got := strings.Join(e.eng.approved, ",")
	e.eng.mu.Unlock()
	if got != "p1||1,p1|edited|-1" {
		t.Errorf("approved = %s", got)
	}
}

func TestSettingsV4(t *testing.T) {
	e := newEnv(t)
	var sv settingsView
	if code := e.do("GET", "/api/settings", nil, &sv); code != 200 || sv.Version != 5 || !sv.Notifications.Enabled || !sv.Safety.Handoff.Bot || sv.Recap.Time != "21:00" || sv.SelfClone.MaxSamples != 500 {
		t.Fatalf("settings = %d %+v", code, sv.Settings)
	}
	if pr := sv.Behavior.Private; pr.TypoPercent != 5 || pr.TypoFixStyle != "correction" {
		t.Errorf("typo defaults = %d %q", pr.TypoPercent, pr.TypoFixStyle)
	}
	if code := e.do("PUT", "/api/settings", map[string]any{"notifications": map[string]any{"sound": false}, "recap": map[string]any{"time": "07:45"}}, &sv); code != 200 ||
		sv.Notifications.Sound || !sv.Notifications.Enabled || sv.Recap.Time != "07:45" {
		t.Fatalf("put = %d %+v %+v", code, sv.Notifications, sv.Recap)
	}
	var apiErr apiError
	for _, body := range []map[string]any{
		{"recap": map[string]any{"time": "25:00"}},
		{"recap": map[string]any{"keepDays": 0}},
		{"safety": map[string]any{"reveal": map[string]any{"template": strings.Repeat("x", 601)}}},
		{"behavior": map[string]any{"private": map[string]any{"typoPercent": 31}}},
		{"behavior": map[string]any{"group": map[string]any{"typoFixStyle": "shout"}}},
	} {
		if code := e.do("PUT", "/api/settings", body, &apiErr); code != 400 || apiErr.Code != "invalid_settings" {
			t.Errorf("PUT %v = %d %+v", body, code, apiErr)
		}
	}
}

func TestWave3Endpoints(t *testing.T) {
	e := newEnv(t)
	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo"}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	key := strings.ReplaceAll(c.Key, ":", "%3A")

	// Reads return their real (empty) shapes.
	var mem model.MemoriesView
	if code := e.do("GET", "/api/chats/"+key+"/memories", nil, &mem); code != 200 || !mem.Enabled || mem.Items == nil {
		t.Errorf("memories = %d %+v", code, mem)
	}
	var rv model.RecapsView
	if code := e.do("GET", "/api/recaps?chat="+key, nil, &rv); code != 200 || rv.Items == nil {
		t.Errorf("recaps = %d %+v", code, rv)
	}
	var mv model.MissionsView
	if code := e.do("GET", "/api/missions", nil, &mv); code != 200 || mv.Templates == nil || mv.Active == nil || mv.History == nil || mv.Achievements == nil {
		t.Errorf("missions = %d %+v", code, mv)
	}
	var cs model.CloneSamplesView
	if code := e.do("GET", "/api/clone/samples", nil, &cs); code != 200 || cs.Enabled || cs.Preview == nil {
		t.Errorf("clone samples = %d %+v", code, cs)
	}
	for _, rq := range []struct{ method, path string }{
		{"DELETE", "/api/chats/" + key + "/memories"},
		{"DELETE", "/api/clone/samples"},
	} {
		if code := e.do(rq.method, rq.path, nil, nil); code != 200 {
			t.Errorf("%s %s = %d", rq.method, rq.path, code)
		}
	}

	// Features that haven't landed answer 501 not_implemented. (Memory and
	// Clone yourself landed: realism_test.go; handoff/reveal/recaps:
	// wave3b_test.go.)
	for _, rq := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/system/notify-test", nil}, // no Notifier wired in this env
	} {
		var apiErr apiError
		if code := e.do(rq.method, rq.path, rq.body, &apiErr); code != 501 || apiErr.Code != "not_implemented" {
			t.Errorf("%s %s = %d %+v", rq.method, rq.path, code, apiErr)
		}
	}
	// Unknown chats are still 404.
	if code := e.do("POST", "/api/chats/dm%3Anope/reveal", map[string]any{}, nil); code != 404 {
		t.Errorf("reveal unknown = %d", code)
	}
}
