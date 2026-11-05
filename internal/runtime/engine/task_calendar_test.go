package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"whatsappdoppel/internal/core/config"
	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/logic/calendar"
	"whatsappdoppel/internal/logic/task"
)

const secretEvent = "Secret surgery at the clinic"

// withCalendar is a harness with one calendar holding a private event; reads
// counts how often the app fetched it.
func withCalendar(t *testing.T, mut func(*config.Settings)) (*harness, *atomic.Int32) {
	t.Helper()
	var reads atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reads.Add(1)
		fmt.Fprintf(w, "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:1\r\nDTSTART:20261005T090000Z\r\nDTEND:20261005T100000Z\r\nSUMMARY:%s\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", secretEvent)
	}))
	t.Cleanup(srv.Close)
	h := newHarnessDeps(t, mut, Deps{Calendar: calendar.New(nil)}, dmChat())
	if _, err := h.st.UpdateCalendars(func(cur []model.CalendarSource) ([]model.CalendarSource, error) {
		return append(cur, model.CalendarSource{ID: "c1", Name: "Private", Kind: model.CalendarICS, URL: srv.URL, Enabled: true}), nil
	}); err != nil {
		t.Fatal(err)
	}
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
	return h, &reads
}

// nothingLeaked: no request to the model, and nothing sent, names the event.
func nothingLeaked(t *testing.T, h *harness) {
	t.Helper()
	for _, r := range h.llm.Requests() {
		text := r.System
		for _, m := range r.Messages {
			text += m.Content
		}
		if strings.Contains(text, secretEvent) || strings.Contains(text, "surgery") || strings.Contains(text, "127.0.0.1") {
			t.Fatalf("the model saw the calendar:\n%s", text)
		}
	}
	for _, s := range h.wa.Sent() {
		if strings.Contains(s.text, "surgery") {
			t.Fatalf("sent: %q", s.text)
		}
	}
}

// No permission, no read: a task doesn't touch your calendars by default.
func TestTaskLeavesCalendarAloneByDefault(t *testing.T) {
	h, reads := withCalendar(t, nil)
	if task.FindTime().Meeting.Calendars {
		t.Fatal("calendars must be off by default")
	}
	if _, err := h.e.StartTask(context.Background(), dmKey, task.FindTime(), true); err != nil {
		t.Fatal(err)
	}
	h.waitSent(1)
	h.msg(dmKey, "m1", "the first one works for me")
	h.advance(15 * time.Second)
	if n := reads.Load(); n != 0 {
		t.Fatalf("read the calendar %d times without permission", n)
	}
	nothingLeaked(t, h)
}

// With your permission code reads it, and still only times reach the model.
func TestAllowedCalendarStaysOutOfTheModel(t *testing.T) {
	h, reads := withCalendar(t, nil)
	spec := task.FindTime()
	spec.Meeting.Calendars = true
	if _, err := h.e.StartTask(context.Background(), dmKey, spec, true); err != nil {
		t.Fatal(err)
	}
	h.waitSent(1)
	h.msg(dmKey, "m1", "the first one works for me")
	h.advance(15 * time.Second)
	if reads.Load() == 0 {
		t.Fatal("the allowed calendar was not read")
	}
	nothingLeaked(t, h)
}

// "Confirm by itself if free" can't know you're free without the calendar.
func TestAutoConfirmNeedsTheCalendar(t *testing.T) {
	auto := func(s *config.Settings) { s.Tasks.Commit.Mode = model.CommitAutoFree }
	h, _ := withCalendar(t, auto)
	if _, err := h.e.StartTask(context.Background(), dmKey, task.FindTime(), true); err != nil {
		t.Fatal(err)
	}
	h.waitSent(1)
	h.msg(dmKey, "m1", "the first one works for me")
	h.advance(15 * time.Second)
	h.msg(dmKey, "m2", "yes")
	h.advance(15 * time.Second)
	run, ok := h.st.ActiveTask(dmKey)
	if !ok || run.Status != model.TaskApproval {
		t.Fatalf("it confirmed by itself: ok=%v status=%q", ok, run.Status)
	}
}

// Nothing about this computer is in the prompt.
func TestPromptKnowsNothingOfThisComputer(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	setWorld(t, h, model.World{City: "Tokyo", Country: "Japan", Timezone: "Asia/Tokyo"})
	h.simulate(dmKey, "hey", false)
	h.advance(10 * time.Second)
	h.waitSent(1)
	if sys := lastReplyRequest(t, h).System; strings.Contains(sys, "people you're chatting with") {
		t.Fatalf("prompt:\n%s", sys)
	}
}
