package store

import (
	"errors"
	"testing"

	"whatsappdoppel/internal/core/config"
	"whatsappdoppel/internal/core/model"
)

func openTemp(t *testing.T) (*Store, config.Paths) {
	t.Helper()
	p, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, []model.Persona{{ID: "leo", Name: "Leo", BuiltIn: true}})
	if err != nil {
		t.Fatal(err)
	}
	return s, p
}

func TestSeedAndPersonaCRUD(t *testing.T) {
	s, p := openTemp(t)
	if got := len(s.Personas()); got != 1 {
		t.Fatalf("seeded personas = %d, want 1", got)
	}
	a, err := s.UpsertPersona(model.Persona{Name: "Leo"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != "leo-2" {
		t.Fatalf("id = %q, want leo-2", a.ID)
	}
	// Reopening must not re-seed and must keep both.
	s2, err := Open(p, []model.Persona{{ID: "other", Name: "Other"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(s2.Personas()); got != 2 {
		t.Fatalf("after reopen = %d, want 2", got)
	}
	if err := s2.DeletePersona("leo-2"); err != nil {
		t.Fatal(err)
	}
	if err := s2.DeletePersona("leo-2"); err != ErrNotFound {
		t.Fatalf("second delete err = %v", err)
	}
}

func TestChatsAndApprovals(t *testing.T) {
	s, _ := openTemp(t)
	if _, err := s.UpsertChat(model.ChatAssignment{Key: "dm:1", JID: "1@s.whatsapp.net", AltJID: "9@lid", PersonaID: "leo"}); err != nil {
		t.Fatal(err)
	}
	if c, ok := s.ChatByJID("9@lid"); !ok || c.Key != "dm:1" {
		t.Fatalf("ChatByJID alt failed: %+v %v", c, ok)
	}
	c, err := s.UpdateChat("dm:1", func(c *model.ChatAssignment) { c.Enabled = true; c.Key = "hijack" })
	if err != nil || !c.Enabled || c.Key != "dm:1" {
		t.Fatalf("UpdateChat = %+v, %v", c, err)
	}
	if err := s.UpsertApproval(model.PendingReply{ID: "a", Text: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Approval("a"); !ok {
		t.Fatal("approval missing")
	}
	if err := s.DeleteApproval("a"); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryCompaction(t *testing.T) {
	s, _ := openTemp(t)
	for i := 0; i < 26; i++ {
		if err := s.AppendHistory("group:123", model.Message{Speaker: "them", Text: "hi"}, 5); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := s.History("group:123", 0)
	if len(all) > 25 || len(all) < 5 {
		t.Fatalf("history len = %d, want compacted to <= 25", len(all))
	}
	last2, _ := s.History("group:123", 2)
	if len(last2) != 2 {
		t.Fatalf("limit 2 = %d", len(last2))
	}
	if err := s.ClearHistory("group:123"); err != nil {
		t.Fatal(err)
	}
	if n := s.HistoryCount("group:123"); n != 0 {
		t.Fatalf("after clear = %d", n)
	}
}

func TestTaskRunSwapAndIndex(t *testing.T) {
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	run := model.TaskRun{ID: "r1", ChatKey: "dm:1", Status: model.TaskActive}
	if err := s.UpdateTaskRuns(func(all []model.TaskRun) ([]model.TaskRun, error) { return append(all, run), nil }); err != nil {
		t.Fatal(err)
	}
	if id, ok := s.ActiveTaskID("dm:1"); !ok || id != "r1" {
		t.Fatalf("index = %q %v", id, ok)
	}
	stale, _ := s.TaskRun("r1")
	if _, _, err := s.UpdateTaskRun("r1", func(r *model.TaskRun) { r.Status = model.TaskStopped }); err != nil {
		t.Fatal(err)
	}
	stale.Sent = 3
	if _, err := s.SwapTaskRun(stale, stale.Rev); !errors.Is(err, ErrTaskConflict) {
		t.Fatalf("a stale write is refused, err = %v", err)
	}
	if r, _ := s.TaskRun("r1"); r.Status != model.TaskStopped || r.Sent != 0 {
		t.Fatalf("the stop stays, run = %+v", r)
	}
	if _, ok := s.ActiveTaskID("dm:1"); ok {
		t.Fatal("a stopped run leaves the index")
	}
}
