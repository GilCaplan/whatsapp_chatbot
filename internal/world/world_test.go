package world

import (
	"slices"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("no tzdata for %s: %v", name, err)
	}
	return loc
}

func TestWeekendTable(t *testing.T) {
	cases := map[string][]string{
		"Israel": {"fri", "sat"}, " il ": {"fri", "sat"}, "Saudi Arabia": {"fri", "sat"},
		"Germany": {"sat", "sun"}, "USA": {"sat", "sun"}, "UAE": {"sat", "sun"},
	}
	for in, want := range cases {
		got, ok := Weekend(in)
		if !ok || !slices.Equal(got, want) {
			t.Errorf("Weekend(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	if _, ok := Weekend("Atlantis"); ok {
		t.Error("unknown country must not be ok")
	}
	if _, ok := Weekend(""); ok {
		t.Error("empty country must not be ok")
	}
	jlm := mustLoc(t, "Asia/Jerusalem")
	if d, known := WeekendFor(model.World{Timezone: "Asia/Jerusalem"}, jlm); !known || d[0] != "fri" {
		t.Errorf("zone guess = %v %v", d, known)
	}
	if d, known := WeekendFor(model.World{}, time.UTC); known || d[0] != "sat" {
		t.Errorf("default = %v %v", d, known)
	}
}

func TestPartOfDay(t *testing.T) {
	want := map[int]string{0: "late night", 4: "late night", 5: "early morning", 8: "morning", 12: "lunchtime", 14: "afternoon", 18: "evening", 21: "evening", 22: "late evening", 23: "late evening"}
	for h, w := range want {
		if got := PartOfDay(h); got != w {
			t.Errorf("PartOfDay(%d) = %q, want %q", h, got, w)
		}
	}
}

func TestRoutineAtAndWrap(t *testing.T) {
	loc := mustLoc(t, "Europe/Berlin")
	w := model.World{Timezone: "Europe/Berlin", Routine: []model.RoutineBlock{
		{Label: "gym", Days: []string{"mon", "wed"}, From: "07:00", To: "08:00", Reach: model.ReachUnreachable},
		{Label: "work", Days: []string{"mon", "tue", "wed", "thu", "fri"}, From: "09:00", To: "17:00", Reach: model.ReachSlow},
		{Label: "sleep", From: "23:30", To: "07:00", Reach: model.ReachUnreachable},
	}}
	// Wednesday 30 Sep 2026.
	at := func(d, h, m int) time.Time { return time.Date(2026, 9, d, h, m, 0, 0, loc) }
	if a, ok := RoutineAt(w, at(30, 7, 30)); !ok || a.Block.Label != "gym" || !a.End.Equal(at(30, 8, 0)) {
		t.Errorf("wed 07:30 = %+v %v", a, ok)
	}
	if _, ok := RoutineAt(w, at(1, 7, 30)); ok { // Thursday 1 Oct (month rollover)
		t.Error("no gym on Thursday")
	}
	if a, ok := RoutineAt(w, at(30, 12, 0)); !ok || a.Block.Label != "work" || a.Block.Reach != model.ReachSlow {
		t.Errorf("wed noon = %+v %v", a, ok)
	}
	// Past midnight: the sleep block started yesterday.
	if a, ok := RoutineAt(w, at(30, 2, 0)); !ok || a.Block.Label != "sleep" || !a.Start.Equal(at(29, 23, 30)) || !a.End.Equal(at(30, 7, 0)) {
		t.Errorf("02:00 = %+v %v", a, ok)
	}
	if _, ok := RoutineAt(w, at(30, 8, 0)); ok {
		t.Error("blocks end exclusive")
	}
	// Sleep then gym chain: unreachable until 08:00.
	if a, ok := Unreachable(w, at(30, 6, 0)); !ok || !a.End.Equal(at(30, 8, 0)) {
		t.Errorf("chain = %+v %v", a, ok)
	}
	// In UTC the same instant still resolves in Berlin time.
	if a, ok := RoutineAt(w, at(30, 7, 30).UTC()); !ok || a.Block.Label != "gym" {
		t.Errorf("utc instant = %+v %v", a, ok)
	}
	between := RoutineBetween(w, at(30, 6, 30), at(30, 9, 30))
	var labels []string
	for _, a := range between {
		labels = append(labels, a.Block.Label)
	}
	if !slices.Equal(labels, []string{"sleep", "gym", "work"}) {
		t.Errorf("between = %v", labels)
	}
	if a, ok := JustFinished(w, at(30, 8, 10), 30*time.Minute); !ok || a.Block.Label != "gym" {
		t.Errorf("just finished = %+v %v", a, ok)
	}
}

func TestRoutineDST(t *testing.T) {
	loc := mustLoc(t, "Europe/Berlin")
	// 25 Oct 2026: clocks go back at 03:00 → 02:00. A 01:00–05:00 block lasts 5 hours.
	w := model.World{Timezone: "Europe/Berlin", Routine: []model.RoutineBlock{{Label: "shift", From: "01:00", To: "05:00", Reach: model.ReachSlow}}}
	a, ok := RoutineAt(w, time.Date(2026, 10, 25, 4, 0, 0, 0, loc))
	if !ok || a.End.Sub(a.Start) != 5*time.Hour {
		t.Fatalf("dst = %+v %v (%v)", a, ok, a.End.Sub(a.Start))
	}
}

func TestLocationFallbackAndNormalize(t *testing.T) {
	if Location(model.World{Timezone: "Mars/Olympus"}) != time.Local {
		t.Error("unknown zone must fall back to Local")
	}
	w := model.World{City: "  Tel   Aviv ", Timezone: "Nope/Zone", Routine: []model.RoutineBlock{
		{Label: "gym", Days: []string{"Monday", "wed", "xyz", "mon"}, From: "7:00", To: "08:00"},
		{Label: "", From: "07:00", To: "08:00"},
		{Label: "bad", From: "25:00", To: "08:00"},
		{Label: "same", From: "08:00", To: "08:00"},
		{Label: "all week", Days: Days, From: "09:00", To: "24:00", Reach: "SLOW"},
	}}
	Normalize(&w)
	if w.City != "Tel Aviv" || w.Timezone != "" || len(w.Routine) != 2 {
		t.Fatalf("normalize = %+v", w)
	}
	if b := w.Routine[0]; b.From != "07:00" || !slices.Equal(b.Days, []string{"mon", "wed"}) || b.Reach != model.ReachNormal {
		t.Errorf("block 0 = %+v", b)
	}
	if b := w.Routine[1]; len(b.Days) != 0 || b.Days == nil || b.Reach != model.ReachSlow || b.To != "24:00" {
		t.Errorf("block 1 = %+v", b)
	}
	var empty model.World
	Normalize(&empty)
	if empty.Routine == nil {
		t.Error("Routine must never be nil after Normalize")
	}
	if err := Validate(model.World{Routine: []model.RoutineBlock{{Label: "x", From: "07:00", To: "07:00"}}}); err == nil {
		t.Error("same from/to must be invalid")
	}
	if err := Validate(model.World{Timezone: "Asia/Tokyo", Routine: []model.RoutineBlock{{Label: "x", Days: []string{"tue"}, From: "07:00", To: "08:00", Reach: "slow"}}}); err != nil {
		t.Error(err)
	}
}

func TestDescribe(t *testing.T) {
	if Place(model.World{City: "Tel Aviv", Country: "Israel"}) != "Tel Aviv, Israel" || Place(model.World{Country: "Japan"}) != "Japan" {
		t.Error("place")
	}
	if FmtDays(nil) != "every day" || FmtDays([]string{"mon", "tue", "wed", "thu", "fri"}) != "Mon–Fri" || FmtDays([]string{"tue"}) != "Tue" {
		t.Error("days")
	}
	if FmtDayRange([]string{"fri", "sat"}) != "Friday–Saturday" {
		t.Error("range")
	}
}
