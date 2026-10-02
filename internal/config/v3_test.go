package config

import (
	"encoding/json"
	"os"
	"testing"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/model"
)

// v2Profile renders a profile as a settings-v2 file had it: without the v3 fields.
func v2Profile(t *testing.T, p model.BehaviorProfile) map[string]any {
	t.Helper()
	b, _ := json.Marshal(p)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, f := range V3Fields {
		delete(m, f)
	}
	return m
}

func writeConfig(t *testing.T, v any) Paths {
	t.Helper()
	p, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	if err := os.WriteFile(p.ConfigFile(), b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestV2ConfigGetsV3FieldsFromItsPreset(t *testing.T) {
	instant, _ := behavior.Preset(behavior.PresetInstant)
	busy, _ := behavior.Preset(behavior.PresetBusy)
	priv := v2Profile(t, busy.Private)
	// Simulate a decoder that left the new fields zero (not merged into defaults).
	grp := v2Profile(t, instant.Group)
	grp["tagReplyPercent"], grp["mentionMax"], grp["allowMentions"], grp["maxStreak"] = 40, 0, false, 0
	p := writeConfig(t, map[string]any{
		"version":  2,
		"behavior": map[string]any{"triggerPrefix": "1", "private": priv, "group": grp},
	})
	m, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	s := m.Get()
	if s.Version != SettingsVersion {
		t.Errorf("version = %d", s.Version)
	}
	g := s.Behavior.Group
	if g.Preset != behavior.PresetInstant || g.TagReplyPercent != 0 || g.MentionMax != 1 || !g.AllowMentions || g.MaxStreak != 6 || g.TriggerWords == nil {
		t.Errorf("group = %+v", g)
	}
	if pr := s.Behavior.Private; pr.Preset != behavior.PresetBusy || !pr.AllowMentions || pr.RespondToAllMaxMembers != 8 {
		t.Errorf("private = %+v", pr)
	}

	// A plain v2 Natural file stays Natural with the new defaults.
	p = writeConfig(t, map[string]any{
		"version":  2,
		"behavior": map[string]any{"private": v2Profile(t, behavior.Default("dm")), "group": v2Profile(t, behavior.Default("group"))},
	})
	m, _ = Load(p)
	g = m.Get().Behavior.Group
	if g.Preset != behavior.PresetNatural || g.TagReplyPercent != 15 || !g.AllowMentions || !g.AnswerAnyoneWhoAddressesIt {
		t.Errorf("natural group = %+v", g)
	}
	raw, _ := os.ReadFile(p.ConfigFile())
	var onDisk struct{ Version int }
	_ = json.Unmarshal(raw, &onDisk)
	if onDisk.Version != SettingsVersion {
		t.Errorf("version on disk = %d", onDisk.Version)
	}

	// A v3 file is left alone (custom values survive a reload).
	if _, err := m.Update(func(s *Settings) { s.Behavior.Group.TagReplyPercent = 70 }); err != nil {
		t.Fatal(err)
	}
	m, _ = Load(p)
	if g := m.Get().Behavior.Group; g.TagReplyPercent != 70 || g.Preset != behavior.PresetCustom {
		t.Errorf("v3 reload = %d %s", g.TagReplyPercent, g.Preset)
	}
}
