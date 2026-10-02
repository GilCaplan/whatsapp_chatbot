package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
)

// Wave 3 realism (Engineer A): typos, world & routine, memory, clone.

func setWorld(t *testing.T, h *harness, w model.World) {
	t.Helper()
	p, ok := h.st.Persona("leo")
	if !ok {
		t.Fatal("no leo")
	}
	p.World = w
	if _, err := h.st.UpsertPersona(p); err != nil {
		t.Fatal(err)
	}
}

func lastReplyRequest(t *testing.T, h *harness) llm.Request {
	t.Helper()
	reqs := replyRequests(h.llm)
	if len(reqs) == 0 {
		t.Fatal("no reply request")
	}
	return reqs[len(reqs)-1]
}

// ─── typos ───────────────────────────────────────────────────

func typoSettings(style string) func(*config.Settings) {
	return both(func(p *model.BehaviorProfile) { p.TypoPercent, p.TypoFixStyle = 30, style })
}

func TestTypoCorrectionBubble(t *testing.T) {
	h := newHarness(t, typoSettings("correction"), dmChat())
	h.rng.set(behavior.Fixed{Hits: true})
	h.llm.Push("see you tomorrow at the cafe")
	h.simulate(dmKey, "when?", false)
	h.advance(15 * time.Second)
	sent := h.waitSent(2)
	// (Hits:true also lets the emoji rules add one, hence the prefixes.)
	if len(sent) != 2 || !strings.HasPrefix(sent[0].text, "see you tmoorrow at the cafe") || sent[1].text != "*tomorrow" {
		t.Fatalf("sent = %+v", sent)
	}
	hist := h.history(dmKey)
	var bubble, fix model.Message
	for _, m := range hist {
		switch {
		case m.Kind == model.MsgKindFix:
			fix = m
		case m.Speaker == "me":
			bubble = m
		}
	}
	if !strings.HasPrefix(bubble.Corrected, "see you tomorrow at the cafe") || bubble.WAID == "" || fix.Text != "*tomorrow" {
		t.Fatalf("history = %+v", hist)
	}
	// The model only ever sees what was meant.
	for _, m := range prompt.Messages(hist, false) {
		if strings.Contains(m.Content, "tmoorrow") || strings.HasPrefix(m.Content, "*") {
			t.Errorf("typo leaked into the prompt: %+v", m)
		}
	}
	var typo, fixed bool
	for _, a := range h.acts(model.ActSent) {
		typo = typo || a.Meta["typo"] == true
		fixed = fixed || a.Meta["fix"] == true
	}
	if !typo || !fixed {
		t.Error("sent activity should mark typo and fix")
	}
}

func TestTypoEditMessage(t *testing.T) {
	h := newHarness(t, typoSettings("edit"), dmChat())
	h.rng.set(behavior.Fixed{Hits: true})
	h.llm.Push("see you tomorrow at the cafe")
	h.simulate(dmKey, "when?", false)
	h.advance(20 * time.Second)
	sent := h.waitSent(1)
	edits := h.wa.Edits()
	if len(sent) != 1 || len(edits) != 1 || edits[0].id != sent[0].id || !strings.HasPrefix(edits[0].text, "see you tomorrow at the cafe") {
		t.Fatalf("sent=%+v edits=%+v", sent, edits)
	}
	var me model.Message
	for _, m := range h.history(dmKey) {
		if m.Speaker == "me" {
			me = m
		}
	}
	if me.Kind != model.MsgKindEdited || !strings.HasPrefix(me.Text, "see you tomorrow at the cafe") || me.Corrected != "" {
		t.Errorf("history = %+v", me)
	}

	// A failed edit falls back to a "*word" bubble.
	h2 := newHarness(t, typoSettings("edit"), dmChat())
	h2.wa.failEdit = true
	h2.rng.set(behavior.Fixed{Hits: true})
	h2.llm.Push("see you tomorrow at the cafe")
	h2.simulate(dmKey, "when?", false)
	h2.advance(20 * time.Second)
	if sent := h2.waitSent(2); sent[1].text != "*tomorrow" {
		t.Errorf("fallback sent = %+v", sent)
	}
}

func TestNoTypoOnTriggerOrApproval(t *testing.T) {
	h := newHarness(t, typoSettings("correction"), dmChat())
	h.rng.set(behavior.Fixed{Hits: true})
	h.llm.Push("see you tomorrow at the cafe")
	h.simulate(dmKey, "1 when?", true)
	h.advance(10 * time.Second)
	if sent := h.waitSent(1); len(sent) != 1 || !strings.HasPrefix(sent[0].text, "see you tomorrow at the cafe") {
		t.Fatalf("trigger sent = %+v", sent)
	}

	c := dmChat()
	c.ApprovalMode = true
	h2 := newHarness(t, typoSettings("correction"), c)
	h2.rng.set(behavior.Fixed{Hits: true})
	h2.llm.Push("see you tomorrow at the cafe")
	h2.simulate(dmKey, "when?", false)
	h2.advance(10 * time.Second)
	eventually(t, "approval", func() bool { return len(h2.st.Approvals()) == 1 })
	if err := h2.e.SendApproved(context.Background(), h2.st.Approvals()[0].ID, "", -1); err != nil {
		t.Fatal(err)
	}
	h2.advance(30 * time.Second)
	if sent := h2.waitSent(1); len(sent) != 1 || !strings.HasPrefix(sent[0].text, "see you tomorrow at the cafe") {
		t.Fatalf("approved sent = %+v", sent)
	}
}

// ─── world & routine ─────────────────────────────────────────

func TestWorldSectionUsesPersonaZone(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	setWorld(t, h, model.World{City: "Tokyo", Country: "Japan", Timezone: "Asia/Tokyo"})
	h.simulate(dmKey, "hey", false)
	h.advance(10 * time.Second)
	h.waitSent(1)
	sys := lastReplyRequest(t, h).System
	if !strings.Contains(sys, "RIGHT NOW: Thursday 1 October, 21:00 — evening in Tokyo, Japan (your local time).") {
		t.Fatalf("world section missing:\n%s", sys)
	}
	if !strings.Contains(sys, "The weekend (Saturday–Sunday here) starts") && !strings.Contains(sys, "The weekend here is Saturday–Sunday") {
		t.Errorf("weekend line missing:\n%s", sys)
	}
}

func TestAvailabilityFollowsPersonaZone(t *testing.T) {
	week := []model.DayHours{}
	for _, d := range []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"} {
		week = append(week, model.DayHours{Day: d, Ranges: []model.TimeRange{{From: "09:00", To: "17:00"}}})
	}
	av := model.Availability{Enabled: true, Timezone: model.PersonaZoneMarker, Week: week, OutsideHours: "queue", CatchUpMaxMin: 0}
	c := dmChat()
	c.Behavior.Availability = &av
	// 12:00 UTC = 21:00 in Tokyo: outside the persona's 09–17 → queued.
	h := newHarness(t, nil, c)
	setWorld(t, h, model.World{Timezone: "Asia/Tokyo"})
	h.simulate(dmKey, "hey", false)
	h.advance(10 * time.Second)
	if len(h.wa.Sent()) != 0 || len(h.acts(model.ActDeferred)) != 1 {
		t.Fatalf("tokyo: sent=%v deferred=%d", h.wa.Sent(), len(h.acts(model.ActDeferred)))
	}
	// Same hours in UTC (12:00 is inside 09–17) → answered.
	h2 := newHarness(t, nil, c)
	setWorld(t, h2, model.World{Timezone: "UTC"})
	h2.simulate(dmKey, "hey", false)
	h2.advance(10 * time.Second)
	h2.waitSent(1)
}

func TestRoutineUnreachableQueuesAndLateNote(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	setWorld(t, h, model.World{Timezone: "UTC", Routine: []model.RoutineBlock{
		{Label: "gym", From: "11:30", To: "13:00", Reach: model.ReachUnreachable},
	}})
	h.simulate(dmKey, "are you free tonight?", false)
	h.advance(10 * time.Second)
	if len(h.wa.Sent()) != 0 {
		t.Fatal("answered during the gym")
	}
	d := h.acts(model.ActDeferred)
	if len(d) != 1 || d[0].Meta["reason"] != "routine" || d[0].Meta["block"] != "gym" || !strings.Contains(d[0].Text, "gym") {
		t.Fatalf("deferred = %+v", d)
	}
	h.advance(time.Hour)
	h.waitSent(1)
	sys := lastReplyRequest(t, h).System
	if !strings.Contains(sys, "LATE REPLY") || !strings.Contains(sys, "gym until 13:00") {
		t.Errorf("late note missing:\n%s", sys)
	}
}

func TestRoutineSlowNotice(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	setWorld(t, h, model.World{Timezone: "UTC", Routine: []model.RoutineBlock{
		{Label: "work", From: "09:00", To: "17:00", Reach: model.ReachSlow},
	}})
	h.simulate(dmKey, "hey", false)
	n := h.acts(model.ActNoticing)
	if len(n) != 1 || n[0].Meta["reason"] != "routine" || n[0].Meta["delaySeconds"] != 90.0 {
		t.Fatalf("noticing = %+v", n)
	}
	h.advance(80 * time.Second)
	if len(h.wa.Sent()) != 0 {
		t.Fatal("noticed too early")
	}
	h.advance(30 * time.Second)
	h.waitSent(1)
}

func TestLateNoteGeneric(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	p, _ := h.st.Persona("leo")
	now := h.clk.Now()
	hist := []model.Message{{Speaker: "them", Text: "hi", TS: now.Add(-50 * time.Minute)}}
	if l := h.e.lateNote(p, hist, now); l == nil || l.Minutes != 50 || l.Busy != "" {
		t.Errorf("generic = %+v", l)
	}
	hist[0].TS = now.Add(-20 * time.Minute)
	if l := h.e.lateNote(p, hist, now); l != nil {
		t.Errorf("20 min without a routine is not late: %+v", l)
	}
}

// ─── memory ──────────────────────────────────────────────────

func memoryLLM(h *harness, found string) {
	h.llm.SetFunc(func(req llm.Request) (llm.Response, error) {
		if req.JSON && strings.Contains(req.System, "private notebook") {
			return llm.Response{Text: found}, nil
		}
		return llm.Response{Text: "nice"}, nil
	})
}

func TestMemoryExtractionAfterSixMessages(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	memoryLLM(h, `{"memories":[
	  {"person":"Dana","evidence":"i work as a nurse at Ichilov","text":"Dana works as a nurse at Ichilov","kind":"fact","expires":""},
	  {"person":"Dana","evidence":"i love skydiving","text":"loves skydiving","kind":"preference","expires":""},
	  {"person":"Leo","evidence":"i work as a nurse at Ichilov","text":"is a nurse","kind":"fact","expires":""},
	  {"person":"Dana","evidence":"my exam is on saturday","text":"has an exam on Saturday 3 Oct","kind":"event","expires":"2026-10-03"}
	]}`)
	msgs := []string{"hey", "long day", "i work as a nurse at Ichilov", "night shifts again", "my exam is on saturday", "ugh"}
	for _, m := range msgs[:5] {
		h.simulate(dmKey, m, false)
		h.advance(30 * time.Second)
	}
	if h.e.jobPending(memoryJob(dmKey)) {
		t.Fatal("job armed before six messages")
	}
	h.simulate(dmKey, msgs[5], false)
	if !h.e.jobPending(memoryJob(dmKey)) {
		t.Fatal("job not armed after six messages")
	}
	h.advance(100 * time.Second)
	eventually(t, "memories", func() bool { return h.st.MemoryCount(dmKey) > 0 })
	mems, _ := h.st.Memories(dmKey)
	if len(mems) != 2 {
		t.Fatalf("memories = %+v", mems)
	}
	var nurse, exam model.Memory
	for _, m := range mems {
		if strings.Contains(m.Text, "nurse") {
			nurse = m
		}
		if strings.Contains(m.Text, "exam") {
			exam = m
		}
	}
	if nurse.Text != "works as a nurse at Ichilov" || nurse.Person != "Dana" || nurse.Source != model.MemorySourceLearned || nurse.PersonJID != dmJID {
		t.Errorf("nurse = %+v", nurse)
	}
	if exam.ExpiresAt == nil || exam.Kind != model.MemoryEvent {
		t.Errorf("exam = %+v", exam)
	}
	if a := h.acts(model.ActMemory); len(a) != 1 || a[0].Meta["added"] != 2 || !strings.Contains(a[0].Text, "Dana") {
		t.Errorf("memory activity = %+v", a)
	}
	if st := h.st.RunnerState(dmKey); st.MemoryPending != 0 || st.MemoryCursor.IsZero() {
		t.Errorf("state = %+v", st)
	}
	// The next reply remembers.
	h.simulate(dmKey, "so tired", false)
	h.advance(30 * time.Second)
	sys := lastReplyRequest(t, h).System
	if !strings.Contains(sys, "WHAT YOU REMEMBER ABOUT THEM") || !strings.Contains(sys, "- works as a nurse at Ichilov") {
		t.Errorf("prompt lacks memories:\n%s", sys)
	}
}

func TestMemoryOffForChat(t *testing.T) {
	c := dmChat()
	off := false
	c.Memory = &off
	h := newHarness(t, nil, c)
	if _, err := h.st.UpsertMemory(model.Memory{ChatKey: dmKey, Text: "likes jazz", Source: model.MemorySourceUser}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 7; i++ {
		h.simulate(dmKey, "message number one two", false)
	}
	if h.e.jobPending(memoryJob(dmKey)) {
		t.Error("memory off: no extraction")
	}
	h.advance(30 * time.Second)
	if sys := lastReplyRequest(t, h).System; strings.Contains(sys, "likes jazz") {
		t.Error("memory off: no memories in the prompt")
	}
}

func TestExtractMemoriesNowAndGroups(t *testing.T) {
	h := newHarness(t, nil, groupChat())
	memoryLLM(h, `{"memories":[
	  {"person":"Josh","evidence":"Arsenal till i die","text":"supports Arsenal","kind":"preference","expires":""},
	  {"person":"Stranger","evidence":"Arsenal till i die","text":"x","kind":"fact","expires":""}
	]}`)
	for _, m := range []model.Message{
		{Speaker: "them", Name: "Josh", SenderJID: "111@s.whatsapp.net", Text: "Arsenal till i die", TS: h.clk.Now()},
		{Speaker: "me", Text: "brave", TS: h.clk.Now(), FromBot: true},
	} {
		if err := h.st.AppendHistory(groupKey, m, 0); err != nil {
			t.Fatal(err)
		}
	}
	res, err := h.e.ExtractMemories(context.Background(), groupKey)
	if err != nil || res.Added != 1 {
		t.Fatalf("extract = %+v %v", res, err)
	}
	mems, _ := h.st.Memories(groupKey)
	if mems[0].Person != "Josh" || mems[0].PersonJID != "111@s.whatsapp.net" {
		t.Errorf("mems = %+v", mems)
	}
	// Nothing new since the cursor: no model call.
	n := len(h.llm.Requests())
	if res, err := h.e.ExtractMemories(context.Background(), groupKey); err != nil || res.Added != 0 || len(h.llm.Requests()) != n {
		t.Errorf("second extract = %+v %v (calls %d→%d)", res, err, n, len(h.llm.Requests()))
	}
}

func TestMemoryJobStopsWithEngine(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.e.schedule("x", time.Minute, func(context.Context) { t.Error("job ran after Stop") })
	h.e.Stop()
	h.clk.Advance(2 * time.Minute)
	if h.e.jobPending("x") {
		t.Error("job still pending")
	}
}

// ─── clone ───────────────────────────────────────────────────

func TestCloneDraft(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	if _, err := h.e.Builder().CloneDraft(context.Background(), model.CloneRequest{Name: "Me"}); err == nil {
		t.Fatal("no samples must fail")
	}
	for i := 0; i < 25; i++ {
		_ = h.st.AppendSelfSample(model.SelfSample{TS: h.clk.Now().Add(time.Duration(i) * time.Second), Kind: "dm", Text: "lol ok see u at 8 call me 054-123-4567"}, 500)
	}
	out, err := h.e.Builder().CloneDraft(context.Background(), model.CloneRequest{Name: "Rocky", Extra: "student in Haifa"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Persona.Name != "Rocky" || out.SampleCount != 25 || out.Persona.ID != "" {
		t.Errorf("draft = %+v", out)
	}
	req := h.llm.Requests()[len(h.llm.Requests())-1]
	body := req.Messages[0].Content
	if !strings.Contains(body, "Name: Rocky") || !strings.Contains(body, "student in Haifa") || strings.Contains(body, "4567") || !strings.Contains(body, "[number]") {
		t.Errorf("clone request:\n%s", body)
	}
	if strings.Count(body, "lol ok") != 1 {
		t.Error("samples should be deduplicated")
	}
	var probe map[string]any
	if json.Unmarshal(req.Schema, &probe) != nil {
		t.Error("schema must be JSON")
	}
}
