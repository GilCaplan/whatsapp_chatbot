package store

import (
	"os"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

const v1Chats = `[
  {"key":"dm:1","kind":"dm","jid":"1@s.whatsapp.net","name":"A","personaId":"leo","enabled":true,
   "overrides":{"debounceSeconds":4,"groupRandomReplyPercent":null,"alwaysReplyInGroup":false}},
  {"key":"group:2","kind":"group","jid":"2@g.us","name":"B","personaId":"leo","enabled":true,
   "overrides":{"debounceSeconds":null,"groupRandomReplyPercent":15,"alwaysReplyInGroup":false}},
  {"key":"group:3","kind":"group","jid":"3@g.us","name":"C","personaId":"leo","enabled":false,
   "overrides":{"debounceSeconds":null,"groupRandomReplyPercent":null,"alwaysReplyInGroup":true}},
  {"key":"dm:4","kind":"dm","jid":"4@s.whatsapp.net","name":"D","personaId":"leo","enabled":true,
   "overrides":{"debounceSeconds":null,"groupRandomReplyPercent":null,"alwaysReplyInGroup":false}}
]`

func TestOpenMigratesChatOverrides(t *testing.T) {
	_, p := openTemp(t)
	if err := os.WriteFile(p.ChatsFile(), []byte(v1Chats), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.Chat("dm:1")
	if a.Behavior.WaitForMoreSec == nil || *a.Behavior.WaitForMoreSec != 4 || a.Behavior.ChimeInPercent != nil || a.LegacyOverrides != nil {
		t.Errorf("debounce: %+v", a.Behavior)
	}
	b, _ := s.Chat("group:2")
	if b.Behavior.ChimeInPercent == nil || *b.Behavior.ChimeInPercent != 15 || b.Behavior.WaitForMoreSec != nil {
		t.Errorf("random percent: %+v", b.Behavior)
	}
	c, _ := s.Chat("group:3")
	if c.Behavior.ChimeInPercent == nil || *c.Behavior.ChimeInPercent != 100 || c.Behavior.AIJudgement == nil || *c.Behavior.AIJudgement || c.Enabled {
		t.Errorf("always reply: %+v", c.Behavior)
	}
	d, _ := s.Chat("dm:4")
	if d.Behavior != (model.BehaviorOverrides{}) {
		t.Errorf("no overrides: %+v", d.Behavior)
	}
	raw, _ := os.ReadFile(p.ChatsFile())
	if strings.Contains(string(raw), `"overrides"`) || !strings.Contains(string(raw), `"waitForMoreSec": 4`) {
		t.Errorf("chats.json not rewritten:\n%s", raw)
	}
	// Second open: nothing left to migrate, values intact.
	s2, err := Open(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a2, _ := s2.Chat("dm:1"); *a2.Behavior.WaitForMoreSec != 4 {
		t.Error("lost on reopen")
	}
}

func TestRuntimeStateRoundTrip(t *testing.T) {
	s, p := openTemp(t)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	due := now.Add(time.Hour)
	st, err := s.UpdateRunnerState("dm:1", func(st *RunnerState) {
		st.LastReplyAt = now
		st.Replies = []time.Time{now.Add(-25 * time.Hour), now.Add(-30 * time.Minute), now}
		st.QueuedWakeAt = &due
	})
	if err != nil {
		t.Fatal(err)
	}
	st.Prune(now)
	if len(st.Replies) != 2 || st.RepliesSince(now.Add(-time.Hour)) != 2 || st.RepliesSince(now) != 1 {
		t.Errorf("prune/count: %+v", st.Replies)
	}
	// Returned copies don't alias the store.
	st.QueuedWakeAt = nil
	s2, err := Open(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := s2.RunnerState("dm:1")
	if !got.LastReplyAt.Equal(now) || len(got.Replies) != 3 || got.QueuedWakeAt == nil || !got.QueuedWakeAt.Equal(due) {
		t.Errorf("round trip: %+v", got)
	}
	if z := s2.RunnerState("dm:none"); !z.LastReplyAt.IsZero() || z.QueuedWakeAt != nil {
		t.Error("unknown chat should be zero")
	}
	// Corrupt file is not fatal.
	if err := os.WriteFile(p.RuntimeFile(), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(p, nil); err != nil {
		t.Errorf("corrupt runtime.json should be ignored: %v", err)
	}
}
