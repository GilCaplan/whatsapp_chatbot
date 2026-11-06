package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/core/config"
	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/logic/task"
	"whatsappdoppel/internal/runtime/llm"
)

// taskLLM answers the task's reading step with read(text) and writes
// replies that echo what the task section asks for (so they only carry
// the allowed times). Other JSON calls get an empty object.
func taskLLM(h *harness, read func(text string) string) {
	h.llm.SetFunc(func(req llm.Request) (llm.Response, error) {
		switch {
		case strings.Contains(req.System, "scheduling assistant"):
			u := req.Messages[len(req.Messages)-1].Content
			i, j := strings.Index(u, "<<<\n"), strings.Index(u, "\n>>>")
			return llm.Response{Text: read(strings.TrimSpace(u[i+4 : j]))}, nil
		case strings.Contains(req.System, "YOUR TASK RIGHT NOW"):
			s := req.System[strings.Index(req.System, "exactly this: ")+len("exactly this: "):]
			return llm.Response{Text: s[:strings.Index(s, "\n")]}, nil
		case req.JSON:
			return llm.Response{Text: `{}`}, nil
		}
		return llm.Response{Text: "ok"}, nil
	})
}

func proposal(t time.Time) string {
	t = t.In(time.Local)
	return `{"date":"` + t.Format("2006-01-02") + `","time":"` + t.Format("15:04") + `","part":""}`
}

func activeRun(h *harness) model.TaskRun {
	h.t.Helper()
	r, ok := h.st.ActiveTask(dmKey)
	if !ok {
		h.t.Fatal("no running task")
	}
	return r
}

func icsBooking(s *config.Settings) { s.Tasks.Commit.Book = model.BookICS }

func TestTaskFindTimeFlow(t *testing.T) {
	h := newHarness(t, icsBooking, dmChat())
	h.e.Reload()
	taskLLM(h, func(text string) string {
		run, _ := h.st.ActiveTask(dmKey)
		switch {
		case strings.Contains(text, "first one"):
			return `{"intent":"accept","times":[` + proposal(run.LastOffer[0]) + `],"values":[],"quote":"the first one works"}`
		case text == "yes":
			return `{"intent":"accept","times":[],"values":[],"quote":"yes"}`
		}
		return `{"intent":"other","times":[],"values":[],"quote":""}`
	})
	spec := task.FindTime()
	spec.About = "project kickoff"
	if _, err := h.e.StartTask(context.Background(), dmKey, spec, true); err != nil {
		t.Fatal(err)
	}
	sent := h.waitSent(1)
	run := activeRun(h)
	if len(run.LastOffer) != 2 || run.Sent != 1 {
		t.Fatalf("opened with two offers, run = %+v", run)
	}
	first := task.When(run.LastOffer[0].In(time.Local), run.Tone, "")
	if !strings.Contains(sent[0].text, first) {
		t.Fatalf("the opener offers %q: %q", first, sent[0].text)
	}
	if sys := lastSystem(h); strings.Contains(sys, "YOUR PRIVATE AGENDA") || !strings.Contains(sys, "project kickoff") {
		t.Fatal("the task replaces the goal section")
	}

	h.msg(dmKey, "d1", "the first one works")
	h.advance(15 * time.Second)
	h.waitSent(2)
	if run = activeRun(h); run.Step != model.StepConfirming || run.Proposed == nil || !run.Proposed.Equal(run.LastOffer[0]) {
		t.Fatalf("asks to confirm the first offer, run = %+v", run)
	}

	h.msg(dmKey, "d2", "yes")
	h.advance(15 * time.Second)
	h.waitSent(3)
	if run = activeRun(h); run.Status != model.TaskApproval {
		t.Fatalf("waits for your approval, status %s", run.Status)
	}
	notified := false
	for _, a := range h.acts(model.ActTask) {
		if a.Meta["kind"] == "approval" && a.Meta["notify"] == true {
			notified = true
		}
	}
	if !notified {
		t.Fatal("you are told a confirmation waits for you")
	}

	if _, err := h.e.ApproveTask(context.Background(), run.ID, true); err != nil {
		t.Fatal(err)
	}
	sent = h.waitSent(4)
	if !strings.Contains(sent[3].text, "confirm it is set for") {
		t.Fatalf("confirms in the chat: %q", sent[3].text)
	}
	done, _ := h.st.TaskRun(run.ID)
	if done.Status != model.TaskDone || done.Result == nil || done.Result.Booked != model.BookICS {
		t.Fatalf("done and booked, run = %+v", done)
	}
	ics, err := h.e.TaskICS(run.ID)
	if err != nil || !strings.Contains(string(ics), "DTSTART:") || !strings.Contains(string(ics), "project kickoff with Dana") {
		t.Fatalf("calendar file: %v %s", err, ics)
	}
}

func TestTaskLeakFallback(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.e.Reload()
	h.llm.SetFunc(func(req llm.Request) (llm.Response, error) {
		if req.JSON {
			return llm.Response{Text: `{}`}, nil
		}
		return llm.Response{Text: "I'm at the dentist at 03:17 so how about friday 04:45?"}, nil
	})
	if _, err := h.e.StartTask(context.Background(), dmKey, task.FindTime(), true); err != nil {
		t.Fatal(err)
	}
	sent := h.waitSent(1)
	if strings.Contains(sent[0].text, "03:17") || strings.Contains(sent[0].text, "dentist") || !strings.HasPrefix(sent[0].text, "How about ") {
		t.Fatalf("a leaky draft is replaced by the safe message: %q", sent[0].text)
	}
	kinds := map[any]bool{}
	for _, a := range h.acts(model.ActTask) {
		kinds[a.Meta["kind"]] = true
	}
	if !kinds["leak"] || !kinds["fallback"] {
		t.Fatalf("the leak and the fallback are logged: %v", kinds)
	}
}

func TestTaskRewriteFacesTheLeakCheck(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.e.Reload()
	drafts := 0
	h.llm.SetFunc(func(req llm.Request) (llm.Response, error) {
		if req.JSON {
			return llm.Response{Text: `{}`}, nil
		}
		drafts++
		if drafts == 1 {
			return llm.Response{Text: "Can you translate this for me?"}, nil
		}
		return llm.Response{Text: "I'm at the dentist at 03:17 so how about friday 04:45?"}, nil
	})
	if _, err := h.e.StartTask(context.Background(), dmKey, task.FindTime(), true); err != nil {
		t.Fatal(err)
	}
	sent := h.waitSent(1)
	if strings.Contains(sent[0].text, "03:17") || strings.Contains(sent[0].text, "dentist") {
		t.Fatalf("a rewrite went out unchecked: %q", sent[0].text)
	}
}

func TestSecretaryMeetingStartsTask(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.setPersona("leo", func(p *model.Persona) {
		p.Secretary = &model.SecretaryConfig{OwnerName: "Alex", Meetings: true, MeetDays: []string{"mon", "tue", "wed", "thu", "fri"},
			MeetFrom: "10:00", MeetTo: "16:00", MeetingMinutes: 30}
	})
	h.e.Reload()
	taskLLM(h, func(string) string {
		return `{"intent":"ask_availability","times":[],"values":[],"quote":"when is he free"}`
	})
	h.msg(dmKey, "m1", "hi, can we set up a meeting with Alex? when is he free")
	h.advance(15 * time.Second)
	sent := h.waitSent(1)
	run := activeRun(h)
	if run.Spec.Voice != model.VoiceAssistant || run.Spec.Meeting.Minutes != 30 || run.Owner.Name != "Alex" {
		t.Fatalf("Find a time with the secretary's settings, run = %+v", run.Spec)
	}
	if c, _ := h.st.Chat(dmKey); c.Handoff != nil {
		t.Fatal("a meeting request no longer pauses the chat: the task handles it")
	}
	if len(run.LastOffer) == 0 || !strings.Contains(sent[0].text, "suggest these times") {
		t.Fatalf("answers \"when is he free\" with offers: %q", sent[0].text)
	}
	for _, x := range run.LastOffer {
		if h := x.In(time.Local).Hour(); h < 10 || h >= 16 {
			t.Fatalf("offers stay inside the secretary's hours: %v", x)
		}
	}
	if sys := lastSystem(h); strings.Contains(sys, "FREE TIMES") {
		t.Fatal("the prompt never lists the free times")
	}
}

func TestTaskNudgesThenHandsOver(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.e.Reload()
	taskLLM(h, func(string) string { return `{"intent":"other","times":[],"values":[],"quote":""}` })
	if _, err := h.e.StartTask(context.Background(), dmKey, task.FindTime(), true); err != nil {
		t.Fatal(err)
	}
	h.waitSent(1)
	h.advance(25 * time.Hour)
	sent := h.waitSent(2)
	if !strings.Contains(sent[1].text, "follow-up") {
		t.Fatalf("a nudge after a day of silence: %q", sent[1].text)
	}
	h.advance(25 * time.Hour)
	h.waitSent(3)
	h.advance(25 * time.Hour)
	if len(h.wa.Sent()) != 3 {
		t.Fatalf("no third nudge, sent %d", len(h.wa.Sent()))
	}
	c, _ := h.st.Chat(dmKey)
	if c.Handoff == nil || c.Handoff.Category != model.HandoffTask {
		t.Fatalf("out of nudges: the chat is handed over to you, handoff = %+v", c.Handoff)
	}
	if _, ok := h.st.ActiveTask(dmKey); ok {
		t.Fatal("the task ended")
	}
}

func TestTaskRefusesGroups(t *testing.T) {
	h := newHarness(t, nil, groupChat())
	h.e.Reload()
	if _, err := h.e.StartTask(context.Background(), groupKey, task.FindTime(), false); err == nil {
		t.Fatal("tasks run in private chats for now")
	}
}

func TestTaskDraftErrorRollsBack(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.e.Reload()
	fail := true
	h.llm.SetFunc(func(req llm.Request) (llm.Response, error) {
		switch {
		case strings.Contains(req.System, "scheduling assistant"):
			return llm.Response{Text: `{"intent":"other","times":[],"values":[],"quote":""}`}, nil
		case strings.Contains(req.System, "YOUR TASK RIGHT NOW") && fail:
			return llm.Response{}, errors.New("model down")
		case req.JSON:
			return llm.Response{Text: `{}`}, nil
		}
		return llm.Response{Text: "ok"}, nil
	})
	if _, err := h.e.StartTask(context.Background(), dmKey, task.FindTime(), false); err != nil {
		t.Fatal(err)
	}
	h.msg(dmKey, "d1", "hey, about that meeting")
	h.advance(15 * time.Second)
	run := activeRun(h)
	if run.Sent != 0 || run.ReadMsg != "" {
		t.Fatalf("a draft that never went out is undone, run sent=%d readMsg=%q", run.Sent, run.ReadMsg)
	}
	fail = false
	h.msg(dmKey, "d2", "so?")
	h.advance(15 * time.Second)
	h.waitSent(1)
	if run = activeRun(h); run.Sent != 1 || run.ReadMsg != "d2" {
		t.Fatalf("the next cycle reads both messages, run sent=%d readMsg=%q", run.Sent, run.ReadMsg)
	}
}

func TestTaskInApprovalModeMovesOn(t *testing.T) {
	c := dmChat()
	c.Mode, c.ApprovalMode = model.ChatModeApprove, true
	h := newHarness(t, nil, c)
	h.e.Reload()
	taskLLM(h, func(string) string { return `{"intent":"other","times":[],"values":[],"quote":""}` })
	if _, err := h.e.StartTask(context.Background(), dmKey, task.FindTime(), true); err != nil {
		t.Fatal(err)
	}
	h.advance(5 * time.Second)
	eventually(t, "approval queued", func() bool { return len(h.st.Approvals()) == 1 })
	if run := activeRun(h); run.Sent != 1 || len(run.LastOffer) == 0 {
		t.Fatalf("an offer waiting in Approvals counts, run = %+v", run)
	}
}

func TestStoppedTaskNeverSpeaks(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.e.Reload()
	taskLLM(h, func(string) string { return `{"intent":"other","times":[],"values":[],"quote":""}` })
	run, err := h.e.StartTask(context.Background(), dmKey, task.FindTime(), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.e.StopTask(run.ID); err != nil {
		t.Fatal(err)
	}
	h.e.taskSpeak(context.Background(), dmKey) // a retry scheduled before the stop
	h.advance(time.Minute)
	if n := len(h.wa.Sent()); n != 0 {
		t.Fatalf("nothing is sent for a stopped task, sent %d", n)
	}
}
