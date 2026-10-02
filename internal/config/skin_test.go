package config

import "testing"

func TestSkinNormalize(t *testing.T) {
	m, err := Load(writeConfig(t, map[string]any{"version": SettingsVersion, "theme": "dark"}))
	if err != nil {
		t.Fatal(err)
	}
	if s := m.Get(); s.Skin != "glass" || s.Theme != "dark" {
		t.Fatalf("missing skin = %q (theme %q)", s.Skin, s.Theme)
	}
	m, err = Load(writeConfig(t, map[string]any{"version": SettingsVersion, "skin": "disco"}))
	if err != nil {
		t.Fatal(err)
	}
	if s := m.Get(); s.Skin != "glass" {
		t.Fatalf("unknown skin = %q", s.Skin)
	}
	if _, err := m.Update(func(s *Settings) { s.Skin = "vintage" }); err != nil {
		t.Fatal(err)
	}
	m2, err := Load(m.paths)
	if err != nil {
		t.Fatal(err)
	}
	if s := m2.Get(); s.Skin != "vintage" {
		t.Fatalf("persisted skin = %q", s.Skin)
	}
	for _, k := range Skins {
		if !ValidSkin(k) {
			t.Fatalf("ValidSkin(%q) = false", k)
		}
	}
	if ValidSkin("") || Defaults().Skin != "glass" {
		t.Fatal("empty skin valid or default not glass")
	}
}
