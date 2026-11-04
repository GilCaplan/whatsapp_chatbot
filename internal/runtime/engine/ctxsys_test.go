package engine

import (
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/core/config"
	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/logic/ctxsys"
	"whatsappdoppel/internal/runtime/llm"
)

func useSystem(id string) func(*config.Settings) {
	return func(s *config.Settings) { s.Context.System = id }
}

func reqsWith(reqs []llm.Request, needle string) int {
	n := 0
	for _, r := range reqs {
		all := r.System
		for _, m := range r.Messages {
			all += m.Content
		}
		if strings.Contains(all, needle) {
			n++
		}
	}
	return n
}

func TestJoinRules(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	me := model.Message{Speaker: "me", FromBot: true, Text: "who's up for pizza", TS: now.Add(-time.Minute)}
	them := func(name, text string) model.Message {
		return model.Message{Speaker: "them", Name: name, Text: text, TS: now.Add(-time.Minute)}
	}
	cases := []struct {
		text, sender string
		recent       []model.Message
		want         verdict
		why          string
	}{
		{"lol", "Avi", nil, clearNo, ReasonSmallReaction},
		{"hahaha", "Avi", nil, clearNo, ReasonSmallReaction},
		{"חחחח", "Avi", nil, clearNo, ReasonSmallReaction},
		{"ok!", "Avi", nil, clearNo, ReasonSmallReaction},
		{"sure, I'm in", "Avi", []model.Message{me}, clearYes, ReasonAnswersMe},
		{"anyone free tonight?", "Avi", nil, clearYes, ReasonAsksEveryone},
		{"מישהו בא מחר?", "Avi", nil, clearYes, ReasonAsksEveryone},
		{"did you see it", "Avi", []model.Message{them("Avi", "a"), them("Noa", "b"), them("Avi", "c"), them("Noa", "d")}, clearNo, ReasonOthersTalking},
		{"the match was crazy", "Avi", []model.Message{them("Avi", "a"), them("Noa", "b"), them("Dan", "c"), them("Noa", "d")}, unsure, ""},
		{"the match was crazy", "Avi", nil, unsure, ""},
	}
	for _, c := range cases {
		v, why := joinRules(c.text, c.sender, c.recent, now)
		if v != c.want || why != c.why {
			t.Errorf("%q: got %v %q, want %v %q", c.text, v, why, c.want, c.why)
		}
	}
}

func TestCodeCheck(t *testing.T) {
	cp := ctxsys.ParamsFor(model.CtxLean)
	hist := []model.Message{{Speaker: "me", FromBot: true, Text: "COMING AT YOU STRONG, BRO! let's train"}}
	ck := replyCheck{cp: cp, rules: "Tag at most one person per message and only when it helps.", plan: "Ask Vsevolod what he's usually up to on a Saturday", hist: hist, lang: "English"}
	cases := []struct {
		text string
		fail bool
	}{
		{"haha yes, saturday works for me", false},
		{"ok i will tag at most one person per message", true},
		{"Ask Vsevolod what he's usually up to on a Saturday", true},
		{"COMING AT YOU STRONG, BRO! again", true},
		{"as an AI I can't do that", true},
		{"מה קורה איתכם היום חברים", true}, // Hebrew where English is the rule
	}
	for _, c := range cases {
		fails, _ := codeCheck(ck, c.text)
		if (len(fails) > 0) != c.fail {
			t.Errorf("%q: fails=%v, want fail=%v", c.text, fails, c.fail)
		}
	}
}

// Lean settles a clear group message in code: no AI join-in call.
func TestLeanJoinsByRules(t *testing.T) {
	h := newHarness(t, func(s *config.Settings) {
		useSystem(model.CtxLean)(s)
		s.Behavior.Group.AIJudgement = true
	}, groupChat())
	h.groupMsg("m1", "lol")
	if got := h.lastReason(model.ActDecisionSkip); got != ReasonSmallReaction {
		t.Fatalf("skip reason = %v, want %s", got, ReasonSmallReaction)
	}
	h.groupMsg("m2", "anyone up for pizza tonight?")
	h.advance(time.Minute)
	h.waitSent(1)
	if n := reqsWith(h.llm.Requests(), "YES or NO"); n != 0 {
		t.Errorf("%d AI join-in calls, want 0", n)
	}
	// Classic asks the AI for the same message.
	h2 := newHarness(t, func(s *config.Settings) { s.Behavior.Group.AIJudgement = true }, groupChat())
	h2.groupMsg("m2", "anyone up for pizza tonight?")
	if n := reqsWith(h2.llm.Requests(), "YES or NO"); n != 1 {
		t.Errorf("classic: %d AI join-in calls, want 1", n)
	}
}

// A draft that keeps failing the check is not sent.
func TestReplyCheckStaysQuiet(t *testing.T) {
	h := newHarness(t, useSystem(model.CtxLean), dmChat())
	h.llm.SetFunc(func(r llm.Request) (llm.Response, error) {
		if strings.Contains(r.System, "compact persona card") {
			return llm.Response{Text: "You are Leo, laid back."}, nil
		}
		return llm.Response{Text: "lol my next move: ask about saturday"}, nil
	})
	h.msg(dmKey, "m1", "hey what's up")
	h.advance(time.Minute)
	if n := len(h.wa.Sent()); n != 0 {
		t.Fatalf("sent %d messages, want 0", n)
	}
	if got := h.lastReason(model.ActDecisionSkip); got != "reply_check" {
		t.Errorf("skip reason = %v, want reply_check", got)
	}
}

// Lean's digest does memories in one call after DigestEvery messages.
func TestDigest(t *testing.T) {
	h := newHarness(t, func(s *config.Settings) {
		useSystem(model.CtxLean)(s)
		s.Memory.Enabled = true
	}, dmChat())
	every := ctxsys.ParamsFor(model.CtxLean).DigestEvery
	for i := range every {
		h.msg(dmKey, "m"+string(rune('a'+i%26))+string(rune('a'+i/26)), "I just started a new job as a nurse at the hospital")
	}
	h.advance(time.Minute)
	reqs := h.llm.Requests()
	if n := reqsWith(reqs, "background jobs at once"); n != 1 {
		t.Fatalf("%d digest calls, want 1", n)
	}
	if n := reqsWith(reqs, "You keep a private notebook") - reqsWith(reqs, "background jobs at once"); n != 0 {
		t.Errorf("%d separate memory calls, want 0", n)
	}
	mems, _ := h.st.Memories(dmKey)
	if len(mems) == 0 {
		t.Error("the digest saved no memories")
	}
}

// The sandbox replays with two systems and counts their calls; nothing is sent.
func TestReplay(t *testing.T) {
	h := newHarness(t, func(s *config.Settings) { s.Behavior.Group.AIJudgement = true }, groupChat())
	for i, text := range []string{"what's everyone doing this weekend?", "the match was crazy", "lol"} {
		m := model.Message{TS: h.clk.Now().Add(time.Duration(i-5) * time.Minute), Speaker: "them", Name: "Avi", SenderJID: "972502222222@s.whatsapp.net", Text: text}
		if err := h.st.AppendHistory(groupKey, m, 0); err != nil {
			t.Fatal(err)
		}
	}
	run, err := h.e.StartReplay(model.ReplayRequest{ChatKey: groupKey, Systems: []string{"classic", "fast"}, Messages: 3})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "the replay", func() bool { r, _ := h.e.Replay(run.ID); return r.Status != "running" })
	r, _ := h.e.Replay(run.ID)
	if r.Status != "done" || len(r.Results) != 2 || r.Done != 6 {
		t.Fatalf("replay = %+v", r)
	}
	classic, fast := r.Results[0], r.Results[1]
	if classic.CallsByStep["join"] != 3 || fast.CallsByStep["join"] != 0 {
		t.Errorf("join calls: classic %d, fast %d", classic.CallsByStep["join"], fast.CallsByStep["join"])
	}
	if fast.Steps[2].Decision != "skip" {
		t.Errorf("fast answered %q", fast.Steps[2].Text)
	}
	if n := len(h.wa.Sent()); n != 0 {
		t.Errorf("the sandbox sent %d messages", n)
	}
	if hist := h.history(groupKey); len(hist) != 3 {
		t.Errorf("the sandbox changed the history: %d messages", len(hist))
	}
}

// Summary facts: compaction keeps one fact per line and only adds changes.
func TestCompactFacts(t *testing.T) {
	h := newHarness(t, useSystem(model.CtxLean), dmChat())
	h.seedHistory(dmKey, 50)
	sm, err := h.e.Compact(h.e.ctx, dmKey, 10)
	if err != nil {
		t.Fatal(err)
	}
	if sm.Format != model.SummaryFacts || !strings.HasPrefix(sm.Text, "Other: ") {
		t.Fatalf("summary = %+v", sm)
	}
	ctx, err := h.e.ChatContext(dmKey)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctx.System, "EARLIER IN THIS CHAT (facts") || ctx.ContextSystem.SystemID != model.CtxLean {
		t.Errorf("context: system %q, prompt without facts", ctx.ContextSystem.SystemID)
	}
}

// Periodic planning: a plan only every PlanEvery replies.
func TestPlanDue(t *testing.T) {
	cp := ctxsys.ParamsFor(model.CtxLean)
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	st := model.GoalStatus{LastPlanAt: &at}
	var hist []model.Message
	for i := range cp.PlanEvery - 1 {
		hist = append(hist, model.Message{Speaker: "me", FromBot: true, TS: at.Add(time.Duration(i+1) * time.Minute)})
	}
	if planDue(cp, st, hist) {
		t.Error("due too early")
	}
	hist = append(hist, model.Message{Speaker: "me", FromBot: true, TS: at.Add(time.Hour)})
	if !planDue(cp, st, hist) {
		t.Error("not due after PlanEvery replies")
	}
	if !planDue(cp, model.GoalStatus{}, nil) {
		t.Error("not due without a plan")
	}
}
