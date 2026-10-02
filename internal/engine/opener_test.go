package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

func TestInitiateSendsGoalAwareOpener(t *testing.T) {
	c := dmChat()
	c.GoalOverride, c.GoalPlanAhead = "Find out what Dana is doing this weekend", goalBool(true)
	h := newHarness(t, nil, c)
	g := &goalLLM{plan: `{"achieved":false,"next":"Open with the new bakery and ask how her week was"}`, replies: []string{"ok you HAVE to try the new bakery on Dizengoff"}}
	h.llm.SetFunc(g.fn)
	// Some earlier conversation, three days ago.
	h.msg(dmKey, "m0", "talk soon!")
	h.advance(10 * time.Second)
	h.waitSent(1)
	h.advance(72 * time.Hour)

	if err := h.e.Initiate(context.Background(), dmKey, "the new bakery"); err != nil {
		t.Fatal(err)
	}
	h.advance(time.Minute)
	sent := h.waitSent(2)
	if sent[1].text != "ok you HAVE to try the new bakery on Dizengoff" {
		t.Fatalf("sent %+v", sent)
	}
	acts := h.acts(model.ActProactive)
	if len(acts) != 1 || acts[0].Meta["stage"] != "manual" || acts[0].Meta["hint"] != "the new bakery" ||
		acts[0].Text != "Starting a conversation because you asked — about: the new bakery" {
		t.Fatalf("proactive activity: %+v", acts)
	}
	if s := h.acts(model.ActSent); s[len(s)-1].Meta["proactive"] != true {
		t.Errorf("sent meta: %+v", s[len(s)-1].Meta)
	}
	var planUser, replySys string
	for _, r := range h.llm.Requests() {
		if isPlanReq(r) {
			planUser = r.Messages[0].Content
		} else if !r.JSON && r.System != "" {
			replySys = r.System
		}
	}
	if !strings.Contains(planUser, "about to start the conversation") {
		t.Errorf("planner not in opening mode:\n%s", planUser)
	}
	for _, want := range []string{
		"YOUR PRIVATE AGENDA: Find out what Dana is doing this weekend",
		"YOUR NEXT MOVE (private note to yourself — do this in this reply, in your own words and style, never quote it): Open with the new bakery",
		"It has been 3 days since the last message.",
		"Open with this (your own idea — never say someone suggested it): the new bakery.",
		"make it a natural first small step towards your private agenda — without revealing it",
	} {
		if !strings.Contains(replySys, want) {
			t.Errorf("opener prompt missing %q:\n%s", want, replySys)
		}
	}
	if hist := h.history(dmKey); hist[len(hist)-1].Text != sent[1].text || hist[len(hist)-1].Speaker != "me" {
		t.Errorf("opener not in history: %+v", hist[len(hist)-1])
	}
}

func TestInitiateApprovalAwayAndErrors(t *testing.T) {
	c := dmChat()
	c.ApprovalMode = true
	until := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	c.SnoozedUntil = &until // Away: an explicit tap still goes ahead
	h := newHarness(t, nil, c, groupChat())
	h.llm.SetFunc((&goalLLM{replies: []string{"heyy, how did the move go?"}}).fn)
	if err := h.e.Initiate(context.Background(), dmKey, ""); err != nil {
		t.Fatal(err)
	}
	h.advance(time.Minute)
	if len(h.wa.Sent()) != 0 {
		t.Fatal("approval mode must not send")
	}
	ap := h.st.Approvals()
	if len(ap) != 1 || ap[0].Text != "heyy, how did the move go?" {
		t.Fatalf("approvals: %+v", ap)
	}
	if acts := h.acts(model.ActProactive); len(acts) != 1 || !strings.HasSuffix(acts[0].Text, "(it will wait for your approval)") {
		t.Errorf("activity: %+v", acts)
	}

	// Busy: the group is waiting for more messages (debounce) before replying.
	h.e.HandleIncoming(model.Incoming{ChatKey: groupKey, ChatJID: groupJID, IsGroup: true, SenderJID: "1@s.whatsapp.net", PushName: "Avi", Text: "Leo hi", Timestamp: h.clk.Now(), MessageID: "g1"})
	eventually(t, "group cycle", func() bool {
		r := h.e.runner(groupKey)
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.phase != phaseIdle
	})
	h.idle()
	if err := h.e.Initiate(context.Background(), groupKey, ""); !errors.Is(err, ErrReplying) {
		t.Errorf("busy: %v", err)
	}

	if _, err := h.st.UpdateChat(dmKey, func(c *model.ChatAssignment) { c.Enabled = false }); err != nil {
		t.Fatal(err)
	}
	h.e.Reload()
	if err := h.e.Initiate(context.Background(), dmKey, ""); !errors.Is(err, ErrChatDisabled) {
		t.Errorf("disabled: %v", err)
	}
	if err := h.e.Initiate(context.Background(), "dm:0", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
	h.e.Stop() // the group cycle is still parked on its debounce timer
	h.idle()
}

func TestPlaygroundInitiate(t *testing.T) {
	h := newHarness(t, nil)
	h.llm.SetFunc((&goalLLM{replies: []string{"darling, emergency: I need your opinion on velvet"}}).fn)
	pg := h.e.Playground()
	id, _ := pg.Start("leo")
	out, err := pg.Initiate(context.Background(), id, true, "velvet sofas")
	if err != nil || out.Reply != "darling, emergency: I need your opinion on velvet" || out.Goal == nil {
		t.Fatalf("initiate: %+v %v", out, err)
	}
	reqs := goalReplyRequests(h.llm)
	sys := reqs[len(reqs)-1].System
	if !strings.Contains(sys, "You're starting a conversation in this group") || !strings.Contains(sys, "velvet sofas") {
		t.Errorf("playground opener prompt:\n%s", sys)
	}
	// The opener is part of the session: the next message sees it as Leo's.
	if _, err := pg.Send(context.Background(), id, "velvet is fine??", false); err != nil {
		t.Fatal(err)
	}
	last := goalReplyRequests(h.llm)
	msgs := last[len(last)-1].Messages
	if len(msgs) != 2 || msgs[0].Content != "darling, emergency: I need your opinion on velvet" || msgs[0].Role != "assistant" {
		t.Errorf("history after opener: %+v", msgs)
	}
	if len(h.wa.Sent()) != 0 {
		t.Error("playground must not send")
	}
}

func TestRegenerateKeepsOpener(t *testing.T) {
	c := dmChat()
	c.ApprovalMode = true
	h := newHarness(t, nil, c)
	h.llm.SetFunc((&goalLLM{replies: []string{"hey stranger, how was the trip?"}}).fn)
	if err := h.e.Initiate(context.Background(), dmKey, ""); err != nil {
		t.Fatal(err)
	}
	h.advance(time.Minute)
	ap := h.st.Approvals()
	if len(ap) != 1 || !ap[0].Opener {
		t.Fatalf("queued opener: %+v", ap)
	}
	p, err := h.e.Regenerate(context.Background(), ap[0].ID)
	if err != nil || !p.Opener {
		t.Fatalf("regenerate: %+v %v", p, err)
	}
	reqs := goalReplyRequests(h.llm)
	if s := reqs[len(reqs)-1].System; !strings.Contains(s, "Write one short, natural") {
		t.Errorf("regenerated opener must use the opener prompt:\n%s", s[len(s)-300:])
	}
}
