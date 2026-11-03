package engine

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// A nickname replaces their WhatsApp name in history and in what the
// persona reads (also for messages from before it was set), and the persona
// can tag them by it.
func TestNicknamesInGroup(t *testing.T) {
	h := newHarness(t, groupReplies(nil, nil), groupChat())
	h.wa.setRoster(groupJID, smallRoster())
	if _, err := h.st.SetNickname([]string{danaLID, "972501234567@s.whatsapp.net"}, "Dee", "Dana Levi"); err != nil {
		t.Fatal(err)
	}
	h.groupFrom(aviPN, "Avi", "G1", "what should we eat")
	h.advance(10 * time.Second)
	h.waitSent(1)
	if _, err := h.st.SetNickname([]string{aviPN}, "Avo", "Avi"); err != nil {
		t.Fatal(err)
	}
	replyWith(h, "@Dee sure")
	h.groupFrom(noamPN, "Noam", "G2", "I'm hungry")
	h.groupFrom(danaLID, "Dana", "G3", "pizza?") // three people talking: the tag helps
	h.advance(10 * time.Second)
	sent := h.waitSent(2)
	if sent[1].text != "@200000000000001 sure" || !slices.Equal(sent[1].mentions, []string{danaLID}) {
		t.Errorf("wire = %q %v", sent[1].text, sent[1].mentions)
	}
	hist := h.history(groupKey)
	if hist[0].Name != "Avo" || hist[3].Name != "Dee" {
		t.Errorf("history names = %q, %q", hist[0].Name, hist[3].Name)
	}
	reqs := replyRequests(h.llm)
	last := reqs[len(reqs)-1]
	all := last.System
	for _, m := range last.Messages {
		all += m.Content
	}
	if !strings.Contains(all, "Avo") || !strings.Contains(all, "Dee") || strings.Contains(all, "Avi:") || strings.Contains(all, "Dana Levi") {
		t.Errorf("prompt names:\n%s", all)
	}
}
