package server

import (
	"net/url"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

func TestChatCrossPatch(t *testing.T) {
	e := newEnv(t)
	key := e.assignGroup(t)
	var c model.ChatAssignment
	if code := e.do("PATCH", "/api/chats/"+key, map[string]any{"cross": map[string]any{"mode": "open", "share": false}}, &c); code != 200 ||
		c.Cross.Mode != model.CrossOpen || c.Cross.Share == nil || *c.Cross.Share {
		t.Fatalf("patch = %d %+v", code, c.Cross)
	}
	// Absent fields are kept; null goes back to the default.
	if code := e.do("PATCH", "/api/chats/"+key, map[string]any{"cross": map[string]any{"share": nil}}, &c); code != 200 || c.Cross.Mode != model.CrossOpen || c.Cross.Share != nil {
		t.Fatalf("partial = %d %+v", code, c.Cross)
	}
	if code := e.do("PATCH", "/api/chats/"+key, map[string]any{"cross": nil}, &c); code != 200 || c.Cross.Mode != "" {
		t.Fatalf("reset = %d %+v", code, c.Cross)
	}
	for _, bad := range []any{map[string]any{"mode": "loud"}, map[string]any{"share": "yes"}, map[string]any{"volume": 1}, "open"} {
		var apiErr apiError
		if code := e.do("PATCH", "/api/chats/"+key, map[string]any{"cross": bad}, &apiErr); code != 400 || apiErr.Code != "invalid_cross" {
			t.Errorf("%v: %d %+v", bad, code, apiErr)
		}
	}
}

func TestCrossViewAndRefresh(t *testing.T) {
	e := newEnv(t)
	key := e.assignGroup(t)
	var v model.CrossView
	if code := e.do("GET", "/api/chats/"+url.PathEscape(key)+"/cross", nil, &v); code != 200 || v.Mode != model.CrossDiscreet || len(v.Sources) != 1 {
		t.Fatalf("view = %d %+v", code, v)
	}
	var b model.Brief
	if code := e.do("POST", "/api/chats/"+url.PathEscape(key)+"/cross/refresh", map[string]any{}, &b); code != 200 || b.ChatKey != key {
		t.Fatalf("refresh = %d %+v", code, b)
	}
	if code := e.do("GET", "/api/chats/nope/cross", nil, nil); code != 404 {
		t.Errorf("unknown chat = %d", code)
	}
}

func TestPeopleCrossFields(t *testing.T) {
	e := newEnv(t)
	key := e.assignGroup(t)
	r := roster(4)
	e.wa.mu.Lock()
	e.wa.rosters = map[string][]model.Participant{friendsJID: r}
	e.wa.mu.Unlock()
	dana := r[1]
	now := time.Now()
	if _, err := e.st.UpsertChat(model.ChatAssignment{Key: "dm:" + dana.Phone, Kind: "dm", JID: dana.Phone + "@s.whatsapp.net", Name: "Dana", PersonaID: "leo", Enabled: true, LastActivityAt: &now}); err != nil {
		t.Fatal(err)
	}
	var v peopleView
	e.do("GET", "/api/chats/"+key+"/people", nil, &v)
	var dv, other personView
	for _, m := range v.Members {
		switch m.Name {
		case "Dana":
			dv = m
		case "Eli":
			other = m
		}
	}
	if !dv.Cross || dv.CrossSource != "default" || dv.DMChatKey != "dm:"+dana.Phone || !dv.DMShares {
		t.Errorf("dana = %+v", dv)
	}
	if other.DMChatKey != "" {
		t.Errorf("eli has no private chat: %+v", other)
	}
	// Per person: off, then back to the default.
	if code := e.do("PATCH", "/api/chats/"+key, map[string]any{"people": map[string]any{"people": []any{map[string]any{"jid": dana.JID, "cross": false}}}}, nil); code != 200 {
		t.Fatalf("patch = %d", code)
	}
	e.do("GET", "/api/chats/"+key+"/people", nil, &v)
	for _, m := range v.Members {
		if m.Name == "Dana" && (m.Cross || m.CrossSource != "person") {
			t.Errorf("after patch = %+v", m)
		}
	}
	var apiErr apiError
	if code := e.do("PATCH", "/api/chats/"+key, map[string]any{"people": map[string]any{"people": []any{map[string]any{"jid": dana.JID, "cross": "no"}}}}, &apiErr); code != 400 {
		t.Errorf("bad cross = %d", code)
	}
	e.do("PATCH", "/api/chats/"+key, map[string]any{"people": map[string]any{"people": []any{map[string]any{"jid": dana.JID, "cross": nil}}}}, nil)
	e.do("GET", "/api/chats/"+key+"/people", nil, &v)
	for _, m := range v.Members {
		if m.Name == "Dana" && (!m.Cross || m.CrossSource != "default") {
			t.Errorf("after reset = %+v", m)
		}
	}
}

func TestMemoryScopeAndShares(t *testing.T) {
	e := newEnv(t)
	key := e.assignGroup(t)
	if _, err := e.st.UpsertChat(model.ChatAssignment{Key: "dm:15550100", Kind: "dm", JID: "15550100@s.whatsapp.net", Name: "Dana", PersonaID: "leo", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	var plain, sick model.Memory
	e.do("POST", "/api/chats/dm:15550100/memories", map[string]any{"text": "hates cilantro", "person": "Dana"}, &plain)
	e.do("POST", "/api/chats/dm:15550100/memories", map[string]any{"text": "is in the hospital this week", "person": "Dana"}, &sick)
	if sick.Sensitive != model.HandoffHealth {
		t.Errorf("classified on add: %+v", sick)
	}
	var v model.MemoriesView
	e.do("GET", "/api/chats/dm:15550100/memories", nil, &v)
	shares := map[string]model.MemoryItem{}
	for _, it := range v.Items {
		shares[it.ID] = it
	}
	if !v.CrossEnabled || !shares[plain.ID].Shares || shares[sick.ID].Shares || shares[sick.ID].EffectiveSensitive != model.HandoffHealth {
		t.Fatalf("view = %+v", v)
	}
	var m model.Memory
	if code := e.do("PATCH", "/api/chats/dm:15550100/memories/"+sick.ID, map[string]any{"scope": "shared"}, &m); code != 200 || m.Scope != model.MemoryScopeShared {
		t.Fatalf("unlock = %d %+v", code, m)
	}
	if code := e.do("PATCH", "/api/chats/dm:15550100/memories/"+plain.ID, map[string]any{"scope": "local"}, &m); code != 200 || m.Scope != model.MemoryScopeLocal {
		t.Fatalf("lock = %d %+v", code, m)
	}
	e.do("GET", "/api/chats/dm:15550100/memories", nil, &v)
	for _, it := range v.Items {
		if (it.ID == sick.ID) != it.Shares {
			t.Errorf("after lock/unlock: %+v", it)
		}
	}
	var m2 model.Memory // fresh: omitempty fields don't overwrite
	if code := e.do("PATCH", "/api/chats/dm:15550100/memories/"+plain.ID, map[string]any{"scope": nil}, &m2); code != 200 || m2.Scope != "" {
		t.Errorf("follow settings = %d %+v", code, m2)
	}
	for _, bad := range []map[string]any{{"scope": "public"}, {"scope": ""}, {"sensitive": "gossip"}} {
		var apiErr apiError
		if code := e.do("PATCH", "/api/chats/dm:15550100/memories/"+plain.ID, bad, &apiErr); code != 400 || apiErr.Code != "invalid_memory" {
			t.Errorf("%v = %d %+v", bad, code, apiErr)
		}
	}
	// Sharing off for the chat: nothing shares.
	e.do("PATCH", "/api/chats/dm:15550100", map[string]any{"cross": map[string]any{"share": false}}, nil)
	e.do("GET", "/api/chats/dm:15550100/memories", nil, &v)
	if v.CrossEnabled || v.Items[0].Shares {
		t.Errorf("share off = %+v", v)
	}
	_ = key
}

func TestSettingsCrossValidation(t *testing.T) {
	e := newEnv(t)
	for _, bad := range []map[string]any{
		{"groupMode": "loud"}, {"dmMode": "x"}, {"freshDays": 0}, {"freshDays": 91}, {"maxPeople": 9}, {"maxItems": 17},
	} {
		var apiErr apiError
		if code := e.do("PUT", "/api/settings", map[string]any{"memory": map[string]any{"cross": bad}}, &apiErr); code != 400 {
			t.Errorf("%v = %d", bad, code)
		}
	}
	var sv settingsView
	if code := e.do("PUT", "/api/settings", map[string]any{"memory": map[string]any{"cross": map[string]any{"groupMode": "open", "sensitive": map[string]any{"romance": false}}}}, &sv); code != 200 ||
		sv.Memory.Cross.GroupMode != "open" || sv.Memory.Cross.Sensitive.Romance || !sv.Memory.Cross.Sensitive.Secret || sv.Memory.Cross.MaxItems != 8 {
		t.Errorf("put = %d %+v", code, sv.Memory.Cross)
	}
}

func TestPlaygroundCrossParams(t *testing.T) {
	e := newEnv(t)
	fp := &fakePlayground{}
	e.s.d.Playground = fp
	if code := e.do("POST", "/api/playground/s1/messages", map[string]any{"text": "hi", "group": true, "source": "dm:15550100", "crossMode": "open"}, nil); code != 200 {
		t.Fatalf("send = %d", code)
	}
	var apiErr apiError
	if code := e.do("POST", "/api/playground/s1/messages", map[string]any{"text": "hi", "crossMode": "loud"}, &apiErr); code != 400 || apiErr.Code != "invalid_cross" {
		t.Errorf("bad mode = %d %+v", code, apiErr)
	}
	if len(fp.cross) != 1 || fp.cross[0] != "dm:15550100|open" {
		t.Errorf("cross = %v", fp.cross)
	}
}
