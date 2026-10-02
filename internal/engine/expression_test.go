package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
)

// setPersona edits a stored persona.
func (h *harness) setPersona(id string, fn func(p *model.Persona)) {
	h.t.Helper()
	p, ok := h.st.Persona(id)
	if !ok {
		h.t.Fatalf("persona %q", id)
	}
	fn(&p)
	if _, err := h.st.UpsertPersona(p); err != nil {
		h.t.Fatal(err)
	}
}

func TestExpressionEmojiNoneOnAutoReply(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.setPersona("leo", func(p *model.Persona) { p.Emoji.Usage = "none" })
	h.llm.Push("love that for you darling 💅✨")
	h.msg(dmKey, "D1", "I got the job!!")
	h.advance(9 * time.Second)
	s := h.waitSent(1)
	if s[0].text != "love that for you darling" {
		t.Fatalf("sent %q", s[0].text)
	}
	if a := h.acts(model.ActSent); a[0].Meta["emojiRemoved"] != 2 {
		t.Errorf("meta = %+v", a[0].Meta)
	}
	if sys := replyRequests(h.llm)[0].System; !strings.Contains(sys, "- Emoji: Never use emoji.") || !strings.HasSuffix(sys, " No emoji.") {
		t.Errorf("prompt does not say none:\n%s", sys[len(sys)-200:])
	}
}

func TestExpressionNoEmojiOnSeriousNews(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.setPersona("leo", func(p *model.Persona) { p.Emoji.Usage = "lots" })
	h.llm.Push("oh darling, I'm so sorry 🙏💔 call me tonight?")
	h.msg(dmKey, "D1", "my grandmother passed away this morning")
	h.advance(9 * time.Second)
	if s := h.waitSent(1); s[0].text != "oh darling, I'm so sorry call me tonight?" {
		t.Fatalf("sent %q", s[0].text)
	}
	if sys := replyRequests(h.llm)[0].System; !strings.HasSuffix(sys, "no emoji and no jokes.") {
		t.Errorf("serious turn hint missing: %q", sys[len(sys)-120:])
	}
}

const longDraft = "Darling, that is spectacular news and I am thrilled for you. " +
	"We absolutely must celebrate this weekend with proper cocktails somewhere with decent lighting. " +
	"Tell me everything about the new office, the team and whether the chairs are mid-century."

func TestExpressionLengthRetry(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.llm.Push(longDraft, "Spectacular news, darling. Cocktails this weekend?")
	h.msg(dmKey, "D1", "I got the job!!")
	h.advance(9 * time.Second)
	if s := h.waitSent(1); s[0].text != "Spectacular news, darling. Cocktails this weekend?" {
		t.Fatalf("sent %q", s[0].text)
	}
	reqs := replyRequests(h.llm)
	if len(reqs) != 2 || !strings.Contains(reqs[1].System, "IMPORTANT: your last draft had 39 words") || !strings.Contains(reqs[1].System, "at most 12 words") {
		t.Fatalf("retry request: %d requests", len(reqs))
	}
	if m := h.acts(model.ActSent)[0].Meta; m["lengthRetried"] != true || m["trimmed"] != nil {
		t.Errorf("meta = %+v", m)
	}
}

func TestExpressionLengthTrimAfterRetry(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.llm.Push(longDraft, longDraft)
	h.msg(dmKey, "D1", "I got the job!!")
	h.advance(9 * time.Second)
	s := h.waitSent(1)
	if s[0].text != "Darling, that is spectacular news and I am thrilled for you." {
		t.Fatalf("sent %q", s[0].text)
	}
	if m := h.acts(model.ActSent)[0].Meta; m["lengthRetried"] != true || m["trimmed"] != true {
		t.Errorf("meta = %+v", m)
	}
}

func TestExpressionLongerBiasKeepsLength(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.setPersona("leo", func(p *model.Persona) { p.MessageLength = "long" })
	h.llm.Push(longDraft)
	h.msg(dmKey, "D1", "I got the job!!")
	h.advance(9 * time.Second)
	if s := h.waitSent(1); s[0].text != longDraft {
		t.Fatalf("a long persona keeps 39 words: %q", s[0].text)
	}
	if len(replyRequests(h.llm)) != 1 {
		t.Error("no retry within budget")
	}
}

func TestExpressionApprovalDraft(t *testing.T) {
	c := dmChat()
	c.ApprovalMode = true
	h := newHarness(t, nil, c)
	h.setPersona("leo", func(p *model.Persona) { p.Emoji.Usage = "none" })
	h.llm.Push("stunning 😍 when do we celebrate?")
	h.msg(dmKey, "D1", "I got the job!!")
	h.advance(9 * time.Second)
	h.idle()
	ap := h.st.Approvals()
	if len(ap) != 1 || ap[0].Text != "stunning when do we celebrate?" {
		t.Fatalf("approvals = %+v", ap)
	}
	// Your own edit is sent as written.
	if err := h.e.SendApproved(context.Background(), ap[0].ID, "edited 😍", -1); err != nil {
		t.Fatal(err)
	}
	h.advance(10 * time.Second)
	if s := h.waitSent(1); s[0].text != "edited 😍" {
		t.Errorf("edited approval changed: %q", s[0].text)
	}
}

func TestExpressionPlaygroundAndOpener(t *testing.T) {
	h := newHarness(t, nil)
	h.setPersona("leo", func(p *model.Persona) { p.Emoji.Usage = "none" })
	pg := h.e.Playground()
	id, _ := pg.Start("leo")
	h.llm.Push("hello darling ✨")
	out, err := pg.Initiate(context.Background(), id, false, "")
	if err != nil || out.Reply != "hello darling" {
		t.Fatalf("opener: %+v %v", out, err)
	}
	h.llm.Push("😂😂")
	out, err = pg.Send(context.Background(), id, "lol", false)
	if err != nil || out.Reply != prompt.WordsFor(prompt.ToneFunny) {
		t.Fatalf("emoji-only reply: %+v %v", out, err)
	}
}

func TestExpressionPreviewEngine(t *testing.T) {
	h := newHarness(t, nil)
	p, _ := h.st.Persona("leo")
	p.Emoji.Usage, p.MessageLength = "none", "medium"
	h.llm.SetFunc(nil)
	out, err := h.e.ExpressionPreview(context.Background(), p, "shorter", 3, 1)
	if err != nil || len(out.Samples) != 3 {
		t.Fatalf("preview: %+v %v", out, err)
	}
	seen := map[string]bool{}
	for _, s := range out.Samples {
		if s.Incoming == "" || s.Reply == "" || seen[s.Incoming] || s.Emoji != 0 {
			t.Errorf("sample = %+v", s)
		}
		seen[s.Incoming] = true
		if want := prompt.WordBudget("medium", "shorter", s.Incoming); s.Budget != want {
			t.Errorf("budget %d, want %d", s.Budget, want)
		}
	}
	for _, r := range replyRequests(h.llm) {
		if strings.Contains(r.System, "YOUR PRIVATE AGENDA") {
			t.Error("the preview has no goal agenda")
		}
	}
	again, _ := h.e.ExpressionPreview(context.Background(), p, "", 3, 2)
	if again.Samples[0].Incoming == out.Samples[0].Incoming {
		t.Error("a re-roll picks other messages")
	}
}
