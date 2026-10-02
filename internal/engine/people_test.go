package engine

import (
	"slices"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
)

// Group members used by the mention / people tests. Dana is lid-addressed.
const (
	danaPN  = "972501234567@s.whatsapp.net"
	danaLID = "200000000000001@lid"
	noamPN  = "972501111112@s.whatsapp.net"
	aviPN   = "972502222222@s.whatsapp.net"
)

func smallRoster() []model.Participant {
	return []model.Participant{
		{JID: ownJID, Phone: "972509999999", Name: "Me", IsSelf: true},
		{JID: danaLID, Phone: "972501234567", LID: danaLID, Name: "Dana Levi"},
		{JID: noamPN, Phone: "972501111112", Name: "Noam"},
		{JID: aviPN, Phone: "972502222222", Name: "Avi"},
	}
}

func bigRoster() []model.Participant {
	r := smallRoster()
	for i := range 10 {
		p := "97250333333" + string(rune('0'+i))
		r = append(r, model.Participant{JID: p + "@s.whatsapp.net", Phone: p, Name: "Member " + string(rune('A'+i))})
	}
	return r
}

// replyWith makes the fake LLM answer reply prompts with text and group
// decisions with YES.
func replyWith(h *harness, text string) {
	h.llm.SetFunc(func(req llm.Request) (llm.Response, error) {
		if strings.Contains(req.System, "CRITICAL SECURITY RULES") {
			return llm.Response{Text: text, Model: req.Model}, nil
		}
		all := req.System
		for _, m := range req.Messages {
			all += m.Content
		}
		if strings.Contains(all, "YES or NO") {
			return llm.Response{Text: "YES"}, nil
		}
		return llm.Response{Text: "{}"}, nil
	})
}

func (h *harness) groupFrom(sender, name, id, text string, mentioned ...string) {
	h.t.Helper()
	h.e.HandleIncoming(model.Incoming{ChatKey: groupKey, ChatJID: groupJID, IsGroup: true, SenderJID: sender,
		PushName: name, Text: text, Timestamp: h.clk.Now(), MessageID: id, MentionedJIDs: mentioned})
	h.idle()
}

func (h *harness) lastReason(typ string) any { return lastMeta(h.acts(typ), "reason") }

func groupReplies(h *harness, mut func(p *model.BehaviorProfile)) func(*config.Settings) {
	return func(s *config.Settings) {
		g := &s.Behavior.Group
		g.ChimeInPercent, g.AIJudgement = 100, false // answer everything unless a rule says otherwise
		if mut != nil {
			mut(g)
		}
	}
}

func TestMentionsEncodedOnSend(t *testing.T) {
	h := newHarness(t, groupReplies(nil, nil), groupChat())
	h.wa.setRoster(groupJID, smallRoster())
	replyWith(h, "@dana sure, and @Nobody too")
	h.groupFrom(aviPN, "Avi", "G1", "what should we eat")
	h.advance(10 * time.Second)
	sent := h.wa.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent = %+v", sent)
	}
	if sent[0].text != "@200000000000001 sure, and Nobody too" || !slices.Equal(sent[0].mentions, []string{danaLID}) {
		t.Errorf("wire = %q %v", sent[0].text, sent[0].mentions)
	}
	hist := h.history(groupKey)
	last := hist[len(hist)-1]
	if last.Text != "@Dana sure, and Nobody too" || !slices.Equal(last.Mentions, []string{"Dana"}) {
		t.Errorf("history = %+v", last)
	}
	if hist[0].SenderJID != aviPN {
		t.Errorf("incoming sender not kept: %+v", hist[0])
	}
	if m := lastMeta(h.acts(model.ActSent), "mentions"); !slices.Equal(m.([]string), []string{"Dana"}) {
		t.Errorf("sent meta mentions = %v", m)
	}
	reqs := replyRequests(h.llm)
	if sys := reqs[len(reqs)-1].System; !strings.Contains(sys, "PEOPLE IN THIS GROUP: Avi, Dana, Noam") || !strings.Contains(sys, "e.g. @Avi") {
		t.Errorf("prompt lacks the member list (recent speaker first): %q", sys)
	}

	// The echo of the tagged message (wire text) is recognised as ours.
	h.e.HandleIncoming(model.Incoming{ChatKey: groupKey, ChatJID: groupJID, IsGroup: true, IsFromMe: true, SenderJID: ownJID,
		Text: sent[0].text, MentionedJIDs: []string{danaLID}, Timestamp: h.clk.Now(), MessageID: "E1"})
	h.idle()
	if n := len(h.history(groupKey)); n != len(hist) {
		t.Errorf("echo should be ignored, history grew to %d", n)
	}
}

func TestTagReplyPercent(t *testing.T) {
	h := newHarness(t, groupReplies(nil, func(p *model.BehaviorProfile) { p.TagReplyPercent = 100 }), groupChat())
	h.wa.setRoster(groupJID, smallRoster())
	replyWith(h, "Dana, absolutely")
	// One speaker: no tag.
	h.groupFrom(danaLID, "Dana", "G1", "pizza tonight?")
	h.advance(10 * time.Second)
	if s := h.wa.Sent(); len(s) != 1 || s[0].text != "Dana, absolutely" || len(s[0].mentions) != 0 {
		t.Fatalf("one speaker: %+v", s)
	}
	// Two people talking: the reply addresses the person it answers.
	h.groupFrom(noamPN, "Noam", "G2", "I'm in")
	h.groupFrom(danaLID, "Dana", "G3", "great, 8pm?")
	h.advance(10 * time.Second)
	s := h.wa.Sent()
	if len(s) != 2 || s[1].text != "@200000000000001 absolutely" || !slices.Equal(s[1].mentions, []string{danaLID}) {
		t.Fatalf("two speakers: %+v", s)
	}
	// Already tagging someone up to mentionMax: no extra tag.
	replyWith(h, "@Noam you bring drinks")
	h.groupFrom(danaLID, "Dana", "G4", "who brings drinks?")
	h.advance(10 * time.Second)
	if s := h.wa.Sent(); len(s) != 3 || s[2].text != "@972501111112 you bring drinks" || len(s[2].mentions) != 1 {
		t.Fatalf("mentionMax respected: %+v", s[2])
	}
}

func TestAllowMentionsOff(t *testing.T) {
	h := newHarness(t, groupReplies(nil, func(p *model.BehaviorProfile) { p.AllowMentions, p.TagReplyPercent = false, 100 }), groupChat())
	h.wa.setRoster(groupJID, smallRoster())
	replyWith(h, "@Dana hi there")
	h.groupFrom(noamPN, "Noam", "G1", "hello")
	h.groupFrom(danaLID, "Dana", "G2", "hey all")
	h.advance(10 * time.Second)
	if s := h.wa.Sent(); len(s) != 1 || s[0].text != "Dana hi there" || len(s[0].mentions) != 0 {
		t.Fatalf("sent = %+v", s)
	}
	if sys := replyRequests(h.llm)[0].System; !strings.Contains(sys, "Never put @ in front of anyone's name.") {
		t.Error("prompt should forbid tags")
	}
}

func TestIncomingMentionHumanized(t *testing.T) {
	h := newHarness(t, func(s *config.Settings) {
		s.Behavior.Group.ChimeInPercent, s.Behavior.Group.AIJudgement = 0, false
	}, groupChat())
	h.wa.setRoster(groupJID, smallRoster())
	h.groupFrom(noamPN, "Noam", "G1", "@972509999999 and @972501234567 what do you think?", ownJID, danaPN)
	if r := h.lastReason(model.ActDecisionReply); r != ReasonMentionedMe {
		t.Fatalf("decision = %v (acts %v)", r, h.actTypes())
	}
	hist := h.history(groupKey)
	if hist[0].Text != "@Leo and @Dana what do you think?" || !slices.Equal(hist[0].Mentions, []string{"Leo", "Dana"}) {
		t.Errorf("history = %+v", hist[0])
	}
	if in := h.acts(model.ActIncoming); in[0].Text != "@Leo and @Dana what do you think?" {
		t.Errorf("incoming activity = %q", in[0].Text)
	}
}

func TestApprovalMentions(t *testing.T) {
	c := groupChat()
	c.ApprovalMode = true
	h := newHarness(t, groupReplies(nil, nil), c)
	h.wa.setRoster(groupJID, smallRoster())
	replyWith(h, "@Dana see you there")
	h.groupFrom(aviPN, "Avi", "G1", "party at 9")
	h.advance(10 * time.Second)
	aps := h.st.Approvals()
	if len(aps) != 1 || aps[0].Text != "@Dana see you there" || !slices.Equal(aps[0].Mentions, []string{"Dana"}) {
		t.Fatalf("approvals = %+v", aps)
	}
	if err := h.e.SendApproved(t.Context(), aps[0].ID, "@noam and @Dana see you there", -1); err != nil {
		t.Fatal(err)
	}
	s := h.wa.Sent()
	if len(s) != 1 || s[0].text != "@972501111112 and @200000000000001 see you there" || len(s[0].mentions) != 2 {
		t.Fatalf("sent = %+v", s)
	}
	hist := h.history(groupKey)
	if last := hist[len(hist)-1]; last.Text != "@Noam and @Dana see you there" || len(last.Mentions) != 2 {
		t.Errorf("history = %+v", last)
	}
}

func TestSendManualEncodes(t *testing.T) {
	h := newHarness(t, nil, groupChat())
	h.wa.setRoster(groupJID, smallRoster())
	if err := h.e.SendManual(t.Context(), groupKey, "@Avi @Noam hi both"); err != nil {
		t.Fatal(err)
	}
	if s := h.wa.Sent(); len(s) != 1 || s[0].text != "@972502222222 @972501111112 hi both" || len(s[0].mentions) != 2 {
		t.Fatalf("sent = %+v", s)
	}
}

func TestSimulatePicksParticipant(t *testing.T) {
	h := newHarness(t, groupReplies(nil, nil), groupChat())
	h.wa.setRoster(groupJID, smallRoster())
	h.simulate(groupKey, "hello", false)
	hist := h.history(groupKey)
	if hist[0].Name != "Dana Levi" || hist[0].SenderJID != danaLID {
		t.Errorf("random pick (Fixed → first member) = %+v", hist[0])
	}
	if err := h.e.Simulate(groupKey, "hi", false, noamPN); err != nil {
		t.Fatal(err)
	}
	h.idle()
	hist = h.history(groupKey)
	if m := hist[len(hist)-1]; m.Name != "Noam" || m.SenderJID != noamPN {
		t.Errorf("chosen sender = %+v", m)
	}
	h2 := newHarness(t, groupReplies(nil, nil), groupChat()) // no roster
	h2.simulate(groupKey, "hello", false)
	if m := h2.history(groupKey)[0]; m.Name != "Tester" {
		t.Errorf("fallback = %+v", m)
	}
}

func TestPeopleNotSelectedInBigGroup(t *testing.T) {
	h := newHarness(t, groupReplies(nil, nil), groupChat())
	h.wa.setRoster(groupJID, bigRoster())
	replyWith(h, "sure")
	h.groupFrom(aviPN, "Avi", "G1", "anyone around?")
	if r := h.lastReason(model.ActDecisionSkip); r != ReasonNotSelected {
		t.Fatalf("big group, nobody picked: %v (%v)", r, h.actTypes())
	}
	if hist := h.history(groupKey); len(hist) != 1 || hist[0].Text != "anyone around?" {
		t.Errorf("skipped message should be kept as context: %+v", hist)
	}
	if a := h.acts(model.ActDecisionSkip); !strings.Contains(a[0].Text, "Not answering Avi") || strings.Contains(a[0].Text, "not_selected") {
		t.Errorf("skip text = %q", a[0].Text)
	}
	// Saying its name still gets an answer (answerAnyoneWhoAddressesIt).
	h.groupFrom(aviPN, "Avi", "G2", "Leo, are you around?")
	h.advance(10 * time.Second)
	if len(h.wa.Sent()) != 1 {
		t.Fatalf("addressed message should be answered: %v", h.actTypes())
	}
	// Picking Avi (saved with his phone, he writes from the same number) makes him answerable.
	c := groupChat()
	c.People = model.PeopleConfig{People: []model.PersonPrefs{{JID: aviPN, Respond: boolPtr(true)}}}
	if _, err := h.st.UpsertChat(c); err != nil {
		t.Fatal(err)
	}
	h.e.Reload()
	h.groupFrom(aviPN, "Avi", "G3", "what's the plan")
	h.advance(10 * time.Second)
	if len(h.wa.Sent()) != 2 {
		t.Fatalf("picked person should be answered: %v", h.actTypes())
	}
	// "Everyone" mode answers the rest; a person set to "don't answer" is skipped.
	c.People = model.PeopleConfig{Mode: model.PeopleEveryone, People: []model.PersonPrefs{{JID: danaPN, Respond: boolPtr(false)}}}
	_, _ = h.st.UpsertChat(c)
	h.e.Reload()
	h.groupFrom(danaLID, "Dana", "G4", "hello?") // matched through the roster (lid ↔ phone)
	if r := h.lastReason(model.ActDecisionSkip); r != ReasonNotSelected {
		t.Errorf("Dana is set to not answer: %v", r)
	}
	h.groupFrom(noamPN, "Noam", "G5", "hi all")
	h.advance(10 * time.Second)
	if len(h.wa.Sent()) != 3 {
		t.Errorf("everyone mode should answer Noam: %v", h.actTypes())
	}
}

func boolPtr(b bool) *bool { return &b }

func TestPriorityPersonBypassesChance(t *testing.T) {
	c := groupChat()
	c.People = model.PeopleConfig{People: []model.PersonPrefs{{JID: noamPN, Priority: true}}}
	h := newHarness(t, func(s *config.Settings) {
		g := &s.Behavior.Group
		g.ChimeInPercent, g.AIJudgement, g.ReplyPercent = 0, false, 0
	}, c)
	h.wa.setRoster(groupJID, smallRoster())
	replyWith(h, "on it")
	h.groupFrom(aviPN, "Avi", "G1", "random chat")
	if r := h.lastReason(model.ActDecisionSkip); r != ReasonChimeOut {
		t.Fatalf("Avi: %v", r)
	}
	h.groupFrom(noamPN, "Noam", "G2", "can you check this")
	if r := h.lastReason(model.ActDecisionReply); r != ReasonPriorityPerson {
		t.Fatalf("Noam: %v", r)
	}
	h.advance(10 * time.Second)
	if len(h.wa.Sent()) != 1 {
		t.Errorf("priority person should be answered despite reply chance 0: %v", h.actTypes())
	}
	// In a big group where only picked people are answered, a starred person still is.
	h.wa.setRoster(groupJID, bigRoster())
	h.groupFrom(noamPN, "Noam", "G3", "and this?")
	if r := h.lastReason(model.ActDecisionReply); r != ReasonPriorityPerson {
		t.Fatalf("Noam in a big group: %v", r)
	}
}

func TestMuteBeatsTriggerWords(t *testing.T) {
	h := newHarness(t, func(s *config.Settings) {
		for _, p := range []*model.BehaviorProfile{&s.Behavior.Private, &s.Behavior.Group} {
			p.ReplyPercent = 0
			p.TriggerWords = []string{"pizza"}
			p.MuteWords = []string{"spoiler"}
		}
	}, dmChat())
	replyWith(h, "yes!")
	h.msg(dmKey, "D1", "PIZZA tonight?")
	if r := h.lastReason(model.ActDecisionReply); r != ReasonTriggerWord {
		t.Fatalf("trigger: %v (%v)", r, h.actTypes())
	}
	h.advance(10 * time.Second)
	if len(h.wa.Sent()) != 1 {
		t.Fatalf("trigger word should force a reply despite reply chance 0")
	}
	h.msg(dmKey, "D2", "pizza spoiler: the ending")
	if r := h.lastReason(model.ActDecisionSkip); r != ReasonMuteWord {
		t.Fatalf("mute: %v", r)
	}
	h.advance(10 * time.Second)
	if len(h.wa.Sent()) != 1 {
		t.Error("mute word must win over the trigger word")
	}
	if hist := h.history(dmKey); hist[len(hist)-1].Text != "pizza spoiler: the ending" {
		t.Error("muted message should be kept as context")
	}
}

func TestStreakGuard(t *testing.T) {
	h := newHarness(t, groupReplies(nil, func(p *model.BehaviorProfile) { p.MaxStreak = 2 }), groupChat())
	h.wa.setRoster(groupJID, smallRoster())
	replyWith(h, "ok")
	for i, id := range []string{"B1", "B2"} {
		h.groupFrom(aviPN, "Avi", id, "ping")
		h.advance(10 * time.Second)
		if n := len(h.wa.Sent()); n != i+1 {
			t.Fatalf("reply %d not sent", i+1)
		}
	}
	h.groupFrom(aviPN, "Avi", "B3", "ping")
	if r := h.lastReason(model.ActDecisionSkip); r != ReasonStreakLimit {
		t.Fatalf("third in a row: %v (%v)", r, h.actTypes())
	}
	// Someone else speaks: the count starts over.
	h.groupFrom(noamPN, "Noam", "B4", "hey")
	h.advance(10 * time.Second)
	h.groupFrom(aviPN, "Avi", "B5", "ping")
	h.advance(10 * time.Second)
	if n := len(h.wa.Sent()); n != 4 {
		t.Fatalf("after someone else spoke: sent %d", n)
	}
	h.groupFrom(aviPN, "Avi", "B6", "ping")
	h.advance(10 * time.Second)
	h.groupFrom(aviPN, "Avi", "B7", "ping")
	if r := h.lastReason(model.ActDecisionSkip); r != ReasonStreakLimit || len(h.wa.Sent()) != 5 {
		t.Fatalf("streak again: %v, sent %d", r, len(h.wa.Sent()))
	}
	// A long quiet resets it too.
	h.advance(31 * time.Minute)
	h.groupFrom(aviPN, "Avi", "B8", "ping")
	h.advance(10 * time.Second)
	if n := len(h.wa.Sent()); n != 6 {
		t.Errorf("after 30 min of quiet: sent %d", n)
	}
}

func TestPeopleNotesInPrompt(t *testing.T) {
	c := groupChat()
	c.People = model.PeopleConfig{People: []model.PersonPrefs{{JID: noamPN, Name: "Noam", Notes: "my little brother"}}}
	h := newHarness(t, groupReplies(nil, nil), c)
	h.wa.setRoster(groupJID, smallRoster())
	replyWith(h, "ok")
	h.groupFrom(noamPN, "Noam", "N1", "hey")
	h.advance(10 * time.Second)
	if sys := replyRequests(h.llm)[0].System; !strings.Contains(sys, "ABOUT THE PEOPLE HERE:\n- Noam: my little brother") {
		t.Errorf("notes missing: %q", sys)
	}
	d := dmChat()
	d.People = model.PeopleConfig{People: []model.PersonPrefs{{JID: dmJID, Notes: "my sister"}}}
	h2 := newHarness(t, nil, d)
	sys, err := h2.e.PromptPreview("leo", dmKey)
	if err != nil || !strings.Contains(sys, "ABOUT THE PERSON YOU'RE TALKING TO: my sister") {
		t.Errorf("dm preview: %v %q", err, sys)
	}
}

func TestPlaygroundGroupSpeakers(t *testing.T) {
	h := newHarness(t, groupReplies(nil, func(p *model.BehaviorProfile) { p.TagReplyPercent = 100 }), nil...)
	replyWith(h, "sounds good")
	pg := h.e.Playground()
	id, err := pg.Start("leo")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := pg.Send(t.Context(), id, "first", true)
	b, _ := pg.Send(t.Context(), id, "second", true)
	if a.Speaker != "Dana" || b.Speaker != "Noam" {
		t.Fatalf("speakers %q %q", a.Speaker, b.Speaker)
	}
	if b.Reply != "@Noam sounds good" || !slices.Equal(b.Mentions, []string{"Noam"}) || a.Reply != "sounds good" {
		t.Errorf("replies %q / %q %v", a.Reply, b.Reply, b.Mentions)
	}
	dm, _ := pg.Send(t.Context(), id, "dm", false)
	if dm.Speaker != "" || len(dm.Mentions) != 0 {
		t.Errorf("dm mode: %+v", dm)
	}
}
