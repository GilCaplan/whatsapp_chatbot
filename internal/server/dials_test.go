package server

import (
	"strings"
	"testing"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/model"
)

func TestBehaviorDialsInResponses(t *testing.T) {
	e := newEnv(t)
	var presets struct {
		Dials     map[string]behavior.DialDef `json:"dials"`
		DialOrder []string                    `json:"dialOrder"`
	}
	if code := e.do("GET", "/api/behavior/presets", nil, &presets); code != 200 {
		t.Fatalf("presets = %d", code)
	}
	if len(presets.DialOrder) != 3 || len(presets.Dials) != 3 || len(presets.Dials["speed"].Levels) != 5 ||
		presets.Dials["boldness"].Levels[2].Group["typoPercent"] == nil {
		t.Fatalf("dials = %+v %v", presets.Dials, presets.DialOrder)
	}

	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo"}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	key := strings.ReplaceAll(c.Key, ":", "%3A")
	var view struct {
		Dials map[string]behavior.DialPos `json:"dials"`
	}
	e.do("GET", "/api/chats/"+key+"/behavior", nil, &view)
	if p := view.Dials["speed"]; p.Level != 3 || !p.Exact {
		t.Fatalf("default dials = %+v", view.Dials)
	}
	// Moving a dial = PATCHing its fields; the view reads it back.
	patch := behavior.DialValues("speed", "dm", 5)
	e.do("PATCH", "/api/chats/"+key, map[string]any{"behavior": patch}, &c)
	e.do("GET", "/api/chats/"+key+"/behavior", nil, &view)
	if p := view.Dials["speed"]; p.Level != 5 || !p.Exact {
		t.Errorf("after speed 5 = %+v", p)
	}
	e.do("PATCH", "/api/chats/"+key, map[string]any{"behavior": map[string]any{"noticeMaxSec": 2}}, &c)
	e.do("GET", "/api/chats/"+key+"/behavior", nil, &view)
	if p := view.Dials["speed"]; p.Level != 5 || p.Exact {
		t.Errorf("tuned = %+v", p)
	}
}
