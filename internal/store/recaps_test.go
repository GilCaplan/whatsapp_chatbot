package store

import (
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

func TestRecapsRoundTrip(t *testing.T) {
	s, _ := openTemp(t)
	if got := s.Recaps("", 0); got == nil || len(got) != 0 {
		t.Fatalf("empty = %#v", got)
	}
	now := time.Now()
	add := func(key, date, headline string, at time.Time) {
		t.Helper()
		if err := s.AppendRecap(model.Recap{ChatKey: key, Date: date, Headline: headline, GeneratedAt: at}, 30); err != nil {
			t.Fatal(err)
		}
	}
	add("dm:1", "2026-09-30", "old day", now.Add(-24*time.Hour))
	add("dm:2", "2026-10-01", "other chat", now)
	add("dm:1", "2026-10-01", "first try", now)
	add("dm:1", "2026-10-01", "second try", now) // same chat + day replaces

	all := s.Recaps("", 0)
	if len(all) != 3 || all[0].Headline != "second try" || all[0].ID == "" || all[0].Topics == nil || all[0].ToKnow == nil {
		t.Fatalf("all = %+v", all)
	}
	if one := s.Recaps("dm:1", 1); len(one) != 1 || one[0].Headline != "second try" {
		t.Errorf("dm:1 limit 1 = %+v", one)
	}
	// keepDays prunes by GeneratedAt.
	add("dm:3", "2026-10-01", "new", now)
	if err := s.AppendRecap(model.Recap{ChatKey: "dm:3", Date: "2026-10-02", Headline: "x", GeneratedAt: now}, 1); err != nil {
		t.Fatal(err)
	}
	for _, r := range s.Recaps("", 0) {
		if r.Headline == "old day" {
			t.Error("keepDays=1 should drop the 24 h old recap")
		}
	}
	if err := s.DeleteRecaps("dm:1"); err != nil {
		t.Fatal(err)
	}
	if got := s.Recaps("dm:1", 0); len(got) != 0 {
		t.Errorf("after delete = %+v", got)
	}
	if got := s.Recaps("dm:2", 0); len(got) != 1 {
		t.Errorf("other chats kept = %+v", got)
	}
}
