package wa

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
	waEvents "go.mau.fi/whatsmeow/types/events"

	"whatsappdoppel/internal/core/events"
	"whatsappdoppel/internal/core/model"
)

func TestGroupMembershipChanges(t *testing.T) {
	m := &Manager{hub: events.NewHub(), self: selfIDs{
		PN:  types.NewJID("972500000000", types.DefaultUserServer),
		LID: types.NewJID("111", types.HiddenUserServer),
	}}
	var got []model.GroupChange
	m.SetGroupHandler(func(c model.GroupChange) { got = append(got, c) })
	a := model.ChatItem{Key: "group:1", JID: "1@g.us", Name: "A"}
	b := model.ChatItem{Key: "group:2", JID: "2@g.us", Name: "B"}
	c := model.ChatItem{Key: "group:3", JID: "3@g.us", Name: "C"}

	if _, known := m.InGroup(a.JID); known {
		t.Fatal("membership must be unknown before the first load")
	}
	m.setGroups([]model.ChatItem{a, b})
	if len(got) != 0 {
		t.Fatalf("first load reports nothing: %+v", got)
	}
	if in, known := m.InGroup(a.JID); !in || !known {
		t.Fatal("member of A")
	}
	m.setGroups([]model.ChatItem{b, c}) // removed from A while offline, added to C
	if len(got) != 2 || got[0] != (model.GroupChange{Key: a.Key, JID: a.JID, Name: "A"}) || !got[1].Joined || got[1].JID != c.JID {
		t.Fatalf("changes = %+v", got)
	}
	if in, _ := m.InGroup(a.JID); in {
		t.Fatal("no longer in A")
	}

	// Live: removed from B (you show up by lid), someone else leaving C changes nothing.
	got = nil
	m.onGroupInfo(nil, &waEvents.GroupInfo{JID: types.NewJID("2", types.GroupServer), Leave: []types.JID{m.self.LID}})
	m.onGroupInfo(nil, &waEvents.GroupInfo{JID: types.NewJID("3", types.GroupServer), Leave: []types.JID{types.NewJID("5", types.DefaultUserServer)}})
	if len(got) != 1 || got[0].JID != b.JID || got[0].Joined {
		t.Fatalf("live changes = %+v", got)
	}
	if in, _ := m.InGroup(b.JID); in {
		t.Fatal("no longer in B")
	}
	acts, _ := m.hub.ActivitySince(0, "", nil, 10)
	if len(acts) != 3 || acts[2].Text != "You are no longer in the group B" {
		t.Errorf("activity = %+v", acts)
	}
}
