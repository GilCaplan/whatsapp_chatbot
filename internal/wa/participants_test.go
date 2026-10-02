package wa

import (
	"context"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"

	"whatsappdoppel/internal/model"
)

func TestParticipantsFromGroup(t *testing.T) {
	selfPN := types.NewJID("15550000000", types.DefaultUserServer)
	selfLID := types.NewJID("100000000000000", types.HiddenUserServer)
	danaPN := types.NewJID("972501234567", types.DefaultUserServer)
	danaLID := types.NewJID("200000000000001", types.HiddenUserServer)
	noam := types.NewJID("972501111111", types.DefaultUserServer)
	g := &types.GroupInfo{Participants: []types.GroupParticipant{
		{JID: selfLID, PhoneNumber: selfPN, LID: selfLID, IsSuperAdmin: true},
		{JID: danaLID, PhoneNumber: danaPN, LID: danaLID},
		{JID: noam}, // PN-addressed, no lid known
	}}
	names := func(j types.JID) types.ContactInfo {
		switch j {
		case danaPN:
			return types.ContactInfo{PushName: "dana push"}
		case danaLID:
			return types.ContactInfo{FullName: "Dana Levi"}
		case noam:
			return types.ContactInfo{BusinessName: "Noam's Bakery"}
		}
		return types.ContactInfo{}
	}
	got := participantsFromGroup(g, names, selfIDs{PN: selfPN, LID: selfLID})
	if len(got) != 3 {
		t.Fatalf("got %d participants", len(got))
	}
	if !got[0].IsSelf || !got[0].IsAdmin || got[0].JID != selfLID.String() {
		t.Errorf("self = %+v", got[0])
	}
	if d := got[1]; d.JID != "200000000000001@lid" || d.Phone != "972501234567" || d.LID != "200000000000001@lid" || d.Name != "Dana Levi" || d.IsSelf {
		t.Errorf("dana = %+v (full name beats push name)", d)
	}
	if n := got[2]; n.JID != noam.String() || n.Phone != "972501111111" || n.LID != "" || n.Name != "Noam's Bakery" {
		t.Errorf("noam = %+v", n)
	}
	if participantsFromGroup(nil, names, selfIDs{}) != nil {
		t.Error("nil group → nil")
	}
}

func TestFakeRosters(t *testing.T) {
	f := NewFake(nil)
	ctx := context.Background()
	for _, g := range f.groups {
		r, err := f.GroupParticipants(ctx, g.JID)
		if err != nil || len(r) != g.ParticipantCount {
			t.Fatalf("%s: %d members (want %d), err %v", g.Name, len(r), g.ParticipantCount, err)
		}
		if !r[0].IsSelf {
			t.Errorf("%s: first member should be you", g.Name)
		}
		lidGroup := g.Name == "Book Club"
		liors := 0
		for _, p := range r[1:] {
			if strings.HasSuffix(p.JID, "@lid") != lidGroup {
				t.Errorf("%s: member %+v has the wrong addressing", g.Name, p)
			}
			if strings.HasPrefix(p.Name, "Lior ") {
				liors++
			}
		}
		if lidGroup && liors != 2 {
			t.Errorf("Book Club should have two Liors, got %d", liors)
		}
		again, _ := f.GroupParticipants(ctx, g.Key)
		if len(again) != len(r) || again[1] != r[1] {
			t.Errorf("%s: roster not deterministic", g.Name)
		}
	}
	if r, err := f.GroupParticipants(ctx, "dm:15550100001"); r != nil || err != nil {
		t.Errorf("DM → nil, nil; got %v %v", r, err)
	}

	res, err := f.SendMessage(ctx, "120363000000000003@g.us", model.OutMessage{Text: "@200000000000001 hi", Mentions: []string{"200000000000001@lid"}})
	if err != nil {
		t.Fatal(err)
	}
	s := f.Sent()
	if len(s) != 1 || len(s[0].Mentions) != 1 || s[0].Quote != nil || res.ID == "" || s[0].ID != res.ID {
		t.Errorf("sent = %+v (result %+v)", s, res)
	}
	if err := f.EditMessage(ctx, "120363000000000003@g.us", res.ID, model.OutMessage{Text: "@200000000000001 hey"}); err != nil {
		t.Fatal(err)
	}
	if e := f.Edits(); len(e) != 1 || e[0].ID != res.ID || e[0].Text != "@200000000000001 hey" {
		t.Errorf("edits = %+v", e)
	}
}
