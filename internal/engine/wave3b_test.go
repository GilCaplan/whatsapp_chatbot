package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
)

// Wave 3, Engineer B: hand-off, co-pilot drafts, reveal, daily recap.

func isHandoffCheck(r llm.Request) bool {
	return r.JSON && strings.Contains(r.System, "needs a real person")
}
func isDrafts(r llm.Request) bool { return r.JSON && strings.Contains(r.System, "CO-PILOT") }

func countReqs(f *llm.Fake, pred func(llm.Request) bool) int {
	n := 0
	for _, r := range f.Requests() {
		if pred(r) {
			n++
		}
	}
	return n
}

func TestHandoffStrongKeywordPausesChat(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.msg(dmKey, "m1", "hey you")
	h.advance(5 * time.Second) // waiting for more messages: the cycle is live
	h.msg(dmKey, "m2", "wait, are you a bot?")
	h.advance(time.Minute)
	if s := h.wa.Sent(); len(s) != 0 {
		t.Fatalf("a paused chat must not reply: %+v", s)
	}
	c, _ := h.st.Chat(dmKey)
	if c.Handoff == nil || c.Handoff.Category != model.HandoffBot || c.Handoff.How != model.HandoffHowKeyword ||
		c.Handoff.Excerpt != "wait, are you a bot?" || c.Handoff.Sender != "Dana" || c.Handoff.MessageID != "m2" {
		t.Fatalf("handoff = %+v", c.Handoff)
	}
	acts := h.acts(model.ActHandoff)
	if len(acts) != 1 || acts[0].Meta["category"] != model.HandoffBot || !strings.Contains(acts[0].Text, "Dana asked if they're talking to a bot") {
		t.Fatalf("handoff acts = %+v", acts)
	}
	// Later messages are context only; the skip is logged once, not per message.
	h.msg(dmKey, "m3", "hello??")
	h.msg(dmKey, "m4", "answer me")
	h.advance(time.Minute)
	if len(h.wa.Sent()) != 0 {
		t.Fatal("still paused")
	}
	skips := 0
	for _, a := range h.acts(model.ActDecisionSkip) {
		if a.Meta["reason"] == "handoff" {
			skips++
		}
	}
	if skips != 1 {
		t.Errorf("handoff skips = %d, want 1", skips)
	}
	if hist := h.history(dmKey); len(hist) != 4 || hist[3].Text != "answer me" {
		t.Fatalf("history = %+v", hist)
	}

	// Resume: the next message is answered with everything as context.
	if err := h.e.ResumeHandoff(dmKey); err != nil {
		t.Fatal(err)
	}
	if c, _ := h.st.Chat(dmKey); c.Handoff != nil {
		t.Fatal("resume should clear the hand-off")
	}
	if len(h.acts(model.ActHandoffResumed)) != 1 {
		t.Error("resume activity")
	}
	if err := h.e.ResumeHandoff(dmKey); err != nil {
		t.Errorf("resuming twice is a no-op: %v", err)
	}
	h.llm.Reset()
	h.msg(dmKey, "m5", "ok whatever, dinner?")
	h.advance(10 * time.Second)
	if s := h.waitSent(1); len(s) != 1 {
		t.Fatalf("after resume sent = %+v", s)
	}
	reqs := replyRequests(h.llm)
	if len(reqs) != 1 || len(reqs[0].Messages) != 5 {
		t.Fatalf("reply should see the whole conversation: %+v", reqs)
	}
	if err := h.e.ResumeHandoff("dm:nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown chat: %v", err)
	}
}

func TestHandoffWeakHitAsksAI(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	// The fake answers "none": a weak hit is not escalated and gets a reply.
	h.msg(dmKey, "m1", "I still owe you for the pizza")
	h.advance(10 * time.Second)
	if n := countReqs(h.llm, isHandoffCheck); n != 1 {
		t.Fatalf("hand-off checks = %d", n)
	}
	if req := h.llm.Requests()[0]; req.MaxTokens > 60 || req.Temperature != 0 {
		t.Errorf("check request = %+v", req)
	}
	if len(h.waitSent(1)) != 1 {
		t.Fatal("not serious: reply as usual")
	}
	// An LLM error never pauses.
	h.llm.SetFunc(func(r llm.Request) (llm.Response, error) {
		if isHandoffCheck(r) {
			return llm.Response{}, errors.New("model offline")
		}
		return llm.Response{Text: "haha ok"}, nil
	})
	h.msg(dmKey, "m2", "can you pay me back")
	h.advance(10 * time.Second)
	if c, _ := h.st.Chat(dmKey); c.Handoff != nil {
		t.Fatal("an LLM error must not pause")
	}
	// The AI says it's serious: paused, how = ai.
	h.llm.SetFunc(func(r llm.Request) (llm.Response, error) {
		if isHandoffCheck(r) {
			return llm.Response{Text: `{"category":"money","serious":true}`}, nil
		}
		return llm.Response{Text: "haha ok"}, nil
	})
	sent := len(h.wa.Sent())
	h.msg(dmKey, "m3", "you owe me 300 for the tickets")
	h.advance(10 * time.Second)
	c, _ := h.st.Chat(dmKey)
	if c.Handoff == nil || c.Handoff.Category != model.HandoffMoney || c.Handoff.How != model.HandoffHowAI {
		t.Fatalf("handoff = %+v", c.Handoff)
	}
	if len(h.wa.Sent()) != sent {
		t.Error("paused chats don't reply")
	}
}

func TestHandoffSettings(t *testing.T) {
	h := newHarness(t, func(s *config.Settings) {
		s.Safety.Handoff.Bot = false
		s.Safety.Handoff.AICheck = false
	}, dmChat())
	h.msg(dmKey, "m1", "are you a bot?") // category off
	h.msg(dmKey, "m2", "I owe you")      // weak, AI check off
	h.advance(10 * time.Second)
	if c, _ := h.st.Chat(dmKey); c.Handoff != nil {
		t.Fatalf("handoff = %+v", c.Handoff)
	}
	if countReqs(h.llm, isHandoffCheck) != 0 {
		t.Error("AI check is off")
	}
	if _, err := h.cfg.Update(func(s *config.Settings) { s.Safety.Handoff.Enabled = false }); err != nil {
		t.Fatal(err)
	}
	h.msg(dmKey, "m3", "I'm in hospital")
	h.advance(10 * time.Second)
	if c, _ := h.st.Chat(dmKey); c.Handoff != nil {
		t.Fatal("hand-off switched off")
	}
}

func TestHandoffHoldsPendingApprovals(t *testing.T) {
	c := dmChat()
	c.Mode = model.ChatModeApprove
	h := newHarness(t, both(func(p *model.BehaviorProfile) { p.AutoSendSeconds = 60 }), c)
	h.msg(dmKey, "m1", "dinner tonight?")
	h.advance(9 * time.Second)
	eventually(t, "pending", func() bool { return len(h.st.Approvals()) == 1 })
	if h.st.Approvals()[0].AutoSendAt == nil {
		t.Fatal("approve mode auto-sends after 60 s")
	}
	h.msg(dmKey, "m2", "actually I'm in the hospital")
	p := h.st.Approvals()[0]
	if p.AutoSendAt != nil || !p.Stale {
		t.Fatalf("held approval = %+v", p)
	}
	h.advance(2 * time.Minute)
	if len(h.wa.Sent()) != 0 {
		t.Fatal("a held approval must not auto-send")
	}
	// You can still send it yourself.
	if err := h.e.SendApproved(context.Background(), p.ID, "", -1); err != nil {
		t.Fatal(err)
	}
}

func TestHandoffInGroupFiresWhenQuiet(t *testing.T) {
	h := newHarness(t, both(func(p *model.BehaviorProfile) { p.ChimeInPercent, p.AIJudgement = 0, false }), groupChat())
	h.groupMsg("g1", "lol anyway")
	h.groupMsg("g2", "is this group chat run by a bot or something? are you a bot")
	if c, _ := h.st.Chat(groupKey); c.Handoff == nil || c.Handoff.Sender != "Avi" {
		t.Fatalf("group handoff = %+v", c.Handoff)
	}
}

// ─── co-pilot ────────────────────────────────────────────────

func copilotChat() model.ChatAssignment {
	c := dmChat()
	c.Mode = model.ChatModeCopilot
	return c
}

func draftsJSON(texts ...string) string {
	var ds []map[string]string
	for i, t := range texts {
		ds = append(ds, map[string]string{"tone": prompt.DraftTones[i%3], "text": t})
	}
	b, _ := json.Marshal(map[string]any{"drafts": ds})
	return string(b)
}

func TestCopilotQueuesThreeDrafts(t *testing.T) {
	h := newHarness(t, both(func(p *model.BehaviorProfile) { p.AutoSendSeconds = 30 }), copilotChat())
	h.llm.Push(draftsJSON("sure", "yes please, I'm starving. where?", "only if you're cooking"))
	h.msg(dmKey, "m1", "dinner tonight?")
	h.advance(9 * time.Second)
	eventually(t, "pending", func() bool { return len(h.st.Approvals()) == 1 })
	p := h.st.Approvals()[0]
	if len(p.Drafts) != 3 || p.Text != "sure" || p.Drafts[1].Tone != model.DraftToneWarm || p.Drafts[2].Text != "only if you're cooking" {
		t.Fatalf("pending = %+v", p)
	}
	if p.AutoSendAt != nil {
		t.Fatal("co-pilot never auto-sends")
	}
	if n := countReqs(h.llm, isDrafts); n != 1 || len(h.llm.Requests()) != 1 {
		t.Fatalf("one drafts call expected, got %d of %d", n, len(h.llm.Requests()))
	}
	if req := h.llm.Requests()[0]; req.MaxTokens != 900 || len(req.Schema) == 0 {
		t.Errorf("drafts request = %d tokens", req.MaxTokens)
	}
	q := h.acts(model.ActApprovalQueued)
	if len(q) != 1 || q[0].Meta["drafts"] != 3 {
		t.Errorf("queued meta = %+v", q)
	}
	h.advance(2 * time.Minute)
	if len(h.wa.Sent()) != 0 {
		t.Fatal("nothing is sent until you pick")
	}
	if err := h.e.SendApproved(context.Background(), p.ID, "", 2); err != nil {
		t.Fatal(err)
	}
	if s := h.wa.Sent(); len(s) != 1 || s[0].text != "only if you're cooking" {
		t.Fatalf("sent = %+v", s)
	}
}

func TestCopilotDropsBadDraftsAndFallsBack(t *testing.T) {
	h := newHarness(t, nil, copilotChat())
	long := strings.Repeat("That sounds great, honestly I would love to come. ", 12)
	h.llm.Push(draftsJSON("As an AI language model, I can't eat dinner.", "sure thing", long))
	h.msg(dmKey, "m1", "dinner tonight?")
	h.advance(9 * time.Second)
	eventually(t, "pending", func() bool { return len(h.st.Approvals()) == 1 })
	p := h.st.Approvals()[0]
	if len(p.Drafts) != 2 || p.Drafts[0].Text != "sure thing" {
		t.Fatalf("drafts = %+v", p.Drafts)
	}
	// The expression rules (word budget) apply to every draft.
	if w := prompt.WordCount(p.Drafts[1].Text); w >= 40 {
		t.Errorf("long draft not trimmed: %d words", w)
	}

	// Fewer than two good drafts: one normal reply, no drafts.
	h.llm.Reset()
	h.llm.Push(draftsJSON("As an AI, I don't eat."), "fine, 8pm")
	if _, err := h.e.Regenerate(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	p, _ = h.st.Approval(p.ID)
	if len(p.Drafts) != 0 || p.Text != "fine, 8pm" || p.AutoSendAt != nil {
		t.Fatalf("fallback = %+v", p)
	}
	if n := countReqs(h.llm, isDrafts); n != 1 {
		t.Errorf("More ideas asks for drafts again: %d", n)
	}
}

// ─── reveal ──────────────────────────────────────────────────

func TestRevealSendsAndPauses(t *testing.T) {
	c := dmChat()
	c.Mode = model.ChatModeApprove
	h := newHarness(t, func(s *config.Settings) { s.Safety.Reveal.Template = "It was {persona} all along. Love, {me}" }, c)
	h.msg(dmKey, "m1", "hey")
	h.advance(9 * time.Second)
	eventually(t, "pending", func() bool { return len(h.st.Approvals()) == 1 })

	text, err := h.e.Reveal(context.Background(), dmKey, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if text != "It was Leo all along. Love, me" {
		t.Errorf("text = %q", text)
	}
	if s := h.wa.Sent(); len(s) != 1 || s[0].text != text || s[0].jid != dmJID {
		t.Fatalf("sent = %+v", s)
	}
	got, _ := h.st.Chat(dmKey)
	if got.Enabled || got.RevealedAt == nil || got.Handoff != nil {
		t.Fatalf("chat = %+v", got)
	}
	if len(h.st.Approvals()) != 0 {
		t.Error("pending replies are discarded")
	}
	hist := h.history(dmKey)
	last := hist[len(hist)-1]
	if last.Kind != model.MsgKindReveal || last.FromBot || last.Speaker != "me" || last.WAID == "" {
		t.Errorf("history = %+v", last)
	}
	if a := h.acts(model.ActReveal); len(a) != 1 || a[0].Text != "Revealed Leo to Dana and paused this chat" {
		t.Errorf("reveal acts = %+v", a)
	}
	if h.e.runner(dmKey) != nil {
		t.Error("the runner stops")
	}
	if _, err := h.e.Reveal(context.Background(), dmKey, "", false); !errors.Is(err, ErrAlreadyRevealed) {
		t.Errorf("second reveal: %v", err)
	}
	if text, err := h.e.Reveal(context.Background(), dmKey, "  again, it was Leo  ", true); err != nil || text != "again, it was Leo" {
		t.Errorf("forced reveal = %q %v", text, err)
	}
	if len(h.wa.Sent()) != 2 {
		t.Error("forced reveal sends again")
	}
}

func TestRenderReveal(t *testing.T) {
	if got := RenderReveal(config.DefaultRevealTemplate, "Leo", "Rocky"); !strings.Contains(got, "chatting with Leo") {
		t.Errorf("default = %q", got)
	}
	if got := RenderReveal("{me} here, {persona} out", "", ""); got != "me here, a persona out" {
		t.Errorf("blanks = %q", got)
	}
}

// ─── daily recap ─────────────────────────────────────────────

func TestNextRecapAt(t *testing.T) {
	loc := time.FixedZone("X", 3*3600)
	now := time.Date(2026, 10, 1, 20, 0, 0, 0, loc)
	if got := nextRecapAt(now, "21:00", loc); !got.Equal(time.Date(2026, 10, 1, 21, 0, 0, 0, loc)) {
		t.Errorf("later today = %v", got)
	}
	if got := nextRecapAt(now, "07:30", loc); !got.Equal(time.Date(2026, 10, 2, 7, 30, 0, 0, loc)) {
		t.Errorf("tomorrow = %v", got)
	}
	if got := nextRecapAt(now, "20:00", loc); !got.Equal(time.Date(2026, 10, 2, 20, 0, 0, 0, loc)) {
		t.Errorf("exactly now = next day: %v", got)
	}
	if got := nextRecapAt(now, "bad", loc); got.Hour() != 21 {
		t.Errorf("bad time = %v", got)
	}
}

func TestRecapScheduledAndOnDemand(t *testing.T) {
	quiet := model.ChatAssignment{Key: "dm:972502222222", Kind: "dm", JID: "972502222222@s.whatsapp.net", Name: "Josh", PersonaID: "leo", Enabled: true}
	h := newHarness(t, nil, dmChat(), quiet)
	now := h.clk.Now()
	for i, text := range []string{"exam on friday", "so stressed", "wish me luck"} {
		if err := h.st.AppendHistory(dmKey, model.Message{ID: "d" + string(rune('0'+i)), TS: now, Speaker: "them", Name: "Dana", Text: text}, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.st.AppendHistory(quiet.Key, model.Message{ID: "j1", TS: now, Speaker: "them", Name: "Josh", Text: "yo"}, 0); err != nil {
		t.Fatal(err)
	}
	// Switching the recap on (a settings change) arms the timer.
	at := now.Add(time.Hour).In(time.Local).Format("15:04")
	if _, err := h.cfg.Update(func(s *config.Settings) { s.Recap.Enabled, s.Recap.Time = true, at }); err != nil {
		t.Fatal(err)
	}
	h.advance(59 * time.Minute)
	if len(h.st.Recaps("", 0)) != 0 {
		t.Fatal("too early")
	}
	h.advance(2 * time.Minute)
	recaps := h.st.Recaps("", 0)
	if len(recaps) != 1 || recaps[0].ChatKey != dmKey || recaps[0].Headline == "" || recaps[0].MessageCount != 3 || recaps[0].OnDemand {
		t.Fatalf("scheduled recaps = %+v", recaps)
	}
	if st := h.st.RunnerState(dmKey); st.LastRecapAt.IsZero() {
		t.Error("LastRecapAt not set")
	}
	if a := h.acts(model.ActRecap); len(a) != 1 || a[0].Meta["chats"] != 1 || a[0].ChatKey != dmKey {
		t.Errorf("recap acts = %+v", a)
	}
	// The next run is armed for tomorrow, not right away.
	h.advance(time.Hour)
	if len(h.acts(model.ActRecap)) != 1 {
		t.Error("ran twice")
	}

	// On demand, a quiet chat gets one too.
	got, err := h.e.GenerateRecap(context.Background(), quiet.Key)
	if err != nil || len(got) != 1 || got[0].ChatKey != quiet.Key || !got[0].OnDemand {
		t.Fatalf("on demand = %+v %v", got, err)
	}
	if _, err := h.e.GenerateRecap(context.Background(), "dm:nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown chat: %v", err)
	}
	// Turning it off stops the timer.
	if _, err := h.cfg.Update(func(s *config.Settings) { s.Recap.Enabled = false }); err != nil {
		t.Fatal(err)
	}
	h.e.recap.mu.Lock()
	armed := h.e.recap.timer != nil
	h.e.recap.mu.Unlock()
	if armed {
		t.Error("recap timer still armed after switching it off")
	}
}

func TestRecapErrorsAndNothingToDo(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	got, err := h.e.GenerateRecap(context.Background(), "")
	if err != nil || len(got) != 0 {
		t.Fatalf("nothing to recap = %+v %v", got, err)
	}
	if err := h.st.AppendHistory(dmKey, model.Message{ID: "x", TS: h.clk.Now(), Speaker: "them", Text: "hi"}, 0); err != nil {
		t.Fatal(err)
	}
	h.llm.Push("not json at all")
	if _, err := h.e.GenerateRecap(context.Background(), dmKey); err == nil {
		t.Error("an unreadable answer is an error")
	}
}
