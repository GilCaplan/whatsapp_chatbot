package store

import (
	"errors"
	"os"
	"runtime"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

func TestMemoriesRoundTrip(t *testing.T) {
	s, p := openTemp(t)
	if m, err := s.Memories("dm:1"); err != nil || len(m) != 0 || m == nil {
		t.Fatalf("empty: %v %v", m, err)
	}
	a, err := s.UpsertMemory(model.Memory{ChatKey: "dm:1", Person: "Dana", Text: "works as a nurse", Source: model.MemorySourceUser})
	if err != nil || a.ID == "" || a.CreatedAt.IsZero() {
		t.Fatalf("add: %+v %v", a, err)
	}
	b, _ := s.UpsertMemory(model.Memory{ChatKey: "dm:1", Person: "Dana", Text: "likes jazz", Pinned: true})
	a.Text = "works as a night nurse"
	if _, err := s.UpsertMemory(a); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertMemory(model.Memory{ID: "nope", ChatKey: "dm:1", Text: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
	ms, _ := s.Memories("dm:1")
	if len(ms) != 2 || ms[0].ID != b.ID || ms[1].Text != "works as a night nurse" {
		t.Fatalf("list: %+v", ms)
	}
	// Windows has no Unix permission bits (the per-user profile folder protects the files).
	if st, err := os.Stat(s.memoriesFile("dm:1")); err != nil || (runtime.GOOS != "windows" && st.Mode().Perm() != 0o600) {
		t.Errorf("file mode: %v %v", st, err)
	}
	if err := s.DeleteMemory("dm:1", "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete unknown: %v", err)
	}
	if err := s.DeleteMemory("dm:1", b.ID); err != nil || s.MemoryCount("dm:1") != 1 {
		t.Errorf("delete: %v %d", err, s.MemoryCount("dm:1"))
	}
	if err := s.ClearMemories("dm:1"); err != nil || s.MemoryCount("dm:1") != 0 {
		t.Errorf("clear: %v", err)
	}
	if err := s.ClearMemories("dm:1"); err != nil {
		t.Errorf("clear twice: %v", err)
	}
	_, _ = s.UpsertMemory(model.Memory{ChatKey: "group:x", Text: "y"})
	if err := s.ClearAllMemories(); err != nil || s.MemoryCount("group:x") != 0 {
		t.Errorf("clear all: %v", err)
	}
	_ = p
}

func TestSelfSamples(t *testing.T) {
	s, p := openTemp(t)
	if n, since := s.SelfSampleStats(); n != 0 || since != nil {
		t.Fatal("not empty")
	}
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 7; i++ {
		if err := s.AppendSelfSample(model.SelfSample{TS: base.Add(time.Duration(i) * time.Minute), Kind: "dm", Text: string(rune('a' + i))}, 5); err != nil {
			t.Fatal(err)
		}
	}
	// exact repeat (same ts + text) is skipped
	_ = s.AppendSelfSample(model.SelfSample{TS: base.Add(6 * time.Minute), Kind: "dm", Text: "g"}, 5)
	xs, _ := s.SelfSamples(0)
	if len(xs) != 5 || xs[0].Text != "c" || xs[4].Text != "g" {
		t.Fatalf("fifo: %+v", xs)
	}
	if last, _ := s.SelfSamples(2); len(last) != 2 || last[1].Text != "g" {
		t.Errorf("limit: %+v", last)
	}
	n, since := s.SelfSampleStats()
	if n != 5 || since == nil || !since.Equal(base.Add(2*time.Minute)) {
		t.Errorf("stats %d %v", n, since)
	}
	if st, err := os.Stat(p.SelfSamplesFile()); err != nil || (runtime.GOOS != "windows" && st.Mode().Perm() != 0o600) {
		t.Errorf("mode %v %v", st, err)
	}
	if err := s.ClearSelfSamples(); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.SelfSampleStats(); n != 0 {
		t.Error("not cleared")
	}
}
