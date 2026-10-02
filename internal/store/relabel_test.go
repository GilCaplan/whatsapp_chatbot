package store

import (
	"encoding/json"
	"os"
	"testing"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/config"
)

// A pre-v4 chat that applied the whole old Busy preset keeps its label.
func TestChatOldPresetRelabelled(t *testing.T) {
	_, p := openTemp(t)
	old, _ := behavior.LegacyPreset(behavior.PresetBusy, behavior.KindDM)
	b, _ := json.Marshal(old)
	var ov map[string]any
	_ = json.Unmarshal(b, &ov)
	for _, f := range config.V4Fields {
		delete(ov, f) // written before v4
	}
	ov["preset"] = behavior.PresetBusy
	chats := []map[string]any{{"key": "dm:1", "kind": "dm", "jid": "1@s.whatsapp.net", "personaId": "leo", "enabled": true, "behavior": ov}}
	raw, _ := json.Marshal(chats)
	if err := os.WriteFile(p.ChatsFile(), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := s.Chat("dm:1")
	if c.Behavior.ReactPercent == nil || *c.Behavior.ReactPercent != 0 {
		t.Fatalf("react = %v", c.Behavior.ReactPercent)
	}
	eff := behavior.Resolve(behavior.Default(behavior.KindDM), c)
	if eff.Profile.Preset != behavior.PresetBusy {
		t.Errorf("label = %s", eff.Profile.Preset)
	}
}
