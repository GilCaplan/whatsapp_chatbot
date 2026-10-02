package crossctx

import (
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

func ptr[T any](v T) *T { return &v }

func TestModeAndShare(t *testing.T) {
	r := Defaults()
	group := model.ChatAssignment{Kind: "group"}
	dm := model.ChatAssignment{Kind: "dm"}
	if m, src := ModeFor(r, group); m != model.CrossDiscreet || src != SourceDefault {
		t.Errorf("group default = %s %s", m, src)
	}
	if m, _ := ModeFor(r, dm); m != model.CrossOpen {
		t.Errorf("dm default = %s", m)
	}
	group.Cross.Mode = model.CrossOpen
	if m, src := ModeFor(r, group); m != model.CrossOpen || src != SourceChat {
		t.Errorf("chat override = %s %s", m, src)
	}
	group.Cross.Mode = "loud"
	if m, _ := ModeFor(r, group); m != model.CrossDiscreet {
		t.Errorf("invalid mode falls back: %s", m)
	}
	r.Enabled = false
	if m, _ := ModeFor(r, group); m != model.CrossOff {
		t.Errorf("disabled = %s", m)
	}
	if ok, _ := ShareFor(r, dm); ok {
		t.Error("disabled shares")
	}
	r.Enabled = true
	if ok, src := ShareFor(r, dm); !ok || src != SourceDefault {
		t.Errorf("share default = %v %s", ok, src)
	}
	dm.Cross.Share = ptr(false)
	if ok, src := ShareFor(r, dm); ok || src != SourceChat {
		t.Errorf("share off = %v %s", ok, src)
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]string{
		"is behind on rent and owes the landlord money": model.HandoffMoney,
		"started chemo last month":                      model.HandoffHealth,
		"starting therapy for anxiety":                  model.HandoffDistress,
		"see you at the gym later lol":                  model.HandoffMeeting, // weak hits count: strict on purpose
		"has a lawyer for the divorce":                  model.HandoffLegal,
		"asked if I'm a bot":                            model.HandoffBot,
		"broke up with her boyfriend":                   model.SensitiveRomance,
		"has a crush on someone at work":                model.SensitiveRomance,
		"told me a secret about Josh":                   model.SensitiveSecret,
		"said keep it quiet until Friday":               model.SensitiveSecret,
		"חולה מאוד השבוע":                               model.HandoffHealth,
		"נפרדנו אתמול":                                  model.SensitiveRomance,
		"זה סוד":                                        model.SensitiveSecret,
		"אל תספר לאף אחד":                               model.SensitiveSecret,
		"is behind on rent this month":                  model.HandoffMoney,
		"got laid off last week":                        model.HandoffMoney,
		"starting therapy next week":                    model.HandoffHealth,
		"has been struggling since the move":            model.HandoffDistress,
		"מחפש עבודה כי פוטר":                            model.HandoffMoney,
		"works night shifts as a nurse":                 "",
		"training for the Tel Aviv marathon":            "",
		"hates cilantro":                                "",
		"new job at a startup":                          "",
		"":                                              "",
	}
	for text, want := range cases {
		if got := Classify(text); got != want {
			t.Errorf("Classify(%q) = %q, want %q", text, got, want)
		}
	}
	if got := EffectiveSensitive(model.Memory{Text: "hates cilantro", Sensitive: model.HandoffHealth}); got != model.HandoffHealth {
		t.Errorf("stored category wins: %q", got)
	}
	if got := EffectiveSensitive(model.Memory{Text: "is fine", Evidence: "my therapist says I'm doing better"}); got != "" {
		// "therapist" alone is not in the lexicon; evidence is scanned though.
		t.Logf("evidence classified as %q", got)
	}
	if got := EffectiveSensitive(model.Memory{Text: "is doing better", Evidence: "out of the hospital finally"}); got != model.HandoffHealth {
		t.Errorf("evidence scanned: %q", got)
	}
}

func TestCarries(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	r := Defaults()
	fresh := model.Memory{Text: "works night shifts as a nurse", UpdatedAt: now.Add(-24 * time.Hour)}
	old := fresh
	old.UpdatedAt = now.Add(-30 * 24 * time.Hour)
	pinnedOld := old
	pinnedOld.Pinned = true
	sens := fresh
	sens.Text = "started chemo"
	locked := fresh
	locked.Scope = model.MemoryScopeLocal
	unlocked := sens
	unlocked.Scope = model.MemoryScopeShared
	past := fresh
	past.ExpiresAt = ptr(now.Add(-72 * time.Hour))
	cases := []struct {
		name string
		m    model.Memory
		ok   bool
		why  string
	}{
		{"fresh", fresh, true, ""},
		{"stale", old, false, WhyStale},
		{"pinned stale", pinnedOld, true, ""},
		{"sensitive", sens, false, WhySensitive},
		{"locked", locked, false, WhyLocked},
		{"unlocked sensitive", unlocked, true, ""},
		{"past event", past, false, WhyExpired},
	}
	for _, c := range cases {
		ok, why := Carries(c.m, r, now)
		if ok != c.ok || why != c.why {
			t.Errorf("%s: got %v %q, want %v %q", c.name, ok, why, c.ok, c.why)
		}
	}
	// A category switched off in the settings may cross.
	r.Sensitive[model.HandoffHealth] = false
	if ok, _ := Carries(sens, r, now); !ok {
		t.Error("health off should carry")
	}
	// Zero rules keep everything sensitive home.
	if ok, _ := Carries(sens, Rules{}, now); ok {
		t.Error("zero rules carried a sensitive memory")
	}
}

func TestSelect(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	r := Defaults()
	r.MaxPeople, r.MaxItems = 2, 5
	dana := Person{Name: "Dana", JID: "972500000001@s.whatsapp.net", IDs: []string{"111@lid"}, Rank: 0}
	josh := Person{Name: "Josh Levi", JID: "972500000002@s.whatsapp.net", Rank: 1}
	maya := Person{Name: "Maya", Rank: 2}
	items := []Item{
		{Kind: KindMemory, Person: "Dana", PersonJID: "972500000001@s.whatsapp.net", Text: "works night shifts", At: now.Add(-time.Hour)},
		{Kind: KindMemory, Person: "Dana", PersonJID: "111@lid", Text: "training for a marathon", At: now.Add(-48 * time.Hour)},
		{Kind: KindMemory, Person: "Dana", Text: "hates cilantro", At: now.Add(-24 * time.Hour)},
		{Kind: KindTopic, Person: "Dana", Text: "lately talking about: flat hunting", At: now.Add(-2 * time.Hour)},
		{Kind: KindCommitment, Person: "Dana", Text: "bring wine on Sat 4 Oct", At: now.Add(-100 * time.Hour)},
		{Kind: KindMemory, Person: "Josh", Text: "supports Maccabi", At: now},
		{Kind: KindMemory, Person: "Maya", Text: "has a cat", At: now},
		{Kind: KindMemory, Person: "Dana", PersonJID: "972500000099@s.whatsapp.net", Text: "another Dana", At: now},
	}
	got := Select(items, []Person{maya, josh, dana}, []string{"anyone up for a marathon?"}, now, r)
	if len(got) != 4 {
		t.Fatalf("got %d items: %+v", len(got), got)
	}
	// Dana first (rank 0), three items, the commitment kept, the marathon (on topic) in.
	if got[0].Person != "Dana" || got[3].Person != "Josh" {
		t.Errorf("order: %+v", got)
	}
	var hasCommit, hasMarathon bool
	for _, it := range got[:3] {
		hasCommit = hasCommit || it.Kind == KindCommitment
		hasMarathon = hasMarathon || it.Text == "training for a marathon"
		if it.Text == "another Dana" {
			t.Error("matched a different Dana by name despite ids")
		}
	}
	if !hasCommit || !hasMarathon {
		t.Errorf("commitment %v marathon %v: %+v", hasCommit, hasMarathon, got)
	}
	for _, it := range got {
		if it.Person == "Maya" {
			t.Error("Maya is beyond MaxPeople")
		}
	}
	r.MaxItems = 2
	if got := Select(items, []Person{dana, josh}, nil, now, r); len(got) != 2 {
		t.Errorf("MaxItems: %d", len(got))
	}
	if Select(nil, []Person{dana}, nil, now, r) != nil || Select(items, nil, nil, now, r) != nil {
		t.Error("empty input")
	}
}

func TestLeak(t *testing.T) {
	night := Item{Kind: KindMemory, Person: "Dana", PersonJID: "972500000001@s.whatsapp.net", Text: "works night shifts at Ichilov"}
	exam := Item{Kind: KindMemory, Person: "Dana", Text: "has an exam on Fri 9 Oct"}
	marathon := Item{Kind: KindMemory, Person: "Dana", Text: "training for the Tel Aviv marathon"}
	group := func(mode string, items []Item, reply string, here []string, present map[string]bool) []string {
		return Leak(LeakInput{Items: items, Reply: reply, Here: here, Mode: mode, IsGroup: true, Present: present})
	}
	cases := []struct {
		name    string
		mode    string
		items   []Item
		reply   string
		here    []string
		present map[string]bool
		leak    bool
	}{
		{"planted facts", model.CrossDiscreet, []Item{night}, "how are the night shifts at Ichilov treating you", nil, nil, true},
		{"stemmed", model.CrossDiscreet, []Item{night}, "still doing those night shift things at ichilov?", nil, nil, true},
		{"already said here", model.CrossDiscreet, []Item{exam}, "good luck on the exam friday!", []string{"Dana: ugh exam on friday"}, nil, false},
		{"single rare token", model.CrossDiscreet, []Item{exam}, "hope the exam went well", nil, nil, true},
		{"unrelated", model.CrossDiscreet, []Item{night, marathon}, "haha same, pizza tonight?", nil, nil, false},
		{"she brought it up", model.CrossDiscreet, []Item{marathon}, "the marathon? you've got this", []string{"Dana: signed up for the marathon!"}, nil, false},
		{"told phrase", model.CrossDiscreet, []Item{marathon}, "like you told me, rest days matter", nil, nil, true},
		{"private chat phrase", model.CrossDiscreet, []Item{marathon}, "we talked about it in the DMs", nil, nil, true},
		{"open present", model.CrossOpen, []Item{marathon}, "like you said the other day, the marathon is close!", nil, map[string]bool{"972500000001": true, "dana": true}, false},
		{"open present but private chat named in group", model.CrossOpen, []Item{marathon}, "as we said in our private chat", nil, map[string]bool{"dana": true}, true},
		{"open absent", model.CrossOpen, []Item{marathon}, "Dana's training for the Tel Aviv marathon btw", nil, map[string]bool{"josh": true}, true},
		{"short proper noun", model.CrossDiscreet, []Item{{Kind: KindMemory, Person: "Dana", Text: "started a new job at Wix"}}, "how's that new job at Wix treating you?", nil, nil, true},
		{"three-letter place", model.CrossDiscreet, []Item{marathon}, "can't believe you're running in Tel Aviv!", []string{"Dana: signed up for my first marathon"}, nil, true},
		{"common short words", model.CrossDiscreet, []Item{{Kind: KindMemory, Person: "Dana", Text: "started a new job at Wix"}}, "how was your day? anything new?", nil, nil, false},
		{"detail of a live topic", model.CrossDiscreet, []Item{{Kind: KindMemory, Person: "Dana", Text: "training for the Tel Aviv marathon in February"}}, "legs will be begging for mercy by February", []string{"Dana: signed up for my first marathon", "few months of training lol"}, nil, true},
		{"lone name", model.CrossDiscreet, []Item{{Kind: KindMemory, Person: "Dana", Text: "started a new job at Wix"}}, "Wix should fix their coffee machine", nil, nil, true},
		{"off", model.CrossOff, []Item{night}, "night shifts at Ichilov", nil, nil, false},
	}
	for _, c := range cases {
		got := group(c.mode, c.items, c.reply, c.here, c.present)
		if (len(got) > 0) != c.leak {
			t.Errorf("%s: leak=%v (%v), want %v", c.name, len(got) > 0, got, c.leak)
		}
	}
	// Keeping a private promise is fine; tying it to the person is not.
	wine := Item{Kind: KindCommitment, Person: "Dana", Text: "bring wine to Josh's dinner on Saturday"}
	if got := group(model.CrossDiscreet, []Item{wine}, "I'll bring the wine!", []string{"who's bringing wine?"}, nil); len(got) > 0 {
		t.Errorf("own promise flagged: %v", got)
	}
	if got := group(model.CrossDiscreet, []Item{wine}, "I'll bring wine, Josh", nil, nil); len(got) > 0 {
		t.Errorf("own promise (not asked) flagged: %v", got)
	}
	if got := group(model.CrossDiscreet, []Item{wine}, "Dana and I already sorted the wine", nil, nil); len(got) == 0 {
		t.Error("promise tied to Dana not flagged")
	}
	// DM, Open: the contact's own group lines may come up; "like you said" is fine.
	dm := Leak(LeakInput{Items: []Item{{Kind: KindGroupNote, Person: "Dana", Text: "pushed for Saturday brunch"}}, Reply: "you really pushed for brunch lol, like you said", Mode: model.CrossOpen, Present: map[string]bool{"dana": true}})
	if len(dm) > 0 {
		t.Errorf("dm open: %v", dm)
	}
}

func TestMatches(t *testing.T) {
	it := Item{Person: "Dana Cohen", PersonJID: "111@lid"}
	if !Matches(it, Person{Name: "D", IDs: []string{"972500000001@s.whatsapp.net", "111@lid"}}) {
		t.Error("lid id")
	}
	if Matches(it, Person{Name: "Dana", JID: "222@lid"}) {
		t.Error("different id, same name")
	}
	if !Matches(Item{Person: "Dana Cohen"}, Person{Name: "dana"}) {
		t.Error("first name")
	}
}
