package store

import (
	"testing"

	"whatsappdoppel/internal/core/model"
)

func TestNicknames(t *testing.T) {
	s, paths := openTemp(t)
	const pn, lid = "972501234567@s.whatsapp.net", "200000000000001@lid"
	if _, err := s.SetNickname([]string{lid, pn}, "  Seva ", "Vsevolod"); err != nil {
		t.Fatal(err)
	}
	if got := s.NicknameFor("972501234567:3@s.whatsapp.net"); got != "Seva" {
		t.Errorf("by phone device JID = %q", got)
	}
	if got := s.NicknameFor("nobody@lid", lid); got != "Seva" {
		t.Errorf("by lid = %q", got)
	}
	msgs := []model.Message{{Speaker: "them", SenderJID: lid, Name: "Vsevolod"}, {Speaker: "me", Name: "Leo"}}
	s.ApplyNicknames(msgs)
	if msgs[0].Name != "Seva" || msgs[1].Name != "Leo" {
		t.Errorf("applied = %+v", msgs)
	}
	for _, bad := range []string{"@Seva", "a:b", "this nickname is much too long for anyone at all"} {
		if _, err := s.SetNickname([]string{pn}, bad, ""); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, err := s.SetNickname(nil, "x", ""); err == nil {
		t.Error("no ids accepted")
	}
	// Setting it again by one address replaces it and keeps the others.
	nk, err := s.SetNickname([]string{pn}, "Sev", "")
	if err != nil || len(s.Nicknames()) != 1 || nk.Original != "Vsevolod" || len(nk.IDs) != 2 {
		t.Fatalf("replace = %+v %v (%d)", nk, err, len(s.Nicknames()))
	}
	s2, err := Open(paths, nil)
	if err != nil || s2.NicknameFor(lid) != "Sev" {
		t.Fatalf("reopened: %v %q", err, s2.NicknameFor(lid))
	}
	if _, err := s.SetNickname([]string{lid}, "", ""); err != nil || s.NicknameFor(pn) != "" || len(s.Nicknames()) != 0 {
		t.Errorf("remove: %v %q", err, s.NicknameFor(pn))
	}
}
