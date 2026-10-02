package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"whatsappdoppel/internal/behavior"
)

const v1Config = `{
  "version": 1,
  "port": 7790,
  "theme": "dark",
  "onboardingCompleted": true,
  "llm": {"defaultProvider": "ollama", "ollamaModel": "llama3.1:8b", "temperature": 0.5},
  "replies": {
    "debounceSeconds": 5,
    "burstCapSeconds": 20,
    "typingIndicator": false,
    "ignoreLinks": false,
    "groupRandomReplyPercent": 70,
    "triggerPrefix": "!!",
    "maxHistoryMessages": 25,
    "maxHistoryChars": 9000,
    "injectionFilter": "strict"
  },
  "approvals": {"autoSendSeconds": 45}
}`

func TestLoadMigratesLegacyConfig(t *testing.T) {
	p, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ConfigFile(), []byte(v1Config), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	s := m.Get()
	if s.Version != SettingsVersion || s.Port != 7790 || s.Theme != "dark" || !s.OnboardingCompleted || s.LLM.Temperature != 0.5 {
		t.Errorf("unrelated settings changed: %+v", s)
	}
	b := s.Behavior
	if b.TriggerPrefix != "!!" {
		t.Errorf("trigger = %q", b.TriggerPrefix)
	}
	for _, pr := range []struct {
		name string
		v    int
		want int
	}{
		{"private.waitForMoreSec", b.Private.WaitForMoreSec, 5},
		{"group.waitForMoreSec", b.Group.WaitForMoreSec, 5},
		{"private.burstCapSec", b.Private.BurstCapSec, 20},
		{"group.chimeInPercent", b.Group.ChimeInPercent, 70},
		{"private.historyMessages", b.Private.HistoryMessages, 25},
		{"group.historyChars", b.Group.HistoryChars, 9000},
		{"private.autoSendSeconds", b.Private.AutoSendSeconds, 45},
		{"group.autoSendSeconds", b.Group.AutoSendSeconds, 45},
	} {
		if pr.v != pr.want {
			t.Errorf("%s = %d, want %d", pr.name, pr.v, pr.want)
		}
	}
	if b.Private.TypingIndicator || b.Group.IgnoreLinks || b.Private.InjectionFilter != "strict" ||
		b.Private.Preset != behavior.PresetCustom || b.Group.Preset != behavior.PresetCustom {
		t.Errorf("behavior = %+v", b)
	}
	if s.Replies != nil || s.Approvals != nil {
		t.Error("legacy blocks should be dropped")
	}
	raw, _ := os.ReadFile(p.ConfigFile())
	var top map[string]json.RawMessage
	_ = json.Unmarshal(raw, &top)
	if _, ok := top["replies"]; ok || top["approvals"] != nil || !strings.Contains(string(raw), `"version": 5`) {
		t.Errorf("config.json not rewritten:\n%s", raw)
	}
	// Re-loading the migrated file is a no-op.
	m2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := mustJSON(t, m.Get()), mustJSON(t, m2.Get()); a != b {
		t.Errorf("second load changed settings:\n%s\n%s", a, b)
	}
}

func TestDefaultsAreNatural(t *testing.T) {
	p, _ := NewPaths(t.TempDir())
	m, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	s := m.Get()
	if s.Behavior.Private.Preset != behavior.PresetNatural || s.Behavior.Group.Preset != behavior.PresetNatural || s.Behavior.TriggerPrefix != "1" {
		t.Errorf("defaults = %+v", s.Behavior)
	}
	if !behavior.Equal(s.Behavior.For("group"), behavior.Default("group")) {
		t.Error("group default")
	}
}

func TestNormalizeClampsBehavior(t *testing.T) {
	p, _ := NewPaths(t.TempDir())
	m, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	s, err := m.Update(func(s *Settings) {
		s.Behavior.Private.ReplyPercent = 500
		s.Behavior.Private.NoticeMinSec, s.Behavior.Private.NoticeMaxSec = 40, 5
		s.Behavior.Group.LengthBias = ""
		s.Behavior.Group.Availability.Week = nil
	})
	if err != nil {
		t.Fatal(err)
	}
	pr, g := s.Behavior.Private, s.Behavior.Group
	if pr.ReplyPercent != 100 || pr.NoticeMinSec != 5 || pr.NoticeMaxSec != 40 || pr.Preset != behavior.PresetCustom {
		t.Errorf("private = %+v", pr)
	}
	if g.LengthBias != "normal" || len(g.Availability.Week) != 7 || g.Preset != behavior.PresetCustom {
		t.Errorf("group = %+v", g)
	}
	// Get returns copies: mutating one must not leak into the manager.
	got := m.Get()
	got.Behavior.Group.Availability.Week[0].Ranges = nil
	if len(m.Get().Behavior.Group.Availability.Week[0].Ranges) == 0 {
		t.Error("Get must deep-copy")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
