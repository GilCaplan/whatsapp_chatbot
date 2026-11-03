package engine

import (
	"testing"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/model"
)

func withSchedule(sc model.ProactiveSchedule) func(p *model.BehaviorProfile) {
	return func(p *model.BehaviorProfile) { p.Proactive.Schedule = sc }
}

func scheduleActs(h *harness, stage string) []model.ActivityEvent {
	var out []model.ActivityEvent
	for _, a := range h.acts(model.ActProactive) {
		if a.Meta["stage"] == stage {
			out = append(out, a)
		}
	}
	return out
}

func TestScheduleRepeatsFixedGap(t *testing.T) {
	h := newHarness(t, both(withSchedule(model.ProactiveSchedule{Repeat: behavior.RepeatFixed, EveryMin: 60})), dmChat())
	h.advance(59 * time.Minute)
	if len(h.wa.Sent()) != 0 {
		t.Fatal("too early")
	}
	h.advance(2 * time.Minute)
	h.waitSent(1)
	if a := scheduleActs(h, "starting"); len(a) != 1 || a[0].Meta["trigger"] != trigRepeat || a[0].Meta["scheduled"] != true {
		t.Fatalf("acts = %+v", a)
	}
	if st := h.st.RunnerState(dmKey); len(st.Scheduled) != 1 || len(st.Proactive) != 0 || st.ScheduleAt == nil {
		t.Errorf("state = %+v (scheduled messages are not check-ins)", st)
	}
	h.advance(60 * time.Minute)
	h.waitSent(2)
}

// A moment that comes while WhatsApp is disconnected is skipped, and nothing
// goes out late once it reconnects.
func TestScheduleSkipsWhileDisconnected(t *testing.T) {
	h := newHarness(t, both(withSchedule(model.ProactiveSchedule{Repeat: behavior.RepeatFixed, EveryMin: 30})), dmChat())
	h.wa.mu.Lock()
	h.wa.state = model.WADisconnected
	h.wa.mu.Unlock()
	h.advance(95 * time.Minute) // three moments pass
	if len(h.wa.Sent()) != 0 {
		t.Fatal("sent while disconnected")
	}
	skipped := scheduleActs(h, "skipped")
	if len(skipped) != 3 || skipped[0].Meta["reason"] != "disconnected" {
		t.Fatalf("skipped = %+v", skipped)
	}
	h.wa.mu.Lock()
	h.wa.state = ""
	h.wa.mu.Unlock()
	h.advance(10 * time.Minute)
	if len(h.wa.Sent()) != 0 {
		t.Fatal("a missed message was sent late")
	}
	h.advance(20 * time.Minute)
	h.waitSent(1)
}

func TestScheduleFixedTimesAndMissed(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC) // the harness clock
	sc := model.ProactiveSchedule{Times: []time.Time{start.Add(-time.Hour), start.Add(30 * time.Minute), start.Add(90 * time.Minute)}}
	h := newHarness(t, both(withSchedule(sc)), dmChat())
	if a := scheduleActs(h, "skipped"); len(a) != 1 || a[0].Meta["reason"] != "missed" {
		t.Fatalf("the moment before the app started should be skipped: %+v", a)
	}
	h.advance(31 * time.Minute)
	h.waitSent(1)
	h.advance(time.Hour)
	h.waitSent(2)
	if a := scheduleActs(h, "starting"); len(a) != 2 || a[1].Meta["trigger"] != trigTime {
		t.Fatalf("acts = %+v", a)
	}
	if st := h.st.RunnerState(dmKey); st.ScheduleAt != nil || !st.ScheduleDoneTo.Equal(sc.Times[2]) {
		t.Errorf("state = %+v", st)
	}
}

// Repeats count from the first fixed moment ("start at … then every …").
func TestScheduleRepeatsFromFirstTime(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	sc := model.ProactiveSchedule{Times: []time.Time{start.Add(2 * time.Hour)}, Repeat: behavior.RepeatFixed, EveryMin: 60}
	h := newHarness(t, both(withSchedule(sc)), dmChat())
	h.advance(119 * time.Minute)
	if len(h.wa.Sent()) != 0 {
		t.Fatal("repeats must wait for the start time")
	}
	h.advance(2 * time.Minute)
	h.waitSent(1)
	h.advance(60 * time.Minute)
	h.waitSent(2)
}

func TestScheduleOnStartWhenTurnedOn(t *testing.T) {
	off := dmChat()
	off.Enabled = false
	h := newHarness(t, both(withSchedule(model.ProactiveSchedule{OnStart: true})), off)
	h.advance(time.Minute)
	if len(h.wa.Sent()) != 0 {
		t.Fatal("disabled chat")
	}
	if _, err := h.st.UpsertChat(dmChat()); err != nil {
		t.Fatal(err)
	}
	h.e.Reload()
	h.waitSent(1)
	if a := scheduleActs(h, "starting"); len(a) != 1 || a[0].Meta["trigger"] != trigStart {
		t.Fatalf("acts = %+v", a)
	}
	// A restart resumes without a new start message.
	h.e.Reload()
	h.advance(time.Hour)
	if len(h.wa.Sent()) != 1 {
		t.Errorf("sent = %d", len(h.wa.Sent()))
	}
}

func TestScheduleGroupLeftAndJoined(t *testing.T) {
	sc := model.ProactiveSchedule{OnStart: true, Repeat: behavior.RepeatFixed, EveryMin: 30}
	h := newHarness(t, both(withSchedule(sc)), groupChat())
	h.wa.mu.Lock()
	h.wa.left = map[string]bool{groupJID: true}
	h.wa.mu.Unlock()
	h.advance(31 * time.Minute)
	if s := scheduleActs(h, "skipped"); len(s) != 1 || s[0].Meta["reason"] != "left_group" || len(h.wa.Sent()) != 0 {
		t.Fatalf("skipped = %+v", s)
	}
	h.wa.mu.Lock()
	h.wa.left = nil
	h.wa.mu.Unlock()
	h.e.HandleGroupChange(model.GroupChange{Key: groupKey, JID: groupJID, Joined: true})
	h.waitSent(1)
	if a := scheduleActs(h, "starting"); len(a) != 1 || a[0].Meta["trigger"] != trigJoined {
		t.Fatalf("acts = %+v", a)
	}
}

func TestScheduleDailyCap(t *testing.T) {
	h := newHarness(t, both(withSchedule(model.ProactiveSchedule{Repeat: behavior.RepeatFixed, EveryMin: 10, MaxPerDay: 2})), dmChat())
	h.advance(35 * time.Minute)
	if n := len(h.wa.Sent()); n != 2 {
		t.Fatalf("sent = %d", n)
	}
	if s := scheduleActs(h, "skipped"); len(s) != 1 || s[0].Meta["reason"] != "cap" {
		t.Fatalf("skipped = %+v", s)
	}
}
