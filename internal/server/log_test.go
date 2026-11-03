package server

import (
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

func TestChatLog(t *testing.T) {
	e := newEnv(t)
	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo"}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	base := "/api/chats/" + strings.ReplaceAll(c.Key, ":", "%3A") + "/log"
	now := time.Now()
	for i, id := range []string{"a", "b", "c"} {
		at := now.Add(time.Duration(i) * time.Minute)
		_ = e.st.UpdateDecisions(c.Key, func(all []model.LogEntry) ([]model.LogEntry, bool) {
			return append(all, model.LogEntry{ID: id, ChatKey: c.Key, At: at, Outcome: model.DecisionSkipped}), true
		})
	}
	var v struct{ Items []model.LogEntry }
	if code := e.do("GET", base+"?limit=2", nil, &v); code != 200 || len(v.Items) != 2 || v.Items[0].ID != "c" {
		t.Fatalf("list = %d %+v", code, v)
	}
	if code := e.do("DELETE", base, nil, nil); code != 200 {
		t.Fatalf("clear = %d", code)
	}
	if code := e.do("GET", base, nil, &v); code != 200 || len(v.Items) != 0 {
		t.Fatalf("after clear = %d %+v", code, v)
	}
	if code := e.do("GET", "/api/chats/dm%3A1/log", nil, nil); code != 404 {
		t.Errorf("unknown chat = %d", code)
	}
}

func TestNicknamesAPI(t *testing.T) {
	e := newEnv(t)
	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo"}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	var apiErr apiError
	if code := e.do("PUT", "/api/nicknames", map[string]any{"ids": []string{"15550001@s.whatsapp.net"}, "name": "@x"}, &apiErr); code != 400 || apiErr.Code != "invalid_nickname" {
		t.Errorf("bad nickname = %d %+v", code, apiErr)
	}
	var nk model.Nickname
	if code := e.do("PUT", "/api/nicknames", map[string]any{"ids": []string{"15550001@s.whatsapp.net"}, "name": "Seva", "original": "Vsevolod"}, &nk); code != 200 || nk.Name != "Seva" {
		t.Fatalf("set = %d %+v", code, nk)
	}
	var list struct{ Items []model.Nickname }
	if code := e.do("GET", "/api/nicknames", nil, &list); code != 200 || len(list.Items) != 1 {
		t.Fatalf("list = %d %+v", code, list)
	}
	var people struct {
		Members []struct{ Name, Nickname string }
	}
	if code := e.do("GET", "/api/chats/"+strings.ReplaceAll(c.Key, ":", "%3A")+"/people", nil, &people); code != 200 || people.Members[0].Nickname != "Seva" {
		t.Errorf("people = %d %+v", code, people)
	}
	var chats []struct{ Name, WAName string }
	if e.do("GET", "/api/chats", nil, &chats); len(chats) != 1 || chats[0].Name != "Seva" || chats[0].WAName == "" {
		t.Errorf("chats = %+v", chats)
	}
}

func TestLanguageSettings(t *testing.T) {
	e := newEnv(t)
	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo"}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	key := "/api/chats/" + strings.ReplaceAll(c.Key, ":", "%3A")
	var apiErr apiError
	if code := e.do("PUT", "/api/settings", map[string]any{"behavior": map[string]any{"languages": map[string]any{"group": map[string]any{"languages": []string{"xx"}}}}}, &apiErr); code != 400 {
		t.Errorf("bad settings language = %d", code)
	}
	if code := e.do("PUT", "/api/settings", map[string]any{"behavior": map[string]any{"languages": map[string]any{"group": map[string]any{"languages": []string{"he"}, "reply": "he"}}}}, nil); code != 200 {
		t.Errorf("settings language = %d", code)
	}
	if code := e.do("PATCH", key, map[string]any{"language": map[string]any{"reply": "nope"}}, &apiErr); code != 400 || apiErr.Code != "invalid_language" {
		t.Errorf("bad chat language = %d %+v", code, apiErr)
	}
	var got model.ChatAssignment
	if code := e.do("PATCH", key, map[string]any{"language": map[string]any{"languages": []string{"en", "ru"}, "others": "answer"},
		"people": map[string]any{"people": []map[string]any{{"jid": "15550001@s.whatsapp.net", "language": "ru"}}}}, &got); code != 200 ||
		got.Language == nil || got.Language.Others != "answer" || got.People.People[0].Language != "ru" {
		t.Fatalf("chat language = %d %+v", code, got)
	}
	if code := e.do("PATCH", key, map[string]any{"people": map[string]any{"people": []map[string]any{{"jid": "15550001@s.whatsapp.net", "language": "xx"}}}}, &apiErr); code != 400 {
		t.Errorf("bad person language = %d", code)
	}
	var reset model.ChatAssignment
	if code := e.do("PATCH", key, map[string]any{"language": nil}, &reset); code != 200 || reset.Language != nil {
		t.Errorf("reset = %d %+v", code, reset.Language)
	}
}
