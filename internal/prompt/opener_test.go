package prompt

import (
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

func TestInitiateOpenerNote(t *testing.T) {
	p := leo(t)
	chat := model.ChatAssignment{GoalOverride: "Convince them to come to dinner on Friday"}
	hist := []model.Message{{Speaker: "them", Name: "Dana", Text: "talk soon"}}

	// Automatic check-ins without options keep the classic note.
	if req := Initiate(p, chat, hist, false); !strings.HasSuffix(req.System, InitiateNote) {
		t.Error("zero Opener must keep InitiateNote")
	}

	sys := Initiate(p, chat, hist, false, Options{Opener: Opener{Manual: true, Silence: 50 * time.Hour, Hint: "  the  new\nbakery "}}).System
	for _, want := range []string{
		"It has been 2 days since the last message.",
		"Don't explain why you're writing.",
		"Open with this (your own idea — never say someone suggested it): the new bakery.",
		"make it a natural first small step towards your private agenda — without revealing it",
	} {
		if !strings.Contains(sys, want) {
			t.Errorf("missing %q in:\n%s", want, sys[strings.Index(sys, "GUIDANCE"):])
		}
	}
	if strings.Contains(sys, InitiateNote) {
		t.Error("manual opener replaces the classic note")
	}

	group := Initiate(p, chat, nil, true, Options{Opener: Opener{Manual: true}}).System
	if !strings.Contains(group, "You're starting a conversation in this group") || !strings.Contains(group, "Talk to the group, not to one person") {
		t.Errorf("group opener:\n%s", group)
	}
	short := Initiate(p, chat, hist, false, Options{Opener: Opener{Manual: true, Silence: 3 * time.Hour}}).System
	if !strings.Contains(short, "The chat has gone quiet for 3 hours.") {
		t.Errorf("short silence:\n%s", short)
	}
	// Goal already reached (relax): no agenda line.
	relaxed := Initiate(p, chat, hist, false, Options{Opener: Opener{Manual: true}, Goal: GoalTurn{Reached: true}}).System
	if strings.Contains(relaxed, "private agenda") || strings.Contains(relaxed, "dinner") {
		t.Errorf("relaxed opener must not pursue the goal:\n%s", relaxed)
	}
	long := Initiate(p, chat, hist, false, Options{Opener: Opener{Hint: strings.Repeat("ж", 500)}}).System
	if n := strings.Count(long, "ж"); n != MaxOpenerHint {
		t.Error("hint not capped")
	}
	for d, want := range map[time.Duration]string{30 * time.Minute: "about an hour", 5 * time.Hour: "5 hours", 30 * time.Hour: "a day", 100 * time.Hour: "4 days"} {
		if got := humanSince(d); got != want {
			t.Errorf("humanSince(%s) = %q, want %q", d, got, want)
		}
	}
}

func TestPlanOpeningMode(t *testing.T) {
	p := leo(t)
	cfg := ResolveForTest(p, "Get Josh to say the word apple")
	if u := Plan(p, cfg, nil, true, nil, PlanOptions{Opening: true}).Messages[0].Content; !strings.Contains(u, "about to start the conversation") || !strings.Contains(u, "(no messages yet)") {
		t.Errorf("opening plan:\n%s", u)
	}
}
