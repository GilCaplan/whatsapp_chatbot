package engine

import (
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/runtime/llm"
)

func TestSecretary(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.setPersona("leo", func(p *model.Persona) {
		p.Secretary = &model.SecretaryConfig{OwnerName: "Gil", Tone: "friendly", TakeMessages: true, FlagUrgent: true,
			AnswerFAQs: true, Meetings: true, MeetDays: []string{"mon", "tue", "wed", "thu", "fri"}, MeetFrom: "09:00", MeetTo: "18:00",
			MeetingMinutes: 30, FAQs: []model.FAQ{{Q: "Where is the office?", A: "Rothschild 1, Tel Aviv"}}}
	})
	h.e.Reload()
	h.llm.SetFunc(func(req llm.Request) (llm.Response, error) {
		if strings.Contains(req.System, "message pad") {
			return llm.Response{Text: `{"notes":[{"from":"Dana","text":"Dana needs Gil to call her about the contract","urgent":true,"callback":"call today","meeting":""}]}`}, nil
		}
		return llm.Response{Text: "I'll pass it on to Gil right away."}, nil
	})
	h.msg(dmKey, "m1", "hi, I need Gil to call me about the contract today, it's urgent")
	h.advance(15 * time.Second)
	h.waitSent(1)
	notes := h.st.SecretaryNotes()
	if len(notes) != 1 || !notes[0].Urgent || notes[0].From != "Dana" || notes[0].ChatKey != dmKey || notes[0].Callback != "call today" {
		t.Fatalf("notes = %+v", notes)
	}
	if a := h.acts(model.ActSecretaryNote); len(a) != 1 || a[0].Meta["urgent"] != true || !strings.HasPrefix(a[0].Text, "Urgent") {
		t.Errorf("acts = %+v", a)
	}
	sys := lastSystem(h)
	for _, want := range []string{"YOUR JOB: you are Gil's personal assistant", "Rothschild 1", "Suggest times only when YOUR TASK RIGHT NOW"} {
		if !strings.Contains(sys, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if strings.Contains(sys, "FREE TIMES") {
		t.Error("the prompt never lists the free times (a Find a time task offers them)")
	}
	// A second message from her replaces her open note instead of adding a duplicate.
	h.msg(dmKey, "m2", "also, the deadline moved to Friday")
	h.advance(time.Minute)
	open := 0
	for _, n := range h.st.SecretaryNotes() {
		if !n.Done {
			open++
		}
	}
	if open != 1 {
		t.Errorf("open notes = %d", open)
	}
}
