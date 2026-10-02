package server

import (
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

// Wave 3 realism (Engineer A): memory endpoints, Clone yourself, the
// "persona" active-hours zone.

func TestMemoriesCRUD(t *testing.T) {
	e := newEnv(t)
	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo"}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	key := strings.ReplaceAll(c.Key, ":", "%3A")
	base := "/api/chats/" + key + "/memories"
	ch, _, cancel := e.hub.Subscribe(-1)
	defer cancel()

	var m model.Memory
	if code := e.do("POST", base, map[string]any{"text": "  has a dog\ncalled Pita. ", "person": "Dana"}, &m); code != 200 {
		t.Fatalf("add = %d", code)
	}
	if m.ID == "" || m.Text != "has a dog called Pita" || m.Source != model.MemorySourceUser || m.Person != "Dana" || m.ChatKey != c.Key {
		t.Fatalf("added = %+v", m)
	}
	got := false
	for !got {
		select {
		case ev := <-ch:
			got = ev.Type == "memories.changed"
		case <-time.After(time.Second):
			t.Fatal("no memories.changed event")
		}
	}
	var apiErr apiError
	for _, bad := range []string{"", "   ", strings.Repeat("x", 161)} {
		if code := e.do("POST", base, map[string]any{"text": bad}, &apiErr); code != 400 || apiErr.Code != "invalid_memory" {
			t.Errorf("add %q = %d %+v", bad, code, apiErr)
		}
	}
	var p model.Memory
	if code := e.do("PATCH", base+"/"+m.ID, map[string]any{"pinned": true, "text": "has a dog called Pita (a beagle)"}, &p); code != 200 || !p.Pinned || p.Text != "has a dog called Pita (a beagle)" {
		t.Fatalf("patch = %d %+v", code, p)
	}
	if code := e.do("PATCH", base+"/nope", map[string]any{"pinned": true}, &apiErr); code != 404 {
		t.Errorf("patch unknown = %d", code)
	}
	if code := e.do("PATCH", base+"/"+m.ID, map[string]any{"text": ""}, &apiErr); code != 400 {
		t.Errorf("patch empty text = %d", code)
	}
	e.do("POST", base, map[string]any{"text": "supports Arsenal", "person": "Josh"}, nil)
	var v model.MemoriesView
	if code := e.do("GET", base, nil, &v); code != 200 || len(v.Items) != 2 || v.Items[0].ID != m.ID || !v.Enabled {
		t.Fatalf("list = %d %+v", code, v)
	}
	if code := e.do("DELETE", base+"/"+m.ID, nil, nil); code != 200 {
		t.Errorf("delete = %d", code)
	}
	if code := e.do("DELETE", base+"/"+m.ID, nil, &apiErr); code != 404 {
		t.Errorf("delete again = %d", code)
	}
	var res model.MemoryExtractResult
	if code := e.do("POST", base+"/extract", nil, &res); code != 200 || res.Added != 1 {
		t.Errorf("extract = %d %+v", code, res)
	}
	if code := e.do("DELETE", base, nil, nil); code != 200 || e.st.MemoryCount(c.Key) != 0 {
		t.Errorf("clear = %d (%d left)", code, e.st.MemoryCount(c.Key))
	}
	if code := e.do("GET", "/api/chats/dm%3Anope/memories", nil, nil); code != 404 {
		t.Errorf("unknown chat = %d", code)
	}
}

func TestCloneEndpoints(t *testing.T) {
	e := newEnv(t)
	var apiErr apiError
	if code := e.do("POST", "/api/clone/draft", map[string]any{"name": "Me"}, &apiErr); code != 400 || apiErr.Code != "too_few_samples" {
		t.Fatalf("no samples = %d %+v", code, apiErr)
	}
	base := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 22; i++ {
		_ = e.st.AppendSelfSample(model.SelfSample{TS: base.Add(time.Duration(i) * time.Minute), Kind: "dm", Text: "msg " + string(rune('a'+i))}, 500)
	}
	var cs model.CloneSamplesView
	if code := e.do("GET", "/api/clone/samples", nil, &cs); code != 200 || cs.Count != 22 || len(cs.Preview) != clonePreview || cs.Since == nil {
		t.Fatalf("samples = %d %+v", code, cs)
	}
	// Enough samples, but this env has no builder.
	if code := e.do("POST", "/api/clone/draft", map[string]any{"name": "Me"}, nil); code != 503 {
		t.Errorf("draft without builder = %d", code)
	}
	if code := e.do("DELETE", "/api/clone/samples", nil, nil); code != 200 {
		t.Fatal("delete samples")
	}
	if n, _ := e.st.SelfSampleStats(); n != 0 {
		t.Error("samples not deleted")
	}
}

func TestBehaviorPersonaZone(t *testing.T) {
	e := newEnv(t)
	p, _ := e.st.Persona("leo")
	p.World = model.World{Timezone: "Asia/Tokyo"}
	if _, err := e.st.UpsertPersona(p); err != nil {
		t.Fatal(err)
	}
	var c model.ChatAssignment
	e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo"}, &c)
	key := strings.ReplaceAll(c.Key, ":", "%3A")
	week := []map[string]any{}
	for _, d := range []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"} {
		week = append(week, map[string]any{"day": d, "ranges": []map[string]string{{"from": "00:00", "to": "24:00"}}})
	}
	av := map[string]any{"enabled": true, "timezone": "persona", "week": week, "outsideHours": "queue", "catchUpMaxMin": 5}
	var apiErr apiError
	if code := e.do("PATCH", "/api/chats/"+key, map[string]any{"behavior": map[string]any{"availability": av}}, &apiErr); code != 200 {
		t.Fatalf("patch persona zone = %d %+v", code, apiErr)
	}
	var v chatBehaviorView
	if code := e.do("GET", "/api/chats/"+key+"/behavior", nil, &v); code != 200 || v.Effective.Availability.Timezone != "persona" || !v.Available {
		t.Fatalf("view = %d tz=%q available=%v", code, v.Effective.Availability.Timezone, v.Available)
	}
	av["timezone"] = "Mars/Base"
	if code := e.do("PATCH", "/api/chats/"+key, map[string]any{"behavior": map[string]any{"availability": av}}, &apiErr); code != 400 {
		t.Errorf("bad zone = %d", code)
	}
}
