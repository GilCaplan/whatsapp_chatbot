package engine

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
)

const (
	crossHeadGroup = "WHAT YOU KNOW FROM PRIVATE CHATS"
	crossHeadDM    = "WHAT YOU KNOW FROM GROUPS YOU SHARE"
	nightFact      = "works night shifts at Ichilov"
)

// crossRoster: Dana (the DM contact, seen by the group through her lid), Avi, you.
func crossRoster() []model.Participant {
	return []model.Participant{
		{JID: ownJID, Phone: "972509999999", Name: "Me", IsSelf: true},
		{JID: dmLID, Phone: "972501111111", LID: dmLID, Name: "Dana"},
		{JID: aviPN, Phone: "972502222222", Name: "Avi"},
	}
}

// newCrossHarness: Leo in the group "Friends" and in a private chat with
// Dana, who told it something there.
func newCrossHarness(t *testing.T, mut func(*config.Settings), chats ...model.ChatAssignment) *harness {
	t.Helper()
	if len(chats) == 0 {
		chats = []model.ChatAssignment{groupChat(), dmChat()}
	}
	h := newHarness(t, func(s *config.Settings) {
		groupReplies(nil, nil)(s)
		if mut != nil {
			mut(s)
		}
	}, chats...)
	h.wa.setRoster(groupJID, crossRoster())
	h.addMemory(dmKey, nightFact, nil)
	return h
}

func (h *harness) addMemory(key, text string, fn func(m *model.Memory)) model.Memory {
	h.t.Helper()
	now := h.clk.Now()
	m := model.Memory{ChatKey: key, Person: "Dana", PersonJID: dmJID, Text: text, Kind: model.MemoryFact,
		Source: model.MemorySourceLearned, CreatedAt: now, UpdatedAt: now}
	if fn != nil {
		fn(&m)
	}
	saved, err := h.st.UpsertMemory(m)
	if err != nil {
		h.t.Fatal(err)
	}
	return saved
}

// captureReplies answers reply requests with answer(system) and records the systems.
func captureReplies(h *harness, answer func(sys string) string) func() []string {
	var mu sync.Mutex
	var systems []string
	h.llm.SetFunc(func(req llm.Request) (llm.Response, error) {
		if strings.Contains(req.System, "CRITICAL SECURITY RULES") {
			mu.Lock()
			systems = append(systems, req.System)
			mu.Unlock()
			return llm.Response{Text: answer(req.System), Model: req.Model}, nil
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
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(systems)
	}
}

func fixed(text string) func(string) string { return func(string) string { return text } }

func (h *harness) danaInGroup(id, text string) {
	h.t.Helper()
	h.groupFrom(dmLID, "Dana", id, text)
	h.advance(10 * time.Second)
}

func TestCrossGroupUsesPrivateChat(t *testing.T) {
	h := newCrossHarness(t, nil)
	systems := captureReplies(h, fixed("haha drinks sound good"))
	h.danaInGroup("G1", "anyone up for drinks tonight?")
	sys := systems()
	if len(sys) != 1 {
		t.Fatalf("reply requests = %d (%v)", len(sys), h.actTypes())
	}
	if !strings.Contains(sys[0], crossHeadGroup+" (background only") || !strings.Contains(sys[0], "Dana (private — do not bring up):\n- "+nightFact) {
		t.Fatalf("section missing:\n%s", sys[0])
	}
	var stage map[string]any
	for _, a := range h.acts(model.ActThinking) {
		if a.Meta["stage"] == "cross" {
			stage = a.Meta
			if a.Text != "Keeping in mind one thing from your private chat with Dana — discreet" || strings.Contains(a.Text, "Ichilov") {
				t.Errorf("activity text = %q", a.Text)
			}
		}
	}
	if stage == nil {
		t.Fatalf("no cross thinking act: %v", h.actTypes())
	}
	sent := h.acts(model.ActSent)
	if len(sent) != 1 {
		t.Fatalf("sent = %v", h.actTypes())
	}
	cc, _ := sent[0].Meta["crossContext"].(map[string]any)
	if cc == nil || cc["items"] != 1 || cc["mode"] != model.CrossDiscreet {
		t.Errorf("sent meta = %+v", sent[0].Meta)
	}
	hist := h.history(groupKey)
	if last := hist[len(hist)-1]; last.Speaker != "me" || !slices.Equal(last.CrossUsed, []string{"Dana"}) {
		t.Errorf("bubble = %+v", last)
	}
	// The preview shows what the persona gets.
	pv, err := h.e.PromptPreview("leo", groupKey)
	if err != nil || !strings.Contains(pv, nightFact) {
		t.Errorf("preview lacks the section: %v", err)
	}
}

func TestCrossSensitiveAndScope(t *testing.T) {
	h := newCrossHarness(t, nil)
	chemo := h.addMemory(dmKey, "started chemo last month", nil)
	jazz := h.addMemory(dmKey, "plays jazz piano", func(m *model.Memory) { m.Scope = model.MemoryScopeLocal })
	rent := h.addMemory(dmKey, "plans a trip to Rome", func(m *model.Memory) { m.Sensitive = model.HandoffMoney })
	systems := captureReplies(h, fixed("sounds fun"))
	h.danaInGroup("G1", "what's everyone doing later?")
	sys := systems()[0]
	for _, m := range []model.Memory{chemo, jazz, rent} {
		if strings.Contains(sys, m.Text) {
			t.Errorf("%q crossed", m.Text)
		}
	}
	if !strings.Contains(sys, nightFact) {
		t.Error("ordinary fact missing")
	}
	// Your unlock lets a sensitive memory cross.
	chemo.Scope = model.MemoryScopeShared
	if _, err := h.st.UpsertMemory(chemo); err != nil {
		t.Fatal(err)
	}
	h.danaInGroup("G2", "anyone?")
	all := systems()
	if !strings.Contains(all[len(all)-1], chemo.Text) {
		t.Error("unlocked memory missing")
	}
}

func TestCrossExclusions(t *testing.T) {
	off := false
	cases := map[string]struct {
		mut   func(*config.Settings)
		chats func() []model.ChatAssignment
	}{
		"settings off": {mut: func(s *config.Settings) { s.Memory.Cross.Enabled = false }},
		"group mode off": {chats: func() []model.ChatAssignment {
			g := groupChat()
			g.Cross.Mode = model.CrossOff
			return []model.ChatAssignment{g, dmChat()}
		}},
		"app group mode off": {mut: func(s *config.Settings) { s.Memory.Cross.GroupMode = model.CrossOff }},
		"source share off": {chats: func() []model.ChatAssignment {
			d := dmChat()
			d.Cross.Share = &off
			return []model.ChatAssignment{groupChat(), d}
		}},
		"person off": {chats: func() []model.ChatAssignment {
			g := groupChat()
			g.People.People = []model.PersonPrefs{{JID: dmJID, Cross: &off}}
			return []model.ChatAssignment{g, dmChat()}
		}},
		"source memory off": {chats: func() []model.ChatAssignment {
			d := dmChat()
			d.Memory = &off
			return []model.ChatAssignment{groupChat(), d}
		}},
		"source handed off": {chats: func() []model.ChatAssignment {
			d := dmChat()
			d.Handoff = &model.HandoffState{Category: model.HandoffMoney}
			return []model.ChatAssignment{groupChat(), d}
		}},
		"source revealed": {chats: func() []model.ChatAssignment {
			d := dmChat()
			now := time.Now()
			d.RevealedAt = &now
			return []model.ChatAssignment{groupChat(), d}
		}},
		"other persona": {chats: func() []model.ChatAssignment {
			d := dmChat()
			d.PersonaID = "maya"
			return []model.ChatAssignment{groupChat(), d}
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var chats []model.ChatAssignment
			if c.chats != nil {
				chats = c.chats()
			}
			h := newCrossHarness(t, c.mut, chats...)
			systems := captureReplies(h, fixed("sure"))
			h.danaInGroup("G1", "hi all")
			sys := systems()
			if len(sys) == 0 {
				t.Fatalf("no reply: %v", h.actTypes())
			}
			if strings.Contains(sys[0], crossHeadGroup) || strings.Contains(sys[0], nightFact) {
				t.Errorf("section present")
			}
		})
	}
}

func TestCrossOnlyActivePeople(t *testing.T) {
	h := newCrossHarness(t, nil)
	systems := captureReplies(h, fixed("ok"))
	h.groupFrom(aviPN, "Avi", "G1", "what's up everyone") // Dana isn't talking
	h.advance(10 * time.Second)
	if sys := systems(); len(sys) != 1 || strings.Contains(sys[0], crossHeadGroup) {
		t.Fatalf("Dana is not in the conversation: %d", len(sys))
	}
	h.danaInGroup("G2", "hey!")
	if sys := systems(); !strings.Contains(sys[len(sys)-1], nightFact) {
		t.Error("Dana spoke: section expected")
	}
}

func TestCrossLeakGuardRewritesThenDrops(t *testing.T) {
	h := newCrossHarness(t, nil)
	leaky := "how are the night shifts at Ichilov treating you"
	systems := captureReplies(h, func(sys string) string {
		if !strings.Contains(sys, crossHeadGroup) {
			return "haha drinks sound good"
		}
		return leaky // even after the reminder
	})
	h.danaInGroup("G1", "anyone up for drinks tonight?")
	sys := systems()
	if len(sys) != 3 || strings.Contains(sys[0], prompt.CrossRetryNote) || !strings.Contains(sys[1], prompt.CrossRetryNote) || strings.Contains(sys[2], crossHeadGroup) {
		t.Fatalf("requests = %d", len(sys))
	}
	sent := h.wa.Sent()
	if len(sent) != 1 || sent[0].text != "haha drinks sound good" {
		t.Fatalf("sent = %+v", sent)
	}
	m := h.acts(model.ActSent)[0].Meta
	if m["crossRewritten"] != true || m["crossDropped"] != true {
		t.Errorf("meta = %+v", m)
	}
	hist := h.history(groupKey)
	if last := hist[len(hist)-1]; len(last.CrossUsed) != 0 {
		t.Errorf("written without context, yet CrossUsed = %v", last.CrossUsed)
	}
}

func TestCrossLeakGuardRetrySucceeds(t *testing.T) {
	h := newCrossHarness(t, nil)
	systems := captureReplies(h, func(sys string) string {
		if strings.Contains(sys, prompt.CrossRetryNote) {
			return "drinks? count me in"
		}
		return "still on night shifts at Ichilov?"
	})
	h.danaInGroup("G1", "anyone up for drinks tonight?")
	if n := len(systems()); n != 2 {
		t.Fatalf("requests = %d", n)
	}
	m := h.acts(model.ActSent)[0].Meta
	if m["crossRewritten"] != true || m["crossDropped"] != nil {
		t.Errorf("meta = %+v", m)
	}
}

func TestCrossOpenModeAllowsPresentPerson(t *testing.T) {
	g := groupChat()
	g.Cross.Mode = model.CrossOpen
	h := newCrossHarness(t, nil, g, dmChat())
	systems := captureReplies(h, fixed("like you said, night shifts at Ichilov are brutal"))
	h.danaInGroup("G1", "so tired today")
	sys := systems()
	if len(sys) != 1 || !strings.Contains(sys[0], "With THAT person you may refer") {
		t.Fatalf("open: %d requests", len(sys))
	}
	if m := h.acts(model.ActSent)[0].Meta; m["crossRewritten"] != nil {
		t.Errorf("present person: no rewrite expected: %+v", m)
	}
}

func TestCrossDMUsesSharedGroup(t *testing.T) {
	h := newCrossHarness(t, nil)
	now := h.clk.Now()
	if err := h.st.SaveBrief(model.Brief{ChatKey: groupKey, PersonaID: "leo", Kind: "group", Topics: []string{"Saturday brunch"},
		Commitments: []string{"book a table for Saturday"},
		People:      []model.BriefPerson{{Name: "Dana", JID: dmLID, Note: "pushed for Saturday, joked about hummus"}, {Name: "Avi", JID: aviPN, Note: "got a new job"}},
		GeneratedAt: now}); err != nil {
		t.Fatal(err)
	}
	h.addMemory(groupKey, "has a dog called Pita", func(m *model.Memory) { m.PersonJID = dmLID })
	h.addMemory(groupKey, "is moving to Haifa", func(m *model.Memory) { m.Person, m.PersonJID = "Avi", aviPN })
	systems := captureReplies(h, fixed("hey you!"))
	h.msg(dmKey, "D1", "hey")
	h.advance(15 * time.Second)
	sys := systems()
	if len(sys) != 1 {
		t.Fatalf("requests = %d (%v)", len(sys), h.actTypes())
	}
	s := sys[0]
	for _, want := range []string{crossHeadDM, `You and Dana are both in "Friends".`, "Friends (group):", "pushed for Saturday", "has a dog called Pita", "you said there: book a table for Saturday"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
	for _, not := range []string{"got a new job", "Haifa", nightFact + "\n- "} {
		if strings.Contains(s, not) {
			t.Errorf("other members' things crossed: %q", not)
		}
	}
	// A brief that touched something sensitive carries only its promises.
	b, _, _ := h.st.Brief(groupKey)
	b.Sensitive = []string{model.HandoffHealth}
	_ = h.st.SaveBrief(b)
	h.msg(dmKey, "D2", "u there?")
	h.advance(15 * time.Second)
	sys = systems()
	s = sys[len(sys)-1]
	if strings.Contains(s, "Saturday brunch") || strings.Contains(s, "hummus") || !strings.Contains(s, "book a table") {
		t.Errorf("sensitive brief:\n%s", s)
	}
}

func TestCrossView(t *testing.T) {
	off := false
	g := groupChat()
	g.People.People = []model.PersonPrefs{{JID: dmJID, Cross: &off}}
	h := newCrossHarness(t, nil, g, dmChat())
	v, err := h.e.CrossView(context.Background(), groupKey)
	if err != nil || v.Mode != model.CrossDiscreet || v.ModeSource != "default" || !v.Share || len(v.Sources) != 1 {
		t.Fatalf("view = %+v %v", v, err)
	}
	if s := v.Sources[0]; s.ChatKey != dmKey || s.Person != "Dana" || s.Blocked != model.CrossBlockedPerson || s.Items != 1 {
		t.Errorf("source = %+v", s)
	}
	dv, _ := h.e.CrossView(context.Background(), dmKey)
	if dv.Mode != model.CrossOpen || len(dv.Sources) != 1 || dv.Sources[0].ChatKey != groupKey {
		t.Errorf("dm view = %+v", dv)
	}
}

func TestBriefJobAndLifecycle(t *testing.T) {
	h := newCrossHarness(t, nil)
	for i, text := range []string{"saturday still on?", "yes!", "can you bring wine", "sure I'll bring wine"} {
		speaker := "them"
		if i%2 == 1 {
			speaker = "me"
		}
		_ = h.st.AppendHistory(dmKey, model.Message{ID: "H" + string(rune('a'+i)), TS: h.clk.Now(), Speaker: speaker, Name: "Dana", Text: text}, 0)
	}
	c, _ := h.st.Chat(dmKey)
	for range briefEvery {
		h.e.briefTick(c)
	}
	h.advance(4 * time.Minute)
	var briefReqs int
	for _, r := range h.llm.Requests() {
		if r.JSON && strings.Contains(r.System, "private brief") {
			briefReqs++
		}
	}
	b, ok, _ := h.st.Brief(dmKey)
	if briefReqs != 1 || !ok || len(b.Commitments) != 1 || b.MessageCount != 4 {
		t.Fatalf("brief reqs=%d ok=%v %+v", briefReqs, ok, b)
	}
	if st := h.st.RunnerState(dmKey); st.BriefPending != 0 || st.BriefCursor.IsZero() {
		t.Errorf("state = %+v", st)
	}
	// On demand.
	if _, err := h.e.RefreshBrief(context.Background(), dmKey); err != nil {
		t.Fatal(err)
	}
	// Clearing the history forgets it.
	if err := h.e.ClearHistory(dmKey); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := h.st.Brief(dmKey); ok {
		t.Error("brief survived ClearHistory")
	}
	// A hand-off forgets it too.
	if _, err := h.e.RefreshBrief(context.Background(), dmKey); err != nil {
		t.Fatal(err)
	}
	h.msg(dmKey, "M1", "can you lend me $500 until friday")
	h.advance(5 * time.Second)
	if c, _ := h.st.Chat(dmKey); c.Handoff == nil {
		t.Fatalf("no hand-off: %v", h.actTypes())
	}
	if _, ok, _ := h.st.Brief(dmKey); ok {
		t.Error("brief survived the hand-off")
	}
}

func TestCrossOffForPlayground(t *testing.T) {
	h := newCrossHarness(t, nil)
	if u := h.e.crossContext(model.ChatAssignment{Key: "playground:x", Kind: "group", PersonaID: "leo"}, nil, nil, h.clk.Now()); u != nil {
		t.Error("playground drew on real chats")
	}
}

func TestCrossPlaygroundSource(t *testing.T) {
	h := newCrossHarness(t, nil)
	systems := captureReplies(h, fixed("haha sure"))
	pg := h.e.Playground()
	id, err := pg.Start("leo")
	if err != nil {
		t.Fatal(err)
	}
	if err := pg.SetCross(id, groupKey, ""); err == nil {
		t.Error("a group can't be a source")
	}
	if err := pg.SetCross(id, dmKey, "loud"); err == nil {
		t.Error("bad mode accepted")
	}
	if err := pg.SetCross(id, dmKey, ""); err != nil {
		t.Fatal(err)
	}
	out, err := pg.Send(context.Background(), id, "anyone up for drinks?", true) // the first cast member speaks: Dana
	if err != nil {
		t.Fatal(err)
	}
	if out.Speaker != "Dana" || out.Cross == nil || out.Cross.Items < 1 || out.Cross.Mode != model.CrossDiscreet {
		t.Fatalf("reply = %+v", out)
	}
	if sys := systems(); !strings.Contains(sys[len(sys)-1], nightFact) {
		t.Error("section missing from the playground prompt")
	}
	_ = pg.SetCross(id, "", "")
	out, _ = pg.Send(context.Background(), id, "hello?", true)
	if out.Cross != nil {
		t.Errorf("source cleared, yet %+v", out.Cross)
	}
}
