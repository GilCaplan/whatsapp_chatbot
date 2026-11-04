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

func TestPromptEditsPatch(t *testing.T) {
	e := newEnv(t)
	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo"}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	key := "/api/chats/" + strings.ReplaceAll(c.Key, ":", "%3A")
	var apiErr apiError
	if code := e.do("PATCH", key, map[string]any{"promptEdits": map[string]any{"goal": "x"}}, &apiErr); code != 400 || apiErr.Code != "invalid_prompt_edit" {
		t.Errorf("non-editable part = %d %+v", code, apiErr)
	}
	var got model.ChatAssignment
	if code := e.do("PATCH", key, map[string]any{"promptEdits": map[string]any{"character": "  You are a pirate.  ", "stay": "Stay a pirate."}}, &got); code != 200 ||
		got.PromptEdits["character"] != "You are a pirate." || len(got.PromptEdits) != 2 {
		t.Fatalf("edit = %d %+v", code, got.PromptEdits)
	}
	var back model.ChatAssignment
	if code := e.do("PATCH", key, map[string]any{"promptEdits": map[string]any{"character": nil, "stay": ""}}, &back); code != 200 || back.PromptEdits != nil {
		t.Errorf("reset = %d %+v", code, back.PromptEdits)
	}
}

func TestCalendarsAndNotesAPI(t *testing.T) {
	e := newEnv(t)
	var apiErr apiError
	if code := e.do("POST", "/api/calendars", map[string]any{"kind": "ics", "url": "ftp://x"}, &apiErr); code != 400 || apiErr.Code != "invalid_calendar" {
		t.Errorf("bad link = %d %+v", code, apiErr)
	}
	var list struct {
		Items []struct {
			ID, Name, Kind, Where string
			HasKey, Enabled       bool
		}
	}
	secret := "https://calendar.google.com/calendar/ical/me%40gmail.com/private-abc123/basic.ics"
	if code := e.do("POST", "/api/calendars", map[string]any{"kind": "ics", "name": "Work", "url": secret}, &list); code != 201 || len(list.Items) != 1 {
		t.Fatalf("add = %d %+v", code, list)
	}
	c := list.Items[0]
	if c.Where != "https://calendar.google.com/…" || !c.Enabled || strings.Contains(c.Where, "private") {
		t.Errorf("not masked: %+v", c)
	}
	if code := e.do("POST", "/api/calendars", map[string]any{"kind": "macalendar", "apiKey": "k"}, &list); code != 201 || list.Items[1].Where != "127.0.0.1:8080" || !list.Items[1].HasKey {
		t.Errorf("macalendar = %d %+v", code, list.Items)
	}
	var ck model.CalendarCheck
	if code := e.do("POST", "/api/calendars/"+c.ID+"/check", nil, &ck); code != 200 || !ck.OK {
		t.Errorf("check = %d %+v", code, ck)
	}
	if code := e.do("PATCH", "/api/calendars/"+c.ID, map[string]any{"enabled": false}, &list); code != 200 || list.Items[0].Enabled {
		t.Errorf("disable = %d %+v", code, list.Items)
	}
	if code := e.do("DELETE", "/api/calendars/"+c.ID, nil, &list); code != 200 || len(list.Items) != 1 {
		t.Errorf("delete = %d %+v", code, list.Items)
	}
	_ = e.st.UpdateSecretaryNotes(func(all []model.SecretaryNote) []model.SecretaryNote {
		return append(all, model.SecretaryNote{ID: "n1", ChatKey: "dm:1", From: "Dana", Text: "call back"}, model.SecretaryNote{ID: "n2", Text: "old", Done: true})
	})
	if code := e.do("PATCH", "/api/secretary/notes/n1", map[string]any{"done": true}, nil); code != 200 {
		t.Errorf("done = %d", code)
	}
	if code := e.do("DELETE", "/api/secretary/notes/done", nil, nil); code != 200 {
		t.Errorf("clear done = %d", code)
	}
	var notes struct{ Items []model.SecretaryNote }
	if code := e.do("GET", "/api/secretary/notes", nil, &notes); code != 200 || len(notes.Items) != 0 {
		t.Errorf("notes = %d %+v", code, notes)
	}
}
