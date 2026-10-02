package server

import (
	"slices"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

const friendsJID = "1203630001@g.us"

func (e *testEnv) assignGroup(t *testing.T) string {
	t.Helper()
	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": friendsJID, "personaId": "leo"}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	return c.Key
}

func roster(n int) []model.Participant {
	out := []model.Participant{{JID: "15550000@s.whatsapp.net", Phone: "15550000", Name: "Me", IsSelf: true}}
	names := []string{"Dana", "Eli", "Josh", "Maya", "Noam", "Omer", "Tal", "Uri", "Yael", "Zoe", "Avi", "Ben"}
	for i := range n - 1 {
		lid := "20000000000000" + string(rune('a'+i))
		out = append(out, model.Participant{JID: lid + "@lid", LID: lid + "@lid", Phone: "1555010" + string(rune('0'+i%10)) + string(rune('0'+i/10)), Name: names[i%len(names)]})
	}
	return out
}

func TestChatPeopleAutoBySize(t *testing.T) {
	e := newEnv(t)
	key := e.assignGroup(t)
	e.wa.mu.Lock()
	e.wa.rosters = map[string][]model.Participant{friendsJID: roster(5)}
	e.wa.mu.Unlock()

	var v peopleView
	if code := e.do("GET", "/api/chats/"+key+"/people", nil, &v); code != 200 {
		t.Fatalf("GET = %d", code)
	}
	if v.Mode != "auto" || v.EffectiveMode != "everyone" || v.MemberCount != 5 || v.Threshold != 8 || len(v.Members) != 5 {
		t.Fatalf("small group = %+v", v)
	}
	if !v.Members[len(v.Members)-1].IsSelf {
		t.Error("you should be listed last")
	}
	for _, m := range v.Members[:4] {
		if !m.Respond || m.RespondSource != "mode" {
			t.Errorf("small group member %+v should follow the mode (everyone)", m)
		}
	}

	e.wa.mu.Lock()
	e.wa.rosters[friendsJID] = roster(12)
	e.wa.mu.Unlock()
	e.do("GET", "/api/chats/"+key+"/people", nil, &v)
	if v.EffectiveMode != "selected" || v.MemberCount != 12 || v.Members[0].Respond {
		t.Errorf("big group = %s %d %+v", v.EffectiveMode, v.MemberCount, v.Members[0])
	}
}

func TestPatchPeopleUpsertAndLeft(t *testing.T) {
	e := newEnv(t)
	key := e.assignGroup(t)
	r := roster(12)
	e.wa.mu.Lock()
	e.wa.rosters = map[string][]model.Participant{friendsJID: r}
	e.wa.mu.Unlock()
	josh := r[3] // lid-addressed; preferences may be saved with the phone form
	body := map[string]any{"people": map[string]any{"mode": "selected", "people": []map[string]any{
		{"jid": josh.Phone + "@s.whatsapp.net", "name": "Josh", "respond": true, "priority": true, "notes": "my little brother"},
		{"jid": "999@s.whatsapp.net", "name": "Gone", "notes": "left last year"},
	}}}
	var c model.ChatAssignment
	if code := e.do("PATCH", "/api/chats/"+key, body, &c); code != 200 {
		t.Fatalf("PATCH = %d", code)
	}
	if c.People.Mode != "selected" || len(c.People.People) != 2 || c.People.People[0].Respond == nil || !*c.People.People[0].Respond {
		t.Fatalf("saved = %+v", c.People)
	}
	// Upsert by JID: clear respond, keep the rest; respond:null on someone without other prefs drops them.
	e.do("PATCH", "/api/chats/"+key, map[string]any{"people": map[string]any{"people": []map[string]any{
		{"jid": josh.Phone + "@s.whatsapp.net", "respond": nil},
		{"jid": "999@s.whatsapp.net", "notes": ""},
	}}}, &c)
	if len(c.People.People) != 1 || c.People.People[0].Respond != nil || !c.People.People[0].Priority || c.People.People[0].Notes != "my little brother" {
		t.Fatalf("after upsert = %+v", c.People)
	}

	e.do("PATCH", "/api/chats/"+key, map[string]any{"people": map[string]any{"people": []map[string]any{
		{"jid": "777@s.whatsapp.net", "name": "Old friend", "respond": true},
	}}}, nil)
	var v peopleView
	e.do("GET", "/api/chats/"+key+"/people", nil, &v)
	var got *personView
	for i := range v.Members {
		if v.Members[i].JID == josh.JID {
			got = &v.Members[i]
		}
	}
	// Priority ("always reply") means answered, even in a big group in "selected" mode.
	if got == nil || !got.Priority || got.Notes != "my little brother" || !got.Respond || got.RespondSource != "person" {
		t.Errorf("josh = %+v", got)
	}
	last := v.Members[len(v.Members)-1]
	if !last.Left || last.Name != "Old friend" || !last.Respond || last.RespondSource != "person" {
		t.Errorf("left member = %+v", last)
	}

	for _, bad := range []map[string]any{
		{"mode": "some"},
		{"people": []map[string]any{{"jid": "nobody"}}},
		{"people": []map[string]any{{"jid": "1@lid", "notes": strings.Repeat("x", 301)}}},
		{"people": []map[string]any{{"jid": "1@lid", "respond": "yes"}}},
		{"people": []map[string]any{{"jid": "1@lid", "colour": "red"}}},
		{"extra": 1},
	} {
		var er map[string]string
		if code := e.do("PATCH", "/api/chats/"+key, map[string]any{"people": bad}, &er); code != 400 || er["code"] != "invalid_people" {
			t.Errorf("%v → %d %v", bad, code, er)
		}
	}
	// null forgets everything.
	e.do("PATCH", "/api/chats/"+key, map[string]any{"people": nil}, &c)
	if c.People.Mode != "" || len(c.People.People) != 0 {
		t.Errorf("reset = %+v", c.People)
	}
}

func TestChatPeopleDMAndLastSpoke(t *testing.T) {
	e := newEnv(t)
	var c model.ChatAssignment
	e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo"}, &c)
	e.do("PATCH", "/api/chats/"+c.Key, map[string]any{"people": map[string]any{"people": []map[string]any{{"jid": c.JID, "notes": "my sister"}}}}, nil)
	var v peopleView
	e.do("GET", "/api/chats/"+c.Key+"/people", nil, &v)
	if v.Kind != "dm" || len(v.Members) != 1 || v.Members[0].Notes != "my sister" || v.Members[0].Name != "Dana" || v.Members[0].Phone != "15550001" {
		t.Errorf("dm = %+v", v)
	}

	key := e.assignGroup(t)
	r := roster(4)
	e.wa.mu.Lock()
	e.wa.rosters = map[string][]model.Participant{friendsJID: r}
	e.wa.mu.Unlock()
	now := time.Now().UTC().Truncate(time.Second)
	_ = e.st.AppendHistory(key, model.Message{ID: "1", TS: now.Add(-time.Hour), Speaker: "them", Name: "Josh", SenderJID: r[3].JID, Text: "a"}, 100)
	_ = e.st.AppendHistory(key, model.Message{ID: "2", TS: now, Speaker: "them", Name: "Eli", SenderJID: r[2].JID, Text: "b"}, 100)
	e.do("GET", "/api/chats/"+key+"/people", nil, &v)
	names := []string{}
	for _, m := range v.Members {
		names = append(names, m.Name)
	}
	if !slices.Equal(names, []string{"Eli", "Josh", "Dana", "Me"}) || v.Members[0].LastSpokeAt == nil {
		t.Errorf("order = %v", names)
	}
}

func TestBehaviorMentionFieldsAndSimulateSender(t *testing.T) {
	e := newEnv(t)
	key := e.assignGroup(t)
	var c model.ChatAssignment
	if code := e.do("PATCH", "/api/chats/"+key, map[string]any{"behavior": map[string]any{"allowMentions": false, "mentionMax": 2, "triggerWords": []string{"pizza"}}}, &c); code != 200 {
		t.Fatalf("PATCH = %d", code)
	}
	if c.Behavior.AllowMentions == nil || *c.Behavior.AllowMentions || *c.Behavior.MentionMax != 2 || len(*c.Behavior.TriggerWords) != 1 {
		t.Errorf("overrides = %+v", c.Behavior)
	}
	var er map[string]string
	if code := e.do("PATCH", "/api/chats/"+key, map[string]any{"behavior": map[string]any{"mentionMax": 9}}, &er); code != 400 || er["code"] != "invalid_behavior" {
		t.Errorf("mentionMax 9 → %d %v", code, er)
	}
	var pr struct {
		Ranges map[string]map[string]int `json:"ranges"`
	}
	e.do("GET", "/api/behavior/presets", nil, &pr)
	if pr.Ranges["mentionMax"]["max"] != 5 || pr.Ranges["maxStreak"]["max"] != 50 {
		t.Errorf("ranges = %v", pr.Ranges["mentionMax"])
	}
	if code := e.do("POST", "/api/chats/"+key+"/simulate", map[string]any{"text": "hi", "senderJid": "20000000000000a@lid"}, nil); code != 200 {
		t.Fatalf("simulate = %d", code)
	}
	e.eng.mu.Lock()
	sim := slices.Clone(e.eng.simulated)
	e.eng.mu.Unlock()
	if len(sim) != 1 || sim[0] != key+"|hi|20000000000000a@lid" {
		t.Errorf("simulated = %v", sim)
	}
}
