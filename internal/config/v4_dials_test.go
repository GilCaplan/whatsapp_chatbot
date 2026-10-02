package config

import (
	"testing"

	"whatsappdoppel/internal/behavior"
)

// v4 tweaked Busy/Slow so presets land on the vibe dials: a v3 file holding
// the whole old preset keeps its label (moved to the new numbers once); a
// customised old preset stays as it is and reads as custom.
func TestV3OldPresetsRelabelledOnce(t *testing.T) {
	slowDM, _ := behavior.LegacyPreset(behavior.PresetSlow, behavior.KindDM)
	busyGroup, _ := behavior.LegacyPreset(behavior.PresetBusy, behavior.KindGroup)
	slowDM.Preset, busyGroup.Preset = behavior.PresetSlow, behavior.PresetBusy
	busyGroup.SplitPercent = 60 // customised
	p := writeConfig(t, map[string]any{
		"version": 3,
		"behavior": map[string]any{"triggerPrefix": "1",
			"private": v3Profile(t, slowDM),
			"group":   v3Profile(t, busyGroup)},
	})
	m, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	s := m.Get()
	slow, _ := behavior.Preset(behavior.PresetSlow)
	if pr := s.Behavior.Private; pr.Preset != behavior.PresetSlow || !behavior.Equal(pr, slow.Private) || pr.DistractedPercent != 35 {
		t.Errorf("private = %s distracted %d", pr.Preset, pr.DistractedPercent)
	}
	if g := s.Behavior.Group; g.Preset != behavior.PresetCustom || g.ReactPercent != 25 || g.SplitPercent != 60 {
		t.Errorf("group = %s react %d split %d", g.Preset, g.ReactPercent, g.SplitPercent)
	}

	// v4 files are never relabelled: old numbers chosen on purpose stay.
	if _, err := m.Update(func(s *Settings) {
		s.Behavior.Private = slowDM
	}); err != nil {
		t.Fatal(err)
	}
	m, err = Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if pr := m.Get().Behavior.Private; pr.DistractedPercent != 25 || pr.Preset != behavior.PresetCustom {
		t.Errorf("v4 reload = %s distracted %d", pr.Preset, pr.DistractedPercent)
	}
}
