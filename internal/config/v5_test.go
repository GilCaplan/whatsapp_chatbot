package config

import (
	"testing"

	"whatsappdoppel/internal/model"
)

func TestV4ConfigGetsCrossDefaults(t *testing.T) {
	p := writeConfig(t, map[string]any{
		"version": 4,
		// A v4 file never had memory.cross: zero values must not stick.
		"memory": map[string]any{"enabled": false},
	})
	m, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	s := m.Get()
	if s.Version != 5 || SettingsVersion != 5 {
		t.Errorf("version = %d", s.Version)
	}
	if s.Memory.Enabled {
		t.Error("memory.enabled=false lost")
	}
	if s.Memory.Cross != Defaults().Memory.Cross {
		t.Errorf("cross not defaulted: %+v", s.Memory.Cross)
	}
	c := s.Memory.Cross
	if !c.Enabled || c.GroupMode != model.CrossDiscreet || c.DMMode != model.CrossOpen || !c.Sensitive.Romance || !c.Sensitive.Secret ||
		c.FreshDays != 14 || c.MaxPeople != 4 || c.MaxItems != 8 {
		t.Errorf("defaults: %+v", c)
	}

	// v5 keeps its choices (false bools too) across reloads.
	if _, err := m.Update(func(s *Settings) {
		s.Memory.Cross.Enabled = false
		s.Memory.Cross.Sensitive.Health = false
		s.Memory.Cross.GroupMode = model.CrossOpen
		s.Memory.Cross.FreshDays = 0 // partial PUT: filled back
	}); err != nil {
		t.Fatal(err)
	}
	m2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	c = m2.Get().Memory.Cross
	if c.Enabled || c.Sensitive.Health || !c.Sensitive.Money || c.GroupMode != model.CrossOpen || c.FreshDays != 14 {
		t.Errorf("reload: %+v", c)
	}
	r := c.Rules()
	if r.Enabled || r.Sensitive[model.HandoffHealth] || !r.Sensitive[model.SensitiveSecret] || r.GroupMode != model.CrossOpen {
		t.Errorf("rules: %+v", r)
	}
}
