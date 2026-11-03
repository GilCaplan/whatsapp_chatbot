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
