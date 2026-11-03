package engine

import (
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

func lastSystem(h *harness) string {
	reqs := replyRequests(h.llm)
	if len(reqs) == 0 {
		h.t.Fatal("no reply request")
	}
	return reqs[len(reqs)-1].System
}

func TestLanguages(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	// Not one of its languages (English and Hebrew by default): quiet.
	h.msg(dmKey, "r1", "привет, как дела?")
	h.advance(15 * time.Second)
	if len(h.wa.Sent()) != 0 || h.lastReason(model.ActDecisionSkip) != "language" {
		t.Fatalf("russian: sent %d, reason %v", len(h.wa.Sent()), h.lastReason(model.ActDecisionSkip))
	}
	if d := h.decisions(dmKey)["r1"]; d.Outcome != model.DecisionSkipped || d.Reason != "language" {
		t.Errorf("log = %+v", d)
	}
	// Hebrew: answered in Hebrew.
	h.msg(dmKey, "h1", "מה קורה? איך היה היום")
	h.advance(15 * time.Second)
	h.waitSent(1)
	if sys := lastSystem(h); !strings.Contains(sys, "REPLY LANGUAGE: write this message in Hebrew") {
		t.Errorf("hebrew prompt: %q", sys)
	}
	// Unclear ("ok"): follows the conversation (Hebrew so far).
	h.msg(dmKey, "o1", "ok")
	h.advance(15 * time.Second)
	h.waitSent(2)
	if sys := lastSystem(h); !strings.Contains(sys, "in Hebrew") {
		t.Errorf("unclear prompt: %q", sys)
	}
	// The contact's own choice wins over matching.
	if _, err := h.st.UpdateChat(dmKey, func(c *model.ChatAssignment) {
		c.People.People = []model.PersonPrefs{{JID: dmJID, Language: "en"}}
		c.Language = &model.LanguagePrefs{Others: model.LangAnswer}
	}); err != nil {
		t.Fatal(err)
	}
	h.e.Reload()
	h.msg(dmKey, "h2", "תגיד, איך היה בעבודה היום")
	h.advance(15 * time.Second)
	h.waitSent(3)
	if sys := lastSystem(h); !strings.Contains(sys, "in English") {
		t.Errorf("person choice: %q", sys)
	}
	// Others answered: Russian gets a reply now.
	h.msg(dmKey, "r2", "привет, как дела?")
	h.advance(15 * time.Second)
	h.waitSent(4)
}

// With auto-continue a hand-off message is only flagged: no pause, a reply.
func TestAutoContinueHandoff(t *testing.T) {
	c := dmChat()
	c.AutoContinue = true
	h := newHarness(t, nil, c)
	h.msg(dmKey, "b1", "wait, are you a bot?")
	h.advance(15 * time.Second)
	h.waitSent(1)
	if got, _ := h.st.Chat(dmKey); got.Handoff != nil {
		t.Errorf("paused: %+v", got.Handoff)
	}
	acts := h.acts(model.ActHandoff)
	if len(acts) != 1 || acts[0].Meta["autoContinued"] != true {
		t.Fatalf("handoff acts = %+v", acts)
	}
	if d := h.decisions(dmKey)["b1"]; d.Outcome != model.DecisionReplied {
		t.Errorf("log = %+v", d)
	}
}

func TestIgnoreStickers(t *testing.T) {
	h := newHarness(t, both(func(p *model.BehaviorProfile) { p.IgnoreStickers = true }), dmChat())
	h.e.HandleIncoming(model.Incoming{ChatKey: dmKey, ChatJID: dmJID, SenderJID: dmJID, PushName: "Dana", Text: "[sticker]",
		Media: model.MediaSticker, Timestamp: h.clk.Now(), MessageID: "s1"})
	h.idle()
	h.advance(15 * time.Second)
	if len(h.wa.Sent()) != 0 || h.lastReason(model.ActDecisionSkip) != "sticker" {
		t.Fatalf("sent %d, reason %v", len(h.wa.Sent()), h.lastReason(model.ActDecisionSkip))
	}
	if hist := h.history(dmKey); len(hist) != 1 {
		t.Errorf("sticker not kept as context: %d messages", len(hist))
	}
}
