package engine

import (
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/core/model"
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

func russianChat(c model.ChatAssignment) model.ChatAssignment {
	c.Language = &model.LanguagePrefs{Languages: []string{"en", "he"}, Reply: "ru"}
	return c
}

// A language chosen in the settings holds against a Hebrew chat: it is the last
// rule, it is repeated on the turn, and a short reply in the wrong language is
// stopped even though reply checks are off (the default Classic system).
func TestFixedLanguage(t *testing.T) {
	h := newHarness(t, nil, russianChat(dmChat()))
	h.llm.Push("לא, תגידי לנו.", "Нет, скажи нам.")
	h.msg(dmKey, "m1", "מה קורה? איך היה היום")
	h.advance(15 * time.Second)
	sent := h.waitSent(1)
	if sent[0].text != "Нет, скажи нам." {
		t.Fatalf("sent %q", sent[0].text)
	}
	reqs := replyRequests(h.llm)
	if len(reqs) != 2 {
		t.Fatalf("requests = %d", len(reqs))
	}
	first := reqs[0]
	if i := strings.LastIndex(first.System, "MOST IMPORTANT RULE"); i < 0 || strings.Contains(first.System[i:], "GUIDANCE") {
		t.Errorf("the rule is not last: %q", first.System)
	}
	if strings.Contains(first.System, "REPLY LANGUAGE: write this message") {
		t.Errorf("two language rules: %q", first.System)
	}
	if last := first.Messages[len(first.Messages)-1].Content; !strings.Contains(last, "in Russian only") {
		t.Errorf("no cue on the turn: %q", last)
	}
	if !strings.Contains(reqs[1].System, "instead of Russian") {
		t.Errorf("the rewrite isn't told why: %q", reqs[1].System)
	}
}

func TestFixedLanguageStaysQuietRatherThanBreakIt(t *testing.T) {
	h := newHarness(t, nil, russianChat(dmChat()))
	h.llm.Push("לא, תגידי לנו.", "ממש לא יודע.")
	h.msg(dmKey, "m1", "מה קורה? איך היה היום")
	h.advance(15 * time.Second)
	if len(h.wa.Sent()) != 0 || h.lastReason(model.ActDecisionSkip) != "reply_check" {
		t.Fatalf("sent %d, reason %v", len(h.wa.Sent()), h.lastReason(model.ActDecisionSkip))
	}
}

// A language that only follows the chat stays a light rule: no check while
// reply checks are off.
func TestMatchedLanguageKeepsTheLightRule(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.llm.Push("ok sure thing")
	h.msg(dmKey, "m1", "מה קורה? איך היה היום")
	h.advance(15 * time.Second)
	h.waitSent(1)
	if sys := lastSystem(h); strings.Contains(sys, "MOST IMPORTANT RULE") {
		t.Errorf("prompt: %q", sys)
	}
}
