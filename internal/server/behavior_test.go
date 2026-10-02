package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/model"
)

func TestSettingsBehaviorMergeAndValidate(t *testing.T) {
	e := newEnv(t)
	var got settingsView
	week := []map[string]any{{"day": "mon", "ranges": []map[string]string{{"from": "09:00", "to": "17:00"}}}}
	code := e.do("PUT", "/api/settings", map[string]any{
		"behavior": map[string]any{
			"group": map[string]any{
				"replyPercent": 80,
				"availability": map[string]any{"enabled": true, "week": week},
			},
		},
	}, &got)
	if code != 200 {
		t.Fatalf("PUT = %d", code)
	}
	g := got.Behavior.Group
	if g.ReplyPercent != 80 || !g.Availability.Enabled || g.Availability.OutsideHours != "queue" || g.Availability.CatchUpMaxMin != 20 {
		t.Fatalf("nested merge: %+v", g.Availability)
	}
	// The week array is replaced wholesale (then normalised to 7 days).
	if len(g.Availability.Week) != 7 || len(g.Availability.Week[0].Ranges) != 1 || len(g.Availability.Week[1].Ranges) != 0 {
		t.Fatalf("week: %+v", g.Availability.Week)
	}
	if g.Preset != behavior.PresetCustom || got.Behavior.Private.Preset != behavior.PresetNatural {
		t.Fatalf("presets: %q %q", g.Preset, got.Behavior.Private.Preset)
	}
	reloads := e.eng.reloads.Load()

	// Applying a preset keeps its label.
	busy, _ := behavior.Preset(behavior.PresetBusy)
	if code := e.do("PUT", "/api/settings", map[string]any{"behavior": map[string]any{"private": busy.Private}}, &got); code != 200 {
		t.Fatalf("preset PUT = %d", code)
	}
	if got.Behavior.Private.Preset != behavior.PresetBusy || got.Behavior.Private.ReplyPercent != 85 {
		t.Fatalf("busy: %+v", got.Behavior.Private)
	}
	if e.eng.reloads.Load() == reloads {
		t.Error("behavior change should reload the engine")
	}

	for _, bad := range []map[string]any{
		{"behavior": map[string]any{"private": map[string]any{"replyPercent": 101}}},
		{"behavior": map[string]any{"group": map[string]any{"lengthBias": "epic"}}},
		{"behavior": map[string]any{"group": map[string]any{"availability": map[string]any{"timezone": "Mars/Base"}}}},
		{"behavior": map[string]any{"group": map[string]any{"availability": map[string]any{"week": []map[string]any{{"day": "xyz"}}}}}},
		{"behavior": map[string]any{"triggerPrefix": "way too long"}},
		{"behavior": map[string]any{"private": map[string]any{"preset": "turbo"}}},
	} {
		var apiErr apiError
		if code := e.do("PUT", "/api/settings", bad, &apiErr); code != 400 || apiErr.Code != "invalid_settings" || !strings.Contains(apiErr.Error, "behavior.") {
			t.Errorf("%v: %d %+v", bad, code, apiErr)
		}
	}
	if e.cfg.Get().Behavior.Private.Preset != behavior.PresetBusy {
		t.Error("invalid PUT must not persist")
	}
	// Legacy keys are ignored.
	if code := e.do("PUT", "/api/settings", map[string]any{"replies": map[string]any{"debounceSeconds": 1}, "approvals": map[string]any{"autoSendSeconds": 5}}, &got); code != 200 {
		t.Fatalf("legacy PUT = %d", code)
	}
	if got.Replies != nil || got.Approvals != nil || got.Behavior.Private.WaitForMoreSec != busy.Private.WaitForMoreSec {
		t.Errorf("legacy keys should be ignored: %+v", got.Behavior.Private)
	}
	// Prefix can be cleared.
	if code := e.do("PUT", "/api/settings", map[string]any{"behavior": map[string]any{"triggerPrefix": ""}}, &got); code != 200 || got.Behavior.TriggerPrefix != "" {
		t.Errorf("clear prefix: %d %q", code, got.Behavior.TriggerPrefix)
	}
}

func TestBehaviorPresetsAndSample(t *testing.T) {
	e := newEnv(t)
	var meta struct {
		Presets  []behavior.PresetDef             `json:"presets"`
		Ranges   map[string]behavior.Range        `json:"ranges"`
		Enums    map[string][]string              `json:"enums"`
		Defaults map[string]model.BehaviorProfile `json:"defaults"`
	}
	if code := e.do("GET", "/api/behavior/presets", nil, &meta); code != 200 {
		t.Fatalf("presets = %d", code)
	}
	if len(meta.Presets) != 5 || meta.Presets[0].ID != "natural" || meta.Ranges["replyPercent"].Max != 100 ||
		meta.Ranges["proactive.maxPerDay"].Max != 10 || len(meta.Enums["preset"]) != 6 || len(meta.Enums["availability.week.day"]) != 7 ||
		meta.Defaults["group"].Preset != "natural" || meta.Defaults["private"].WaitForMoreSec != 8 {
		t.Fatalf("meta = %+v", meta)
	}

	type phase struct {
		Name     string  `json:"name"`
		Sec      float64 `json:"sec"`
		StartSec float64 `json:"startSec"`
	}
	var out struct {
		Samples []struct {
			TotalSec float64 `json:"totalSec"`
			Phases   []phase `json:"phases"`
			Bubbles  []struct {
				Text      string  `json:"text"`
				TypingSec float64 `json:"typingSec"`
			} `json:"bubbles"`
			Quoted bool `json:"quoted"`
		} `json:"samples"`
		Summary struct {
			MinTotalSec    float64 `json:"minTotalSec"`
			MedianTotalSec float64 `json:"medianTotalSec"`
			MaxTotalSec    float64 `json:"maxTotalSec"`
			SplitShare     float64 `json:"splitShare"`
		} `json:"summary"`
	}
	inst, _ := behavior.Preset(behavior.PresetInstant)
	p := inst.Private
	p.NoticeMinSec, p.NoticeMaxSec = 10, 10
	p.SplitPercent = 100
	p.ThinkMinSec, p.ThinkMaxSec = 2, 2
	if code := e.do("POST", "/api/behavior/sample", map[string]any{"kind": "private", "profile": p, "samples": 3}, &out); code != 200 {
		t.Fatalf("sample = %d", code)
	}
	if len(out.Samples) != 3 {
		t.Fatalf("samples = %d", len(out.Samples))
	}
	s0 := out.Samples[0]
	names := []string{}
	for _, ph := range s0.Phases {
		names = append(names, ph.Name)
	}
	joined := strings.Join(names, ",")
	if !strings.HasPrefix(joined, "notice,seen,wait,think,typing,send,gap,typing,send") || s0.Quoted || len(s0.Bubbles) < 2 {
		t.Errorf("phases = %s bubbles=%d", joined, len(s0.Bubbles))
	}
	if s0.Phases[0].Sec != 10 || s0.Phases[2].StartSec != 10 || s0.Phases[2].Sec != 2 || s0.Phases[3].Sec != 2 {
		t.Errorf("timings: %+v", s0.Phases)
	}
	last := s0.Phases[len(s0.Phases)-1]
	if last.StartSec != s0.TotalSec || out.Summary.SplitShare != 1 || out.Summary.MinTotalSec > out.Summary.MaxTotalSec {
		t.Errorf("total/summary: %+v %+v", s0.TotalSec, out.Summary)
	}

	// Garbage values are clamped, partial profiles default; group quoting.
	if code := e.do("POST", "/api/behavior/sample", map[string]any{"kind": "group", "samples": 50,
		"profile": map[string]any{"replyPercent": 900, "quoteReplyPercent": 100, "thinkMinSec": -5}}, &out); code != 200 {
		t.Fatalf("clamped sample = %d", code)
	}
	if len(out.Samples) != 10 || !out.Samples[0].Quoted {
		t.Errorf("group sample: n=%d quoted=%v", len(out.Samples), out.Samples[0].Quoted)
	}
	var apiErr apiError
	if code := e.do("POST", "/api/behavior/sample", map[string]any{"kind": "channel"}, &apiErr); code != 400 || apiErr.Code != "invalid_kind" {
		t.Errorf("bad kind: %d %+v", code, apiErr)
	}
	// Approval: no think, typing capped.
	if code := e.do("POST", "/api/behavior/sample", map[string]any{"kind": "dm", "approval": true, "text": strings.Repeat("long words here ", 20)}, &out); code != 200 {
		t.Fatal(code)
	}
	for _, s := range out.Samples {
		for _, ph := range s.Phases {
			if ph.Name == "think" || ph.Name == "distracted" || (ph.Name == "typing" && ph.Sec > 3) {
				t.Errorf("approval phase %+v", ph)
			}
		}
	}
}

func TestChatBehaviorPatchNullSemantics(t *testing.T) {
	e := newEnv(t)
	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "1203630001@g.us", "personaId": "leo",
		"behavior": map[string]any{"replyPercent": 90}}, &c); code != 201 {
		t.Fatalf("create = %d", code)
	}
	if c.Behavior.ReplyPercent == nil || *c.Behavior.ReplyPercent != 90 {
		t.Fatalf("create behavior: %+v", c.Behavior)
	}
	path := "/api/chats/" + strings.ReplaceAll(c.Key, ":", "%3A")
	patch := func(body map[string]any) int {
		t.Helper()
		c = model.ChatAssignment{} // decode into a fresh value
		return e.do("PATCH", path, body, &c)
	}
	if code := patch(map[string]any{"behavior": map[string]any{"waitForMoreSec": 30, "lengthBias": "longer"}}); code != 200 {
		t.Fatalf("patch = %d", code)
	}
	if *c.Behavior.ReplyPercent != 90 || *c.Behavior.WaitForMoreSec != 30 || *c.Behavior.LengthBias != "longer" {
		t.Fatalf("partial keep: %+v", c.Behavior)
	}
	// null inherits one field; other fields stay.
	patch(map[string]any{"behavior": map[string]any{"replyPercent": nil}})
	if c.Behavior.ReplyPercent != nil || c.Behavior.WaitForMoreSec == nil {
		t.Fatalf("null field: %+v", c.Behavior)
	}
	// Availability is a block: set, then replace, then null.
	patch(map[string]any{"behavior": map[string]any{"availability": map[string]any{"enabled": true, "outsideHours": "silent", "catchUpMaxMin": 5,
		"week": []map[string]any{{"day": "tue", "ranges": []map[string]string{{"from": "10:00", "to": "12:00"}}}}}}})
	if c.Behavior.Availability == nil || c.Behavior.Availability.OutsideHours != "silent" || c.Behavior.Availability.CatchUpMaxMin != 5 {
		t.Fatalf("availability set: %+v", c.Behavior.Availability)
	}
	patch(map[string]any{"behavior": map[string]any{"availability": map[string]any{"enabled": false, "outsideHours": "queue"}}})
	if c.Behavior.Availability.CatchUpMaxMin != 0 || len(c.Behavior.Availability.Week) != 0 {
		t.Fatalf("block should be replaced, not merged: %+v", c.Behavior.Availability)
	}
	patch(map[string]any{"behavior": map[string]any{"availability": nil}})
	if c.Behavior.Availability != nil || c.Behavior.WaitForMoreSec == nil {
		t.Fatalf("availability null: %+v", c.Behavior)
	}

	// Invalid → 400 and nothing saved.
	for _, bad := range []map[string]any{
		{"behavior": map[string]any{"replyPercent": 150}},
		{"behavior": map[string]any{"waitForMore": 3}},
		{"behavior": map[string]any{"replyPercent": "lots"}},
		{"behavior": []int{1}},
		{"behavior": map[string]any{"proactive": map[string]any{"enabled": true, "afterHours": 0, "maxPerDay": 1}}},
		{"behavior": map[string]any{"waitForMoreSec": 1}, "snoozedUntil": "tomorrow"},
	} {
		var apiErr apiError
		code := e.do("PATCH", path, bad, &apiErr)
		if code != 400 || (apiErr.Code != "invalid_behavior" && apiErr.Code != "invalid_snooze") {
			t.Errorf("%v: %d %+v", bad, code, apiErr)
		}
	}
	if cur, _ := e.st.Chat(c.Key); *cur.Behavior.WaitForMoreSec != 30 {
		t.Error("invalid patch must not persist")
	}

	// GET …/behavior: effective + sources.
	var view struct {
		Kind         string                  `json:"kind"`
		Effective    model.BehaviorProfile   `json:"effective"`
		Sources      map[string]string       `json:"sources"`
		Overrides    model.BehaviorOverrides `json:"overrides"`
		Defaults     model.BehaviorProfile   `json:"defaults"`
		SnoozedUntil *time.Time              `json:"snoozedUntil"`
		Available    bool                    `json:"available"`
		NextChangeAt *time.Time              `json:"nextChangeAt"`
	}
	if code := e.do("GET", path+"/behavior", nil, &view); code != 200 {
		t.Fatalf("GET behavior = %d", code)
	}
	if view.Kind != "group" || view.Effective.WaitForMoreSec != 30 || view.Effective.LengthBias != "longer" ||
		view.Effective.ReplyPercent != 100 || view.Sources["waitForMoreSec"] != "chat" || view.Sources["replyPercent"] != "default" ||
		view.Sources["availability"] != "default" || view.Defaults.WaitForMoreSec != 12 || view.Effective.Preset != "custom" ||
		view.Overrides.WaitForMoreSec == nil || !view.Available || view.NextChangeAt != nil || view.SnoozedUntil != nil {
		t.Fatalf("view = %+v", view)
	}

	// Snooze set / clear.
	until := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	if code := patch(map[string]any{"snoozedUntil": until.Format(time.RFC3339)}); code != 200 || c.SnoozedUntil == nil || !c.SnoozedUntil.Equal(until) {
		t.Fatalf("snooze: %d %v", code, c.SnoozedUntil)
	}
	view.NextChangeAt, view.SnoozedUntil = nil, nil
	e.do("GET", path+"/behavior", nil, &view)
	if view.Available || view.NextChangeAt == nil || !view.NextChangeAt.Equal(until) || view.SnoozedUntil == nil {
		t.Fatalf("snoozed view: %+v", view)
	}
	patch(map[string]any{"enabled": false})
	if c.SnoozedUntil == nil {
		t.Fatal("absent snoozedUntil must keep it")
	}
	patch(map[string]any{"snoozedUntil": nil})
	if c.SnoozedUntil != nil {
		t.Fatal("null clears snooze")
	}
	// behavior:null resets everything.
	patch(map[string]any{"behavior": nil})
	if c.Behavior != (model.BehaviorOverrides{}) {
		t.Fatalf("reset all: %+v", c.Behavior)
	}
	raw, _ := json.Marshal(c)
	if !strings.Contains(string(raw), `"behavior":{}`) || strings.Contains(string(raw), `"overrides"`) {
		t.Errorf("json = %s", raw)
	}
	var apiErr apiError
	if code := e.do("GET", "/api/chats/dm%3Anope/behavior", nil, &apiErr); code != 404 {
		t.Errorf("unknown chat: %d", code)
	}
}
