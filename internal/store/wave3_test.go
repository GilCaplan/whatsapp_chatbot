package store

import (
	"errors"
	"os"
	"testing"

	"whatsappdoppel/internal/model"
)

func TestChatModeSync(t *testing.T) {
	s, p := openTemp(t)
	// Legacy file: approvalMode only → mode derived on open.
	if err := os.WriteFile(p.ChatsFile(), []byte(`[
	  {"key":"dm:1","kind":"dm","jid":"1@s.whatsapp.net","personaId":"leo","enabled":true,"approvalMode":true},
	  {"key":"dm:2","kind":"dm","jid":"2@s.whatsapp.net","personaId":"leo","enabled":true}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a, _ := s.Chat("dm:1"); a.Mode != model.ChatModeApprove || !a.ApprovalMode {
		t.Errorf("dm:1 = %q %v", a.Mode, a.ApprovalMode)
	}
	if b, _ := s.Chat("dm:2"); b.Mode != model.ChatModeAuto || b.ApprovalMode {
		t.Errorf("dm:2 = %q %v", b.Mode, b.ApprovalMode)
	}
	// Setting the mode updates approvalMode.
	c, _ := s.UpdateChat("dm:2", func(c *model.ChatAssignment) { c.Mode = model.ChatModeCopilot })
	if !c.ApprovalMode {
		t.Error("copilot implies approvalMode")
	}
	// Only approvalMode changed → the mode follows it.
	c, _ = s.UpdateChat("dm:2", func(c *model.ChatAssignment) { c.ApprovalMode = false })
	if c.Mode != model.ChatModeAuto || c.ApprovalMode {
		t.Errorf("after approvalMode=false: %q %v", c.Mode, c.ApprovalMode)
	}
	c, _ = s.UpdateChat("dm:2", func(c *model.ChatAssignment) { c.ApprovalMode = true })
	if c.Mode != model.ChatModeApprove {
		t.Errorf("after approvalMode=true: %q", c.Mode)
	}
	// New chats: an explicit mode wins, else approvalMode.
	n, _ := s.UpsertChat(model.ChatAssignment{Key: "dm:3", Mode: model.ChatModeCopilot})
	if !n.ApprovalMode {
		t.Error("new copilot chat should have approvalMode")
	}
	n, _ = s.UpsertChat(model.ChatAssignment{Key: "dm:4", ApprovalMode: true})
	if n.Mode != model.ChatModeApprove {
		t.Errorf("new approval chat mode = %q", n.Mode)
	}
}

func TestUpdateHistory(t *testing.T) {
	s, _ := openTemp(t)
	if err := s.UpdateHistory("dm:1", "x", func(*model.Message) {}); !errors.Is(err, ErrNotFound) {
		t.Errorf("no file: %v", err)
	}
	for _, m := range []model.Message{{ID: "a", Speaker: "them", Text: "hi"}, {ID: "b", Speaker: "me", Text: "helo", Corrected: "hello", WAID: "W1"}} {
		if err := s.AppendHistory("dm:1", m, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.UpdateHistory("dm:1", "b", func(m *model.Message) { m.Text, m.Kind = m.Corrected, model.MsgKindEdited }); err != nil {
		t.Fatal(err)
	}
	h, _ := s.History("dm:1", 0)
	if len(h) != 2 || h[1].Text != "hello" || h[1].Kind != "edited" || h[1].WAID != "W1" || h[0].Text != "hi" {
		t.Errorf("history = %+v", h)
	}
	if err := s.UpdateHistory("dm:1", "zz", func(*model.Message) {}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
}
