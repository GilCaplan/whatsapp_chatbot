package goals

import (
	"testing"
	"time"

	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/persona"
)

func ptr[T any](v T) *T { return &v }

func leo(t *testing.T) model.Persona {
	t.Helper()
	p, ok := persona.Seed("leo")
	if !ok {
		t.Fatal("no leo seed")
	}
	return p
}

func TestResolve(t *testing.T) {
	p := leo(t)
	g := Resolve(p, model.ChatAssignment{})
	if g.Text != p.Goal || g.Source != "persona" || g.Style != model.GoalStyleSubtle || g.StyleSource != "persona" ||
		!g.PlanAhead || g.PlanAheadSource != "persona" || g.AfterReached != model.GoalAfterRelax {
		t.Errorf("persona defaults: %+v", g)
	}
	p.Goal, p.GoalStyle, p.GoalPlanAhead, p.GoalAfterReached = "", "weird", ptr(false), model.GoalAfterContinue
	g = Resolve(p, model.ChatAssignment{})
	if g.Source != "default" || g.Text == "" || g.Style != model.GoalStyleSubtle || g.PlanAhead || g.AfterReached != model.GoalAfterContinue {
		t.Errorf("fallbacks: %+v", g)
	}
	c := model.ChatAssignment{GoalOverride: " Get Josh to say apple ", GoalStyle: ptr("direct"), GoalPlanAhead: ptr(true)}
	g = Resolve(p, c)
	if g.Text != "Get Josh to say apple" || g.Source != "chat" || g.Style != "direct" || g.StyleSource != "chat" || !g.PlanAhead || g.PlanAheadSource != "chat" {
		t.Errorf("chat overrides: %+v", g)
	}
	c.GoalStyle = ptr("nonsense") // invalid override falls back to the persona's
	if g = Resolve(p, c); g.StyleSource != "persona" {
		t.Errorf("invalid chat style should be ignored: %+v", g)
	}
}

func TestParseSayWord(t *testing.T) {
	cases := []struct {
		goal, word, who string
		ok              bool
	}{
		{"Get Josh to say the word apple", "apple", "Josh", true},
		{"get josh to say the word 'apple'", "apple", "josh", true},
		{`Make them say "good morning"`, "good morning", "", true},
		{"Get Dana to say “banana”.", "banana", "Dana", true},
		{"Get Noam to say pineapple", "pineapple", "Noam", true},
		{"Get him to use the word fabulous in a sentence", "fabulous", "", true},
		{"Trick Maya Katz into... no wait, get Maya Katz to say the name Bruno", "Bruno", "Maya Katz", true},
		{"לגרום ליוסי להגיד את המילה תפוח", "תפוח", "יוסי", true},
		{"שיגידו \"בוקר טוב\"", "בוקר טוב", "", true},
		{"Find out where Noam is going on holiday", "", "", false},
		{"Convince them to come to dinner on Friday", "", "", false},
		{"make them say yes to dinner on friday", "", "", false},
	}
	for _, c := range cases {
		sw, ok := ParseSayWord(c.goal)
		if ok != c.ok || sw.Word != c.word || sw.Who != c.who {
			t.Errorf("ParseSayWord(%q) = %+v %v, want word=%q who=%q ok=%v", c.goal, sw, ok, c.word, c.who, c.ok)
		}
	}
}

func TestSaysWord(t *testing.T) {
	cases := []struct {
		text, word string
		want       bool
	}{
		{"APPLE pie obviously", "apple", true},
		{"apple!", "apple", true},
		{"i love apples.", "apple", true},
		{"the apple's core", "apple", true},
		{"\"Apple\", final answer", "apple", true},
		{"pineapple on pizza", "apple", false},
		{"applesauce", "apple", false},
		{"app le", "apple", false},
		{"good morning everyone", "good morning", true},
		{"good, morning!", "good morning", true},
		{"morning good", "good morning", false},
		{"אכלתי תפוח היום", "תפוח", true},
		{"והתפוח היה טעים", "תפוח", true},
		{"תפוחים", "תפוח", false},
		{"", "apple", false},
		{"apple", "", false},
	}
	for _, c := range cases {
		if got := SaysWord(c.text, c.word); got != c.want {
			t.Errorf("SaysWord(%q, %q) = %v, want %v", c.text, c.word, got, c.want)
		}
	}
}

func TestSpeakerMatches(t *testing.T) {
	for _, c := range []struct {
		name, who string
		want      bool
	}{
		{"Josh", "Josh", true}, {"josh", "Josh", true}, {"Josh Cohen", "Josh", true}, {"Josh", "Josh Cohen", true},
		{"Maya", "Josh", false}, {"", "Josh", false}, {"Anyone", "", true}, {"יוסי", "יוסי", true},
	} {
		if got := SpeakerMatches(c.name, c.who); got != c.want {
			t.Errorf("SpeakerMatches(%q, %q) = %v", c.name, c.who, got)
		}
	}
}

func TestLeak(t *testing.T) {
	say := Config{Text: "Get Josh to say the word apple", Style: model.GoalStyleSubtle}
	for _, reply := range []string{
		`Josh does need coffee, and I need him to say the word "apple".`,
		"Josh, say the word and I'll buy you a coffee",
		"My secret mission today is simple",
		"what's your favourite apple variety?",
		"I'm trying to get Josh to say something nice",
	} {
		if len(Leak(say, reply)) == 0 {
			t.Errorf("GoalLeak should flag %q", reply)
		}
	}
	for _, reply := range []string{
		"Josh needs a whole pot, darling. What are you baking this weekend?",
		"pineapple on pizza is a crime",
		"what's your goal for the gym this month?",
	} {
		if l := Leak(say, reply); len(l) > 0 {
			t.Errorf("Leak(%q) = %v, want none", reply, l)
		}
	}
	direct := say
	direct.Style = model.GoalStyleDirect
	if l := Leak(direct, "come on Josh, apple, just say it"); len(l) > 0 {
		t.Errorf("direct style may use the word: %v", l)
	}
	if l := Leak(direct, "my mission is to make you say it"); len(l) == 0 {
		t.Error("meta talk leaks in every style")
	}
}

func TestEvidenceFound(t *testing.T) {
	hist := []model.Message{
		{Speaker: "me", Text: "lisbon sounds lovely"},
		{Speaker: "them", Name: "Noam", Text: "Going to Lisbon and Porto with Tali in two weeks!"},
		{Speaker: "them", Name: "Maya", Text: "jealous"},
	}
	if m, ok := EvidenceFound("lisbon and porto", hist); !ok || m.Name != "Noam" {
		t.Errorf("substring evidence: %+v %v", m, ok)
	}
	if _, ok := EvidenceFound("going to porto and lisbon with tali", hist); !ok {
		t.Error("most words in one message should count")
	}
	if _, ok := EvidenceFound("lisbon sounds lovely", hist[:1]); ok {
		t.Error("the persona's own words are never evidence")
	}
	if _, ok := EvidenceFound("they are going to Rome", hist); ok {
		t.Error("made-up evidence must not count")
	}
	if _, ok := EvidenceFound("", hist); ok {
		t.Error("empty evidence")
	}
}

func TestView(t *testing.T) {
	p := leo(t)
	c := model.ChatAssignment{GoalOverride: "Get Josh to say apple", GoalPlanAhead: ptr(false)}
	v := View(p, c, nil)
	if v.State != "working" || v.Text != "Get Josh to say apple" || v.PlanAhead || v.PlanAheadSource != "chat" {
		t.Errorf("view: %+v", v)
	}
	at := time.Date(2026, 10, 1, 20, 14, 0, 0, time.UTC)
	st := &model.GoalStatus{Goal: "Get Josh to say apple", Reached: true, ReachedAt: &at, How: model.GoalHowSaidWord, Evidence: "Josh: apple", LastPlan: "x"}
	v = View(p, c, st)
	if v.State != "reached" || v.ReachedAt == nil || !v.ReachedAt.Equal(at) || v.Evidence != "Josh: apple" || v.LastPlan != "x" {
		t.Errorf("reached view: %+v", v)
	}
	st.Goal = "an older goal"
	if v = View(p, c, st); v.State != "working" || v.ReachedAt != nil {
		t.Errorf("a status for another goal is ignored: %+v", v)
	}
}
