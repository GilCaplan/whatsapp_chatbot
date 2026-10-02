package engine

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
	"whatsappdoppel/internal/store"
)

// realistic sets explicit, jitter-free timings (Fixed{Frac:0} picks minimums).
func realistic(p *model.BehaviorProfile) {
	p.NoticeMinSec, p.NoticeMaxSec = 5, 5
	p.MarkRead = true
	p.WaitForMoreSec, p.BurstCapSec = 8, 60
	p.ThinkMinSec, p.ThinkMaxSec = 4, 4
	p.TypingIndicator = true
	p.TypingCharsPerSec, p.TypingJitterPercent, p.TypingMinSec, p.TypingMaxSec = 10, 0, 1, 25
}

func (h *harness) msg(key, id, text string) {
	h.t.Helper()
	h.e.HandleIncoming(model.Incoming{ChatKey: key, ChatJID: dmJID, SenderJID: dmJID, PushName: "Dana", Text: text,
		Timestamp: h.clk.Now(), MessageID: id})
	h.idle()
}

func (h *harness) groupMsg(id, text string) {
	h.t.Helper()
	h.e.HandleIncoming(model.Incoming{ChatKey: groupKey, ChatJID: groupJID, IsGroup: true, SenderJID: "972502222222@s.whatsapp.net",
		PushName: "Avi", Text: text, Timestamp: h.clk.Now(), MessageID: id})
	h.idle()
}

func lastMeta(acts []model.ActivityEvent, key string) any {
	if len(acts) == 0 {
		return nil
	}
	return acts[len(acts)-1].Meta[key]
}

func TestPipelinePhasesInOrder(t *testing.T) {
	h := newHarness(t, both(realistic), dmChat())
	h.llm.Push("Sounds great, see you there!") // 28 runes → 2.8 s at 10 cps
	h.msg(dmKey, "M1", "dinner at 8?")
	if len(h.wa.Reads()) != 0 || len(h.acts(model.ActNoticing)) != 1 {
		t.Fatal("should be noticing, not read yet")
	}
	h.advance(4900 * time.Millisecond)
	if len(h.wa.Reads()) != 0 {
		t.Fatal("read before the notice delay")
	}
	h.advance(100 * time.Millisecond) // t=5: seen
	if r := h.wa.Reads(); len(r) != 1 || r[0].ids[0] != "M1" || r[0].chat != dmJID {
		t.Fatalf("reads = %+v", r)
	}
	h.advance(8 * time.Second) // t=13: thinking (LLM runs meanwhile)
	if len(h.acts(model.ActThinking)) != 1 || len(h.llm.Requests()) != 1 || len(h.wa.Typings()) != 0 {
		t.Fatalf("thinking: acts=%v typings=%v", h.actTypes(), h.wa.Typings())
	}
	h.advance(4 * time.Second) // t=17: typing starts
	if ty := h.wa.Typings(); len(ty) == 0 || !ty[0].on || len(h.wa.Sent()) != 0 {
		t.Fatalf("typing should be on and nothing sent yet: %+v", ty)
	}
	if a := h.acts(model.ActTyping); len(a) != 1 || a[0].Meta["typingSeconds"] != 2.8 || a[0].Meta["chars"] != 28 {
		t.Fatalf("typing act = %+v", a)
	}
	h.advance(2700 * time.Millisecond)
	if len(h.wa.Sent()) != 0 {
		t.Fatal("sent before typing finished")
	}
	h.advance(100 * time.Millisecond)
	if s := h.wa.Sent(); len(s) != 1 || s[0].text != "Sounds great, see you there!" {
		t.Fatalf("sent = %+v", s)
	}
	want := []string{model.ActIncoming, model.ActNoticing, model.ActSeen, model.ActWaiting, model.ActThinking, model.ActTyping, model.ActSent}
	if got := h.actTypes(want...); !slices.Equal(got, want) {
		t.Errorf("acts = %v", got)
	}
	if ty := h.wa.Typings(); ty[len(ty)-1].on {
		t.Error("typing should end paused")
	}
	for _, a := range append(h.acts(model.ActNoticing), h.acts(model.ActSeen)...) {
		if a.Text == "" || strings.ContainsAny(a.Text, "✨💅") {
			t.Errorf("activity text %q", a.Text)
		}
	}
	if st := h.st.RunnerState(dmKey); len(st.Replies) != 1 || st.LastIncomingAt.IsZero() {
		t.Errorf("runtime state = %+v", st)
	}
	if h.clk.Pending() != 0 {
		t.Errorf("timers left: %d", h.clk.Pending())
	}
}

func TestTypingScalesWithLength(t *testing.T) {
	h := newHarness(t, both(func(p *model.BehaviorProfile) {
		p.TypingCharsPerSec, p.TypingJitterPercent, p.TypingMinSec, p.TypingMaxSec = 10, 0, 0, 60
	}), dmChat())
	h.llm.Push(strings.Repeat("a", 20), strings.Repeat("b", 80))
	for i, want := range []float64{2, 8} {
		h.simulate(dmKey, "msg", false)
		h.advance(9 * time.Second)
		h.advance(time.Duration(want * float64(time.Second)))
		a := h.acts(model.ActTyping)
		if len(a) != i+1 || a[i].Meta["typingSeconds"] != want || len(h.wa.Sent()) != i+1 {
			t.Fatalf("reply %d: typing acts %+v, sent %d", i, a, len(h.wa.Sent()))
		}
	}
}

func TestLLMLatencyAbsorbedByThink(t *testing.T) {
	h := newHarness(t, both(func(p *model.BehaviorProfile) {
		p.WaitForMoreSec = 0
		p.TypingCharsPerSec, p.TypingJitterPercent, p.TypingMinSec, p.TypingMaxSec = 10, 0, 0, 60
	}), dmChat())
	h.llm.SetDelay(200 * time.Millisecond)
	h.llm.Push(strings.Repeat("x", 30))
	if err := h.e.Simulate(dmKey, "hey", false, ""); err != nil {
		t.Fatal(err)
	}
	// Think time is 0, the model is slow: "typing…" shows while it writes.
	eventually(t, "typing before the text is ready", func() bool {
		ty := h.wa.Typings()
		return len(ty) > 0 && ty[0].on
	})
	if len(h.wa.Sent()) != 0 {
		t.Fatal("sent too early")
	}
	h.idle()
	// The planned 3 s of typing still run after the text arrives (fake time did not move).
	h.advance(2900 * time.Millisecond)
	if len(h.wa.Sent()) != 0 {
		t.Fatal("typing cut short")
	}
	h.advance(100 * time.Millisecond)
	h.waitSent(1)
}

func TestSplitIntoBubblesWithGaps(t *testing.T) {
	h := newHarness(t, both(func(p *model.BehaviorProfile) {
		p.SplitPercent, p.SplitMaxParts = 100, 3
		p.BubbleGapMinSec, p.BubbleGapMaxSec = 2, 2
		p.TypingCharsPerSec, p.TypingJitterPercent, p.TypingMinSec, p.TypingMaxSec = 20, 0, 1, 60
		p.LengthBias = "longer" // 17 words fit Leo's "short" budget only when longer
	}), dmChat())
	reply := "Honestly I loved it. The second half dragged a little though. Want to go again next week?"
	h.llm.Push(reply)
	h.simulate(dmKey, "how was the movie?", false)
	h.advance(9 * time.Second) // think 0 → typing bubble 1 (20 runes → 1 s)
	h.advance(time.Second)
	if s := h.wa.Sent(); len(s) != 1 || s[0].text != "Honestly I loved it." {
		t.Fatalf("first bubble: %+v", s)
	}
	h.advance(1900 * time.Millisecond) // gap 2 s not over
	if len(h.wa.Sent()) != 1 {
		t.Fatal("gap ignored")
	}
	h.advance(time.Minute)
	sent := h.wa.Sent()
	if len(sent) != 3 || strings.Join([]string{sent[0].text, sent[1].text, sent[2].text}, " ") != reply {
		t.Fatalf("bubbles = %+v", sent)
	}
	acts := h.acts(model.ActSent)
	if len(acts) != 3 || acts[2].Meta["part"] != 3 || acts[2].Meta["parts"] != 3 {
		t.Errorf("sent acts = %+v", acts)
	}
	hist := h.history(dmKey)
	if len(hist) != 4 || hist[3].Text != sent[2].text || !hist[1].FromBot {
		t.Errorf("history = %+v", hist)
	}
	if st := h.st.RunnerState(dmKey); len(st.Replies) != 1 {
		t.Errorf("a split reply counts once: %+v", st.Replies)
	}
}

func TestQuoteReplyInGroup(t *testing.T) {
	h := newHarness(t, both(func(p *model.BehaviorProfile) { p.QuoteReplyPercent = 100 }), groupChat(), dmChat())
	h.groupMsg("G1", "Leo, velvet or linen?")
	h.advance(9 * time.Second)
	s := h.waitSent(1)
	if s[0].quote == nil || s[0].quote.MessageID != "G1" || s[0].quote.SenderJID != "972502222222@s.whatsapp.net" || s[0].quote.Text != "Leo, velvet or linen?" {
		t.Fatalf("group reply should quote: %+v", s[0])
	}
	if a := h.acts(model.ActSent); a[0].Meta["quoted"] != true {
		t.Errorf("meta = %+v", a[0].Meta)
	}
	h.msg(dmKey, "D1", "hi")
	h.advance(9 * time.Second)
	if s := h.waitSent(2); s[1].quote != nil {
		t.Error("DMs never quote")
	}
}

func TestReactInsteadOfReplying(t *testing.T) {
	h := newHarness(t, func(s *config.Settings) {
		g := &s.Behavior.Group
		g.AIJudgement, g.ChimeInPercent, g.ReactPercent = false, 0, 100
		g.NoticeMinSec, g.NoticeMaxSec = 3, 3
	}, groupChat())
	h.groupMsg("G7", "I got the job!!")
	if a := h.acts(model.ActDecisionSkip); len(a) != 1 || a[0].Meta["reason"] != ReasonChimeOut {
		t.Fatalf("skip = %+v", a)
	}
	if len(h.wa.Reactions()) != 0 {
		t.Fatal("reacted before noticing")
	}
	h.advance(3 * time.Second)
	re := h.wa.Reactions()
	if len(re) != 1 || re[0].id != "G7" || re[0].chat != groupJID || re[0].emoji == "" {
		t.Fatalf("reactions = %+v", re)
	}
	// Good news → a favourite of Leo's that suits it (💅 ✨ 🍸 🛋️: ✨ and 🍸 do; Fixed picks the first).
	if re[0].emoji != "✨" {
		t.Errorf("emoji %q should be Leo's favourite that suits good news", re[0].emoji)
	}
	a := h.acts(model.ActReacted)
	if len(a) != 1 || a[0].Text != "Reacted to Avi's good news with sparkles instead of replying" ||
		a[0].Meta["emoji"] != re[0].emoji || a[0].Meta["tone"] != "good" || prompt.CountEmoji(a[0].Text) != 0 {
		t.Errorf("reacted act = %+v", a)
	}
	if len(h.wa.Reads()) != 1 || len(h.wa.Sent()) != 0 || len(replyRequests(h.llm)) != 0 {
		t.Error("should read and react, never reply")
	}
}

func TestLeftOnRead(t *testing.T) {
	h := newHarness(t, both(func(p *model.BehaviorProfile) {
		p.ReplyPercent = 50
		p.NoticeMinSec, p.NoticeMaxSec = 4, 4
	}), dmChat())
	h.msg(dmKey, "M9", "you there?")
	a := h.acts(model.ActDecisionSkip)
	if len(a) != 1 || a[0].Meta["reason"] != "reply_chance" || !strings.Contains(a[0].Text, "left on read") {
		t.Fatalf("skip = %+v", a)
	}
	h.advance(4 * time.Second)
	if r := h.wa.Reads(); len(r) != 1 || r[0].ids[0] != "M9" {
		t.Fatalf("reads = %+v", r)
	}
	h.advance(time.Minute)
	if len(h.wa.Sent()) != 0 || len(h.llm.Requests()) != 0 || len(h.history(dmKey)) != 1 {
		t.Error("left on read must not reply (but keeps context)")
	}
	// A lucky roll replies.
	h.rng.set(behavior.Fixed{Hits: true})
	h.msg(dmKey, "M10", "hello??")
	h.advance(20 * time.Second)
	h.waitSent(1)
}

func TestNewMessageWhileThinkingRegenerates(t *testing.T) {
	h := newHarness(t, both(func(p *model.BehaviorProfile) { p.ThinkMinSec, p.ThinkMaxSec = 10, 10 }), dmChat())
	h.msg(dmKey, "A", "first")
	h.advance(9 * time.Second) // thinking for 10 s, LLM call 1 done
	if len(replyRequests(h.llm)) != 1 {
		t.Fatal("expected first generation")
	}
	h.msg(dmKey, "B", "second thought")
	if r := h.wa.Reads(); len(r) != 2 || r[1].ids[0] != "B" {
		t.Errorf("message during thinking is read at once: %+v", r)
	}
	h.advance(10 * time.Second) // draft discarded → wait 3 s → think 10 s
	if w := h.acts(model.ActWaiting); lastMeta(w, "reason") != "new_messages" {
		t.Fatalf("waiting = %+v", w)
	}
	if len(h.wa.Sent()) != 0 {
		t.Fatal("stale draft sent")
	}
	h.advance(3*time.Second + 10*time.Second)
	sent := h.waitSent(1)
	reqs := replyRequests(h.llm)
	if len(sent) != 1 || len(reqs) != 2 || len(reqs[1].Messages) != 2 || reqs[1].Messages[1].Content != "second thought" {
		t.Fatalf("sent=%d reqs=%d last=%+v", len(sent), len(reqs), reqs[len(reqs)-1].Messages)
	}
}

func TestNewMessageWhileTypingFinishesThenReplies(t *testing.T) {
	h := newHarness(t, both(func(p *model.BehaviorProfile) {
		p.TypingCharsPerSec, p.TypingJitterPercent, p.TypingMinSec, p.TypingMaxSec = 2, 0, 0, 60
	}), dmChat())
	h.llm.Push(strings.Repeat("y", 20), "second reply")
	h.msg(dmKey, "A", "hey")
	h.advance(9*time.Second + 5*time.Second) // typing 10 s, half way
	h.msg(dmKey, "B", "also, pizza?")
	h.advance(5 * time.Second)
	if s := h.wa.Sent(); len(s) != 1 || s[0].text != strings.Repeat("y", 20) {
		t.Fatalf("current reply should finish: %+v", s)
	}
	if len(replyRequests(h.llm)) != 1 {
		t.Fatal("next cycle must wait")
	}
	h.advance(9 * time.Second) // straight to seen → wait 9 s → generate
	h.advance(10 * time.Second)
	sent := h.waitSent(2)
	if sent[1].text != "second reply" || len(h.acts(model.ActSeen)) != 2 {
		t.Fatalf("sent=%+v acts=%v", sent, h.actTypes())
	}
}

func TestYouRepliedStandsDown(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.msg(dmKey, "A", "lunch?")
	if h.clk.Pending() == 0 {
		t.Fatal("expected a pending wait")
	}
	h.simulate(dmKey, "sure, 1pm works", true) // you answered from your phone
	if a := h.acts(model.ActDecisionSkip); len(a) != 1 || a[0].Meta["reason"] != "you_replied" {
		t.Fatalf("skip = %+v", a)
	}
	if h.clk.Pending() != 0 {
		t.Error("cycle should be cancelled")
	}
	h.advance(time.Minute)
	if len(h.wa.Sent()) != 0 || len(h.llm.Requests()) != 0 {
		t.Error("must not reply after you did")
	}
	if st := h.st.RunnerState(dmKey); st.LastYouRepliedAt.IsZero() {
		t.Error("LastYouRepliedAt not recorded")
	}
	// Off: the persona answers anyway.
	h2 := newHarness(t, both(func(p *model.BehaviorProfile) { p.PauseWhenYouReply = false }), dmChat())
	h2.msg(dmKey, "A", "lunch?")
	h2.simulate(dmKey, "sure", true)
	h2.advance(9 * time.Second)
	h2.waitSent(1)
}

func activeHours(from, to, outside string) func(*model.BehaviorProfile) {
	return func(p *model.BehaviorProfile) {
		p.Availability = model.Availability{Enabled: true, Timezone: "UTC", OutsideHours: outside, CatchUpMaxMin: 10,
			Week: behavior.NormalizeWeek([]model.DayHours{
				{Day: "mon", Ranges: []model.TimeRange{{From: from, To: to}}}, {Day: "tue", Ranges: []model.TimeRange{{From: from, To: to}}},
				{Day: "wed", Ranges: []model.TimeRange{{From: from, To: to}}}, {Day: "thu", Ranges: []model.TimeRange{{From: from, To: to}}},
				{Day: "fri", Ranges: []model.TimeRange{{From: from, To: to}}}, {Day: "sat", Ranges: []model.TimeRange{{From: from, To: to}}},
				{Day: "sun", Ranges: []model.TimeRange{{From: from, To: to}}},
			})}
	}
}

func TestActiveHoursQueueAndWake(t *testing.T) {
	// Clock starts Thursday 12:00 UTC; active 18:00–23:00; catch-up 0–10 min (Fixed → 0).
	h := newHarness(t, both(activeHours("18:00", "23:00", "queue")), dmChat())
	h.msg(dmKey, "A", "free tonight?")
	d := h.acts(model.ActDeferred)
	if len(d) != 1 || d[0].Meta["reason"] != "outside_hours" || !strings.Contains(d[0].Text, "18:00") {
		t.Fatalf("deferred = %+v", d)
	}
	wake := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
	if ra, _ := d[0].Meta["resumeAt"].(time.Time); !ra.Equal(wake) {
		t.Errorf("resumeAt = %v", d[0].Meta["resumeAt"])
	}
	if st := h.st.RunnerState(dmKey); st.QueuedWakeAt == nil || !st.QueuedWakeAt.Equal(wake) {
		t.Errorf("queued wake not persisted: %+v", st.QueuedWakeAt)
	}
	if len(h.wa.Reads()) != 0 {
		t.Error("a sleeping persona doesn't read")
	}
	h.msg(dmKey, "B", "hello?")
	if len(h.acts(model.ActDeferred)) != 1 {
		t.Error("second message joins the queue")
	}
	h.advance(6*time.Hour - time.Second)
	if len(h.wa.Sent()) != 0 {
		t.Fatal("replied while asleep")
	}
	h.advance(time.Second + 9*time.Second) // wake → seen → wait → reply (6 h old: no stale check)
	sent := h.waitSent(1)
	if len(sent) != 1 || len(h.acts(model.ActDecisionSkip)) != 0 {
		t.Fatalf("sent = %+v skips=%+v", sent, h.acts(model.ActDecisionSkip))
	}
	if r := h.wa.Reads(); len(r) != 1 || len(r[0].ids) != 2 {
		t.Errorf("both queued messages read on wake: %+v", r)
	}
	if st := h.st.RunnerState(dmKey); st.QueuedWakeAt != nil {
		t.Error("queued wake should be cleared")
	}
}

func TestQueuedWakeSurvivesRestart(t *testing.T) {
	h := newHarness(t, both(activeHours("18:00", "23:00", "queue")), dmChat())
	h.msg(dmKey, "A", "free tonight?")
	// Restart the runner (as after an app restart): the wake-up is re-armed from runtime.json.
	h.st.UpdateChat(dmKey, func(c *model.ChatAssignment) { c.Enabled = false })
	h.e.Reload()
	if h.clk.Pending() != 0 {
		t.Fatal("disabled runner left timers")
	}
	h.st.UpdateChat(dmKey, func(c *model.ChatAssignment) { c.Enabled = true })
	h.e.Reload()
	if h.clk.Pending() != 1 {
		t.Fatalf("wake not restored (pending %d)", h.clk.Pending())
	}
	h.advance(6*time.Hour + 9*time.Second)
	h.waitSent(1)
}

func TestActiveHoursSilent(t *testing.T) {
	h := newHarness(t, both(activeHours("18:00", "23:00", "silent")), dmChat())
	h.msg(dmKey, "A", "free tonight?")
	if a := h.acts(model.ActDecisionSkip); len(a) != 1 || a[0].Meta["reason"] != "outside_hours" {
		t.Fatalf("skip = %+v", a)
	}
	if h.clk.Pending() != 0 || len(h.history(dmKey)) != 1 {
		t.Error("silent: context only, nothing scheduled")
	}
	h.advance(7 * time.Hour)
	if len(h.wa.Sent()) != 0 {
		t.Error("silent mode must not reply later")
	}
}

func TestSnooze(t *testing.T) {
	c := dmChat()
	until := time.Date(2026, 10, 1, 13, 0, 0, 0, time.UTC)
	c.SnoozedUntil = &until
	h := newHarness(t, nil, c)
	h.msg(dmKey, "A", "ping")
	d := h.acts(model.ActDeferred)
	if len(d) != 1 || d[0].Meta["reason"] != "snoozed" || !strings.HasPrefix(d[0].Text, "Away until") {
		t.Fatalf("deferred = %+v", d)
	}
	h.advance(59 * time.Minute)
	if len(h.wa.Sent()) != 0 {
		t.Fatal("replied while away")
	}
	h.advance(time.Minute + 9*time.Second)
	h.waitSent(1)
}

func TestRateLimitsAndCooldown(t *testing.T) {
	h := newHarness(t, both(func(p *model.BehaviorProfile) { p.MaxRepliesPerHour = 1 }), dmChat())
	h.msg(dmKey, "A", "one")
	h.advance(9 * time.Second)
	h.waitSent(1)
	h.msg(dmKey, "B", "two")
	if a := h.acts(model.ActDecisionSkip); len(a) != 1 || a[0].Meta["reason"] != "rate_limit" || a[0].Meta["window"] != "hour" {
		t.Fatalf("skip = %+v", a)
	}
	// Counters survive a runner restart and are on disk.
	h.st.UpdateChat(dmKey, func(c *model.ChatAssignment) { c.Enabled = false })
	h.e.Reload()
	h.st.UpdateChat(dmKey, func(c *model.ChatAssignment) { c.Enabled = true })
	h.e.Reload()
	h.msg(dmKey, "C", "three")
	if a := h.acts(model.ActDecisionSkip); len(a) != 2 {
		t.Fatalf("limit lost on reload: %+v", a)
	}
	st2, err := store.Open(h.cfg.Paths(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(st2.RunnerState(dmKey).Replies) != 1 {
		t.Error("runtime.json not written")
	}
	h.advance(time.Hour + time.Second) // the rolling hour has passed
	h.msg(dmKey, "D", "four")
	h.advance(9 * time.Second)
	h.waitSent(2)

	// Cooldown: a reply within cooldownSec of the last waits.
	h2 := newHarness(t, both(func(p *model.BehaviorProfile) { p.CooldownSec, p.WaitForMoreSec = 120, 5 }), dmChat())
	h2.msg(dmKey, "A", "one")
	h2.advance(5 * time.Second)
	h2.waitSent(1)
	h2.advance(5 * time.Second) // t=10
	h2.msg(dmKey, "B", "two")
	w := h2.acts(model.ActWaiting)
	if lastMeta(w, "reason") != "cooldown" || lastMeta(w, "waitSeconds") != 115.0 {
		t.Fatalf("cooldown wait = %+v", w[len(w)-1])
	}
	h2.advance(114 * time.Second)
	if len(h2.wa.Sent()) != 1 {
		t.Fatal("cooldown ignored")
	}
	h2.advance(time.Second)
	h2.waitSent(2)
}

func TestProactiveCheckIn(t *testing.T) {
	pro := both(func(p *model.BehaviorProfile) {
		p.Proactive = model.Proactive{Enabled: true, AfterHours: 2, MaxPerDay: 1, SpreadMinutes: 30}
	})
	h := newHarnessDeps(t, pro, Deps{ProactiveEvery: 10 * time.Minute}, dmChat())
	h.advance(3 * time.Hour)
	if len(h.acts(model.ActProactive)) != 0 {
		t.Fatal("no prior history: no check-in")
	}
	h.msg(dmKey, "A", "talk later!")
	h.advance(9 * time.Second)
	h.waitSent(1)
	h.advance(2*time.Hour + 10*time.Minute) // silence threshold passes on a tick
	p := h.acts(model.ActProactive)
	if len(p) == 0 || p[0].Meta["stage"] != "starting" {
		// Fixed{Frac:0} → spread 0: starts on the same tick.
		t.Fatalf("proactive acts = %+v", p)
	}
	sent := h.waitSent(2)
	reqs := replyRequests(h.llm)
	last := reqs[len(reqs)-1]
	// Check-ins get the silence-aware opener note (prompt.Opener, opener.go).
	if !strings.Contains(last.System, "The chat has gone quiet for 2 hours.") || last.Messages[len(last.Messages)-1].Role != llm.RoleUser {
		t.Errorf("check-in prompt: %q", last.System[len(last.System)-80:])
	}
	if a := h.acts(model.ActSent); a[len(a)-1].Meta["proactive"] != true || sent[1].text == "" {
		t.Errorf("sent meta = %+v", a[len(a)-1].Meta)
	}
	if st := h.st.RunnerState(dmKey); len(st.Proactive) != 1 {
		t.Errorf("proactive log = %+v", st.Proactive)
	}
	h.advance(5 * time.Hour) // maxPerDay 1
	if len(h.wa.Sent()) != 2 {
		t.Errorf("maxPerDay exceeded: %d", len(h.wa.Sent()))
	}

	// Spread: scheduled first, sent when due.
	h3 := newHarnessDeps(t, pro, Deps{ProactiveEvery: 10 * time.Minute}, dmChat())
	h3.rng.set(behavior.Fixed{Frac: 1})
	h3.msg(dmKey, "A", "bye")
	h3.rng.set(behavior.Fixed{})
	h3.advance(9 * time.Second)
	h3.waitSent(1)
	h3.rng.set(behavior.Fixed{Frac: 1}) // spread → 30 min
	h3.advance(2*time.Hour + 10*time.Minute)
	if p := h3.acts(model.ActProactive); len(p) != 1 || p[0].Meta["stage"] != "scheduled" {
		t.Fatalf("scheduled = %+v", p)
	}
	h3.rng.set(behavior.Fixed{})
	h3.advance(40 * time.Minute)
	if p := h3.acts(model.ActProactive); len(p) != 2 || p[1].Meta["stage"] != "starting" {
		t.Fatalf("starting = %+v", p)
	}
	h3.waitSent(2)

	// Approval mode queues the opener.
	c := dmChat()
	c.ApprovalMode = true
	h4 := newHarnessDeps(t, pro, Deps{ProactiveEvery: 10 * time.Minute}, c)
	if err := h4.st.AppendHistory(dmKey, model.Message{Speaker: "them", Text: "see ya", TS: h4.clk.Now()}, 0); err != nil {
		t.Fatal(err)
	}
	h4.e.Reload()
	h4.st.UpdateChat(dmKey, func(c *model.ChatAssignment) { c.Enabled = false })
	h4.e.Reload()
	h4.st.UpdateChat(dmKey, func(c *model.ChatAssignment) { c.Enabled = true })
	h4.e.Reload() // runner reloads the history from disk
	h4.advance(2*time.Hour + 10*time.Minute)
	eventually(t, "proactive approval", func() bool { return len(h4.st.Approvals()) == 1 })
	if len(h4.wa.Sent()) != 0 {
		t.Error("approval mode must not send")
	}

	// Off by default: no scan timer at all.
	h5 := newHarness(t, nil, dmChat())
	if h5.clk.Pending() != 0 {
		t.Error("proactive scan should not run when disabled")
	}
}

func TestApprovalSendsWithCappedTypingAndSplit(t *testing.T) {
	c := dmChat()
	c.ApprovalMode = true
	h := newHarness(t, both(func(p *model.BehaviorProfile) {
		p.SplitPercent, p.SplitMaxParts = 100, 3
		p.BubbleGapMinSec, p.BubbleGapMaxSec = 10, 10
		p.TypingCharsPerSec, p.TypingJitterPercent, p.TypingMinSec, p.TypingMaxSec = 1, 0, 0, 120
		p.ThinkMinSec, p.ThinkMaxSec = 30, 30
	}), c)
	h.simulate(dmKey, "dinner?", false)
	h.advance(9 * time.Second) // approval mode skips think time
	eventually(t, "pending", func() bool { return len(h.st.Approvals()) == 1 })
	if len(h.acts(model.ActThinking)) != 0 || len(h.acts(model.ActGenerating)) != 1 {
		t.Errorf("approval: acts %v", h.actTypes())
	}
	id := h.st.Approvals()[0].ID
	text := "Honestly I loved it. The second half dragged a little though. Want to go again next week?"
	done := make(chan error, 1)
	go func() { done <- h.e.SendApproved(context.Background(), id, text, -1) }()
	eventually(t, "typing", func() bool { return len(h.acts(model.ActTyping)) == 1 })
	h.advance(3*time.Second + 3*time.Second + 3*time.Second + 3*time.Second + 3*time.Second)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if s := h.wa.Sent(); len(s) != 3 {
		t.Fatalf("sent = %+v", s)
	}
	for _, a := range h.acts(model.ActTyping) {
		if a.Meta["typingSeconds"].(float64) > 3 {
			t.Errorf("typing not capped: %+v", a.Meta)
		}
	}
	if a := h.acts(model.ActApprovalSent); len(a) != 3 || a[2].Meta["parts"] != 3 || len(h.st.Approvals()) != 0 {
		t.Errorf("approval.sent = %+v", a)
	}
}

func TestTriggerBypassesDelays(t *testing.T) {
	h := newHarness(t, both(func(p *model.BehaviorProfile) {
		realistic(p)
		p.NoticeMinSec, p.NoticeMaxSec = 600, 600
		p.ThinkMinSec, p.ThinkMaxSec = 120, 120
		p.ReplyPercent = 0
		p.MaxRepliesPerHour = 1
		p.TypingCharsPerSec = 1
		p.Availability = model.Availability{Enabled: true, OutsideHours: "silent", Week: behavior.NormalizeWeek([]model.DayHours{{Day: "mon"}})}
	}), dmChat())
	h.simulate(dmKey, "1 quick test", true)
	if a := h.acts(model.ActGenerating); len(a) != 1 || len(h.acts(model.ActNoticing)) != 0 {
		t.Fatalf("trigger acts = %v", h.actTypes())
	}
	h.advance(3 * time.Second) // typing capped at 3 s
	h.waitSent(1)
	h.simulate(dmKey, "1 again", true)
	h.advance(3 * time.Second)
	h.waitSent(2) // limits don't apply either
}

func TestGroupDecisionFlags(t *testing.T) {
	h := newHarness(t, nil, groupChat())
	var calls int
	h.llm.SetFunc(func(r llm.Request) (llm.Response, error) {
		if r.MaxTokens == 5 {
			calls++
			return llm.Response{Text: "NO"}, nil
		}
		return llm.Response{Text: "sure"}, nil
	})
	set := func(fn func(o *model.BehaviorOverrides)) {
		h.st.UpdateChat(groupKey, func(c *model.ChatAssignment) { c.Behavior = model.BehaviorOverrides{}; fn(&c.Behavior) })
		h.e.Reload()
	}
	reason := func() any {
		all, _ := h.hub.ActivitySince(0, "", map[string]bool{model.ActDecisionSkip: true, model.ActDecisionReply: true}, 1000)
		last := all[len(all)-1]
		if strings.Contains(last.Text, "_") {
			t.Errorf("decision text exposes a code: %q", last.Text)
		}
		return last.Meta["reason"]
	}
	off, on := false, true
	set(func(o *model.BehaviorOverrides) { o.ReplyWhenNameMentioned = &off })
	h.groupMsg("1", "Leo what do you think")
	if reason() != ReasonLLMNo || calls != 1 {
		t.Errorf("name flag off → judgement: %v", reason())
	}
	set(func(o *model.BehaviorOverrides) { o.ReplyWhenAtMentioned = &off })
	h.e.HandleIncoming(model.Incoming{ChatKey: groupKey, IsGroup: true, PushName: "Avi", Text: "@me?", MentionedJIDs: []string{"972509999999@s.whatsapp.net"}, Timestamp: h.clk.Now()})
	h.idle()
	if reason() != ReasonLLMNo || calls != 2 {
		t.Errorf("@me flag off → judgement: %v", reason())
	}
	set(func(o *model.BehaviorOverrides) { o.SkipWhenOthersMentioned = &off })
	h.e.HandleIncoming(model.Incoming{ChatKey: groupKey, IsGroup: true, PushName: "Avi", Text: "@Noa?", MentionedJIDs: []string{"972502222222@s.whatsapp.net"}, Timestamp: h.clk.Now()})
	h.idle()
	if reason() != ReasonLLMNo || calls != 3 {
		t.Errorf("skip-others off → judgement: %v", reason())
	}
	zero := 0
	set(func(o *model.BehaviorOverrides) { o.AIJudgement = &off; o.ChimeInPercent = &zero })
	h.groupMsg("4", "random chatter")
	if reason() != ReasonChimeOut || calls != 3 {
		t.Errorf("aiJudgement off → no LLM call: %v %d", reason(), calls)
	}
	h.rng.set(behavior.Fixed{Hits: true})
	forty := 40
	set(func(o *model.BehaviorOverrides) {
		o.AIJudgement = &off
		o.ChimeInPercent = &forty
		o.ReplyWhenNameMentioned = &on
	})
	h.groupMsg("5", "more chatter")
	if reason() != ReasonChimeIn || calls != 3 {
		t.Errorf("chime-in roll: %v", reason())
	}
}

func TestNoTimerLeaksAfterStop(t *testing.T) {
	slow := both(func(p *model.BehaviorProfile) {
		realistic(p)
		p.TypingCharsPerSec, p.TypingMaxSec = 1, 120
		p.Proactive = model.Proactive{Enabled: true, AfterHours: 1, MaxPerDay: 1}
	})
	h := newHarness(t, slow, dmChat(), groupChat())
	h.llm.Push(strings.Repeat("z", 60))
	h.msg(dmKey, "A", "hi")
	h.groupMsg("G", "Leo hi")                                                 // pending notice timer in the group
	h.advance(5*time.Second + 8*time.Second + 4*time.Second + 10*time.Second) // mid-typing in the DM
	dmTyping := 0
	for _, a := range h.acts(model.ActTyping) {
		if a.ChatKey == dmKey {
			dmTyping++
		}
	}
	if dmTyping != 1 || len(h.wa.Sent()) != 0 {
		t.Fatalf("expected to be mid-typing: %v", h.actTypes())
	}
	// Disable mid-typing: its timers go away, typing is cleared.
	h.st.UpdateChat(dmKey, func(c *model.ChatAssignment) { c.Enabled = false })
	h.e.Reload()
	h.idle()
	var lastDM *typingCall
	for _, ty := range h.wa.Typings() {
		if ty.jid == dmJID {
			lastDM = &ty
		}
	}
	if lastDM == nil || lastDM.on {
		t.Error("typing left on after disabling")
	}
	h.e.Stop()
	h.idle()
	// Sleeping deliveries stop their timers when they see the cancelled
	// context, which happens asynchronously after Stop returns.
	deadline := time.Now().Add(2 * time.Second)
	for h.clk.Pending() != 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if n := h.clk.Pending(); n != 0 {
		t.Errorf("%d timers left after Stop", n)
	}
	if h.e.busy.Load() != 0 {
		t.Errorf("busy = %d", h.e.busy.Load())
	}
	h.clk.Advance(time.Hour)
	if len(h.wa.Sent()) != 0 {
		t.Error("sent after Stop")
	}
}
