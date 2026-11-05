package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/core/config"
	"whatsappdoppel/internal/core/model"
)

// fakeTranslator marks what it translated, in the target's alphabet.
type fakeTranslator struct {
	answer string
	err    error
}

func (f *fakeTranslator) Translate(_ context.Context, text, code string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if f.answer != "" {
		return f.answer, nil
	}
	return map[string]string{"ru": "[я] ", "he": "[א] ", "ja": "[あ] "}[code] + text, nil
}
func (f *fakeTranslator) Status() model.TranslatorStatus { return model.TranslatorStatus{} }
func (f *fakeTranslator) Install([]string)               {}

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
	// Hebrew: written in English, sent in Hebrew.
	h.msg(dmKey, "h1", "מה קורה? איך היה היום")
	h.advance(15 * time.Second)
	if s := h.waitSent(1); !strings.HasPrefix(s[0].text, "[א] ") {
		t.Errorf("hebrew: %q", s[0].text)
	}
	// Unclear ("ok"): follows the conversation (Hebrew so far).
	h.msg(dmKey, "o1", "ok")
	h.advance(15 * time.Second)
	if s := h.waitSent(2); !strings.HasPrefix(s[1].text, "[א] ") {
		t.Errorf("unclear: %q", s[1].text)
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
	if s := h.waitSent(3); strings.HasPrefix(s[2].text, "[") {
		t.Errorf("person choice: %q", s[2].text)
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

// The language settings never reach the prompt: it is the same text whatever
// the chat speaks, and the reply goes through the translator.
func TestPromptIgnoresTheLanguageSetting(t *testing.T) {
	prompts := map[string]string{}
	for _, reply := range []string{"", model.LangMatch, "ru", "he", "ja"} {
		c := dmChat()
		c.Language = &model.LanguagePrefs{Others: model.LangAnswer, Reply: reply}
		h := newHarness(t, nil, c)
		h.msg(dmKey, "m1", "מה קורה? איך היה היום")
		h.advance(15 * time.Second)
		h.waitSent(1)
		sys := lastSystem(h)
		if !strings.HasSuffix(sys, "never write in any other language.") {
			t.Fatalf("the English rule isn't last: %q", sys[len(sys)-120:])
		}
		req := replyRequests(h.llm)[0]
		if last := req.Messages[len(req.Messages)-1].Content; !strings.Contains(last, "in English.)") {
			t.Errorf("no cue on the turn: %q", last)
		}
		prompts[sys] = reply
	}
	if len(prompts) != 1 {
		t.Fatalf("%d different prompts", len(prompts))
	}
}

func TestReplyIsTranslatedAndItsSourceKept(t *testing.T) {
	h := newHarness(t, nil, russianChat(dmChat()))
	h.llm.Push("Darling, that lighting is dreadful.")
	h.msg(dmKey, "m1", "מה קורה? איך היה היום")
	h.advance(15 * time.Second)
	if s := h.waitSent(1); s[0].text != "[я] Darling, that lighting is dreadful." {
		t.Fatalf("sent %q", s[0].text)
	}
	own := h.history(dmKey)[1]
	if own.Source != "Darling, that lighting is dreadful." {
		t.Fatalf("source = %+v", own)
	}
	// It reads its own words back in English.
	h.msg(dmKey, "m2", "מה עוד?")
	h.advance(15 * time.Second)
	h.waitSent(2)
	reqs := replyRequests(h.llm)
	if got := reqs[1].Messages[1]; got.Content != "Darling, that lighting is dreadful." {
		t.Errorf("history shows %+v", got)
	}
}

// A reply in the chat's language already is left alone, and English settings
// never call the translator.
func TestNoTranslationWhenNotNeeded(t *testing.T) {
	h := newHarness(t, nil, russianChat(dmChat()))
	h.llm.Push("Нет, скажи нам об этом.")
	h.msg(dmKey, "m1", "hello there how are you")
	h.advance(15 * time.Second)
	if s := h.waitSent(1); s[0].text != "Нет, скажи нам об этом." {
		t.Fatalf("sent %q", s[0].text)
	}
	h2 := newHarness(t, nil, dmChat())
	h2.msg(dmKey, "m1", "hello there how are you")
	h2.advance(15 * time.Second)
	if s := h2.waitSent(1); strings.HasPrefix(s[0].text, "[") {
		t.Fatalf("sent %q", s[0].text)
	}
}

// What code stops in a reply, with a rewrite first.
func TestFilterStopsAReply(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.llm.Push("I want to commit suicide", "The lighting is lovely tonight")
	h.msg(dmKey, "m1", "how are you today")
	h.advance(15 * time.Second)
	if s := h.waitSent(1); s[0].text != "The lighting is lovely tonight" {
		t.Fatalf("sent %q", s[0].text)
	}
	h2 := newHarness(t, nil, dmChat())
	h2.llm.Push("nigga please", "still nigga please")
	h2.msg(dmKey, "m1", "how are you today")
	h2.advance(15 * time.Second)
	if len(h2.wa.Sent()) != 0 || h2.lastReason(model.ActDecisionSkip) != "reply_check" {
		t.Fatalf("sent %d, reason %v", len(h2.wa.Sent()), h2.lastReason(model.ActDecisionSkip))
	}
}

func TestFilterTakesOutCurseWordsAndCanBeOff(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.llm.Push("You're a fucking genius")
	h.msg(dmKey, "m1", "how are you today")
	h.advance(15 * time.Second)
	if s := h.waitSent(1); s[0].text != "You're a genius" {
		t.Fatalf("sent %q", s[0].text)
	}
	h2 := newHarness(t, func(s *config.Settings) { s.Safety.Filter.Enabled = false }, dmChat())
	h2.llm.Push("You're a fucking genius")
	h2.msg(dmKey, "m1", "how are you today")
	h2.advance(15 * time.Second)
	if s := h2.waitSent(1); s[0].text != "You're a fucking genius" {
		t.Fatalf("sent %q", s[0].text)
	}
}

// The filter also reads the translation: a clean draft can come back dirty.
func TestFilterReadsTheTranslation(t *testing.T) {
	h := newHarness(t, nil, russianChat(dmChat()))
	h.tr.answer = "ты ниггер"
	h.llm.Push("You are lovely, darling")
	h.msg(dmKey, "m1", "hello there how are you")
	h.advance(15 * time.Second)
	if len(h.wa.Sent()) != 0 || h.lastReason(model.ActDecisionSkip) != "reply_check" {
		t.Fatalf("sent %+v", h.wa.Sent())
	}
}

// Without the add-on the AI model translates, and its answer is checked.
func TestTranslationFallsBackToTheModel(t *testing.T) {
	h := newHarness(t, nil, russianChat(dmChat()))
	h.tr.err = errors.New("not ready")
	h.llm.Push("You are lovely, darling", "Ты прекрасна, дорогая моя.")
	h.msg(dmKey, "m1", "hello there how are you")
	h.advance(15 * time.Second)
	if s := h.waitSent(1); s[0].text != "Ты прекрасна, дорогая моя." {
		t.Fatalf("sent %q", s[0].text)
	}
	h2 := newHarness(t, nil, russianChat(dmChat()))
	h2.tr.err = errors.New("not ready")
	h2.llm.Push("You are lovely, darling", "still in english, sorry about that")
	h2.msg(dmKey, "m1", "hello there how are you")
	h2.advance(15 * time.Second)
	if len(h2.wa.Sent()) != 0 {
		t.Fatalf("sent %+v", h2.wa.Sent())
	}
}
