package config

import (
	"encoding/json"
	"testing"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/model"
)

// v3Profile renders a profile as a settings-v3 file had it: without the v4 fields.
func v3Profile(t *testing.T, p model.BehaviorProfile) map[string]any {
	t.Helper()
	b, _ := json.Marshal(p)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, f := range V4Fields {
		delete(m, f)
	}
	return m
}

func TestV3ConfigGetsV4FieldsAndBlocks(t *testing.T) {
	instant, _ := behavior.Preset(behavior.PresetInstant)
	p := writeConfig(t, map[string]any{
		"version": 3,
		"behavior": map[string]any{"triggerPrefix": "1",
			"private": v3Profile(t, behavior.Default(behavior.KindDM)),
			"group":   v3Profile(t, instant.Group)},
		// A v3 file never had these blocks; zero values must not stick.
		"notifications": map[string]any{"enabled": false},
	})
	m, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	s := m.Get()
	if s.Version != 4 {
		t.Errorf("version = %d", s.Version)
	}
	if pr := s.Behavior.Private; pr.Preset != behavior.PresetNatural || pr.TypoPercent != 5 || pr.TypoFixStyle != "correction" {
		t.Errorf("private = %s %d %q", pr.Preset, pr.TypoPercent, pr.TypoFixStyle)
	}
	if g := s.Behavior.Group; g.Preset != behavior.PresetInstant || g.TypoPercent != 0 {
		t.Errorf("group = %s %d", g.Preset, g.TypoPercent)
	}
	d := Defaults()
	if s.Notifications != d.Notifications || s.Safety != d.Safety || s.Memory != d.Memory || s.Recap != d.Recap || s.SelfClone != d.SelfClone {
		t.Errorf("v4 blocks not defaulted: %+v %+v %+v %+v %+v", s.Notifications, s.Safety, s.Memory, s.Recap, s.SelfClone)
	}

	// A v4 file keeps its choices (including false bools) across reloads.
	if _, err := m.Update(func(s *Settings) {
		s.Notifications.Enabled = false
		s.Safety.Handoff.Bot = false
		s.Memory.Enabled = false
		s.Recap = RecapSettings{Enabled: true, Time: "08:30", KeepDays: 7}
		s.SelfClone.CollectSamples = true
	}); err != nil {
		t.Fatal(err)
	}
	m, err = Load(p)
	if err != nil {
		t.Fatal(err)
	}
	s = m.Get()
	if s.Notifications.Enabled || s.Safety.Handoff.Bot || !s.Safety.Handoff.Money || s.Memory.Enabled ||
		s.Recap.Time != "08:30" || !s.SelfClone.CollectSamples || s.SelfClone.MaxSamples != 500 {
		t.Errorf("v4 reload = %+v %+v %+v %+v %+v", s.Notifications, s.Safety, s.Memory, s.Recap, s.SelfClone)
	}
	if s.Safety.Reveal.Template != DefaultRevealTemplate {
		t.Errorf("template = %q", s.Safety.Reveal.Template)
	}
}
