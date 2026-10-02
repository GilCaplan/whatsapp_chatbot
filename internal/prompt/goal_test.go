package prompt

import (
	"encoding/json"
	"strings"
	"testing"

	"whatsappdoppel/internal/goals"
	"whatsappdoppel/internal/model"
)

func TestGoalSectionStyles(t *testing.T) {
	cfg := goals.Config{Text: "Get Josh to say the word apple", Style: model.GoalStyleSubtle, AfterReached: model.GoalAfterRelax}
	sub := GoalSection(cfg, GoalTurn{})
	for _, want := range []string{
		"YOUR PRIVATE AGENDA: Get Josh to say the word apple",
		"nobody in the chat knows about it",
		"Never say it, say that you want something, or explain why you're asking.",
		`Never write the word "apple" yourself and never ask them to say it`,
		"Ask a normal question whose most natural answer is that word",
		"No riddles or cryptic hints",
		"take one small step",
	} {
		if !strings.Contains(sub, want) {
			t.Errorf("subtle section missing %q:\n%s", want, sub)
		}
	}
	if strings.Contains(sub, "NEXT MOVE") || strings.Contains(sub, "IMPORTANT") {
		t.Error("no plan/retry note without a plan/retry")
	}

	cfg.Style = model.GoalStyleBalanced
	bal := GoalSection(cfg, GoalTurn{})
	if !strings.Contains(bal, "may bring the topic up directly") || !strings.Contains(bal, `Never write the word "apple"`) || !strings.Contains(bal, "never say you have one") {
		t.Errorf("balanced section:\n%s", bal)
	}
	cfg.Style = model.GoalStyleDirect
	dir := GoalSection(cfg, GoalTurn{})
	if !strings.Contains(dir, "YOUR AGENDA: ") || !strings.Contains(dir, "openly") || strings.Contains(dir, "Never write the word") {
		t.Errorf("direct section:\n%s", dir)
	}

	// A goal that is not "say the word" gets the generic wording.
	other := GoalSection(goals.Config{Text: "Find out where Noam is going on holiday", Style: model.GoalStyleSubtle}, GoalTurn{})
	if !strings.Contains(other, "suggesting/inviting something as your own idea is fine") || strings.Contains(other, "Paris") {
		t.Errorf("generic subtle section:\n%s", other)
	}
}

func TestGoalSectionTurn(t *testing.T) {
	cfg := goals.Config{Text: "Get Josh to say apple", Style: model.GoalStyleSubtle, AfterReached: model.GoalAfterRelax}
	s := GoalSection(cfg, GoalTurn{Plan: "Ask Josh what pie he'd bake for the party"})
	if !strings.HasSuffix(s, "YOUR NEXT MOVE (private note to yourself — do this in this reply, in your own words and style, never quote it): Ask Josh what pie he'd bake for the party") {
		t.Errorf("plan note must close the section:\n%s", s)
	}
	s = GoalSection(cfg, GoalTurn{Retry: true})
	if !strings.Contains(s, "your last draft gave your agenda away") || !strings.Contains(s, `do not use the word "apple"`) {
		t.Errorf("retry note:\n%s", s)
	}
	relaxed := GoalSection(cfg, GoalTurn{Reached: true, Plan: "ignored"})
	if strings.Contains(relaxed, "apple") || strings.Contains(relaxed, "ignored") || !strings.Contains(relaxed, "Just chat naturally") {
		t.Errorf("relaxed section must drop the goal entirely:\n%s", relaxed)
	}
	if off := GoalSection(cfg, GoalTurn{Off: true}); off != relaxed {
		t.Errorf("Off should equal the relaxed section: %q", off)
	}
	cfg.AfterReached = model.GoalAfterContinue
	cont := GoalSection(cfg, GoalTurn{Reached: true})
	if !strings.Contains(cont, "YOUR PRIVATE AGENDA") || !strings.Contains(cont, "It already happened once") {
		t.Errorf("continue keeps pursuing:\n%s", cont)
	}
}

func TestSystemPromptUsesGoalSection(t *testing.T) {
	p := leo(t)
	chat := model.ChatAssignment{GoalOverride: "Get Josh to say the word apple"}
	sys := SystemPrompt(p, chat, true, "i think Josh needs some coffee", Options{Goal: GoalTurn{Plan: "Ask about dessert"}})
	if strings.Contains(sys, "\nGOAL: ") {
		t.Error("the bare GOAL line is gone")
	}
	agenda := strings.Index(sys, "YOUR PRIVATE AGENDA: Get Josh to say the word apple")
	move := strings.Index(sys, "YOUR NEXT MOVE")
	guidance := strings.Index(sys, "GUIDANCE: ")
	if agenda < 0 || move < agenda || guidance < move || strings.Index(sys, GroupNote) > agenda {
		t.Errorf("order: group note, agenda, next move, guidance:\n%s", sys)
	}
	for _, s := range []string{"Prioritize the GOAL", "through the lens"} {
		if strings.Contains(sys, s) {
			t.Errorf("seed text still encourages bluntness: %q", s)
		}
	}
}

func TestPlanRequest(t *testing.T) {
	p := leo(t)
	cfg := goals.Resolve(p, model.ChatAssignment{GoalOverride: "Get Josh to say the word apple"})
	hist := []model.Message{
		{Speaker: "them", Name: "Maya", Text: "i think Josh needs some coffee"},
		{Speaker: "me", Text: "a whole pot, darling"},
		{Speaker: "them", Name: "Josh", Text: "ha.\nvery funny"},
	}
	req := Plan(p, cfg, hist, true, []string{"Tease Josh about his kitchen"})
	if !req.JSON || len(req.Schema) == 0 || !json.Valid(req.Schema) || req.MaxTokens <= 0 || req.Temperature < 0 {
		t.Fatalf("plan request: %+v", req)
	}
	if !strings.Contains(string(req.Schema), `"ideas"`) || strings.Contains(string(Plan(p, ResolveForTest(p, "Find out where Noam is going"), hist, true, nil).Schema), `"ideas"`) {
		t.Error("only say-the-word goals brainstorm ideas")
	}
	for _, want := range []string{"private coach of Leo", "a WhatsApp group chat", "GOAL: Get Josh to say the word apple", `must NOT contain the word "apple"`, `"next"`, `"achieved"`, `"ideas": "first say what kind of thing the target word is`} {
		if !strings.Contains(req.System, want) {
			t.Errorf("plan system missing %q", want)
		}
	}
	u := req.Messages[0].Content
	for _, want := range []string{"Maya: i think Josh needs some coffee", "Leo (you're coaching them): a whole pot, darling", "Josh: ha. very funny", "- Tease Josh about his kitchen"} {
		if !strings.Contains(u, want) {
			t.Errorf("plan user turn missing %q:\n%s", want, u)
		}
	}
	long := make([]model.Message, 40)
	for i := range long {
		long[i] = model.Message{Speaker: "them", Name: "A", Text: "msg" + string(rune('a'+i%26))}
	}
	if n := strings.Count(Plan(p, cfg, long, false, nil).Messages[0].Content, "A: msg"); n != MaxPlanHistory {
		t.Errorf("plan reads %d messages, want %d", n, MaxPlanHistory)
	}
}

func TestParsePlan(t *testing.T) {
	cases := []struct {
		raw      string
		next     string
		achieved bool
		evidence string
		err      bool
	}{
		{`{"situation":"chatting about coffee","achieved":false,"evidence":"","next":"Ask Josh what dessert he is making"}`, "Ask Josh what dessert he is making", false, "", false},
		{"Sure! Here's the plan:\n```json\n{\"next_move\": \"Ask about pie\", \"achieved\": \"false\"}\n```", "Ask about pie", false, "", false},
		{`{"achieved": "yes", "evidence": "hiking with my sister", "next": ""}`, "", true, "hiking with my sister", false},
		{`{"Next": ["Ask about", "the weekend"], "achieved": true, "quote": "lisbon!"}`, "Ask about the weekend", true, "lisbon!", false},
		{`{"move": "  \"Compliment the dog\"  ", "done": 1}`, "Compliment the dog", false, "", false},
		{"Next move: ask what fruit is in the bowl\nmore text", "ask what fruit is in the bowl", false, "", false},
		{`{"situation": "x", "achieved": false, "next": ""}`, "", false, "", true},
		{"", "", false, "", true},
		{"{{{", "", false, "", true},
	}
	for _, c := range cases {
		got, err := ParsePlan(c.raw)
		if (err != nil) != c.err || got.Next != c.next || got.Achieved != c.achieved || got.Evidence != c.evidence {
			t.Errorf("ParsePlan(%q) = %+v, %v; want next=%q achieved=%v evidence=%q err=%v", c.raw, got, err, c.next, c.achieved, c.evidence, c.err)
		}
	}
	long, _ := ParsePlan(`{"next":"` + strings.Repeat("word ", 100) + `"}`)
	if r := []rune(long.Next); len(r) > MaxPlanRunes+1 || strings.Contains(long.Next, "\n") {
		t.Errorf("move not capped: %d runes", len(r))
	}
}

// ResolveForTest resolves p's goal settings with a chat goal override.
func ResolveForTest(p model.Persona, goal string) goals.Config {
	return goals.Resolve(p, model.ChatAssignment{GoalOverride: goal})
}

func TestGoalCheck(t *testing.T) {
	p := leo(t)
	cfg := ResolveForTest(p, "Find out where Noam is going on holiday")
	var hist []model.Message
	for i := 0; i < 15; i++ {
		hist = append(hist, model.Message{Speaker: "them", Name: "Maya", Text: "filler " + string(rune('a'+i))})
		hist = append(hist, model.Message{Speaker: "me", Text: "my own words"})
	}
	hist = append(hist, model.Message{Speaker: "them", Name: "Noam", Text: "porto\nand lisbon!"})
	req := GoalCheck(p, cfg, hist)
	c := req.Messages[0].Content
	if !req.JSON || req.Temperature != 0 || !json.Valid(req.Schema) {
		t.Fatalf("request: %+v", req)
	}
	if strings.Contains(c, "my own words") || !strings.Contains(c, "[12] Noam: porto and lisbon!") || strings.Count(c, "Maya: filler") != MaxCheckMessages-1 {
		t.Errorf("check reads only their last %d messages:\n%s", MaxCheckMessages, c)
	}
	if !strings.Contains(c, `Leo wanted this: "Find out where Noam is going on holiday"`) {
		t.Errorf("goal missing:\n%s", c)
	}
	for raw, want := range map[string]struct {
		yes   bool
		quote string
		err   bool
	}{
		`{"answer": true, "quote": "porto and lisbon"}`:    {true, "porto and lisbon", false},
		"sure: {\"Answer\": \"yes\", \"Quote\": \"x\"} ok": {true, "x", false},
		`{"answer": false, "quote": ""}`:                   {false, "", false},
		"no idea":                                          {false, "", true},
	} {
		yes, q, err := ParseGoalCheck(raw)
		if yes != want.yes || q != want.quote || (err != nil) != want.err {
			t.Errorf("ParseGoalCheck(%q) = %v %q %v", raw, yes, q, err)
		}
	}
}

func TestSpeaksAs(t *testing.T) {
	hist := []model.Message{
		{Speaker: "them", Name: "Josh Cohen", Text: "morning"},
		{Speaker: "them", Name: "Maya", Text: "hi"},
		{Speaker: "me", Text: "i think Josh needs some coffee"},
	}
	for reply, want := range map[string]string{
		"Josh: haha, I'm running on fumes":      "Josh",
		"josh cohen: no":                        "Josh Cohen",
		`"Maya: lol"`:                           "Maya",
		"Josh, darling, you look dreadful":      "",
		"Leo: darling, coffee":                  "",
		"Maya's right, Josh needs an espresso.": "",
	} {
		if got := SpeaksAs(reply, "Leo", hist); !strings.EqualFold(got, want) {
			t.Errorf("SpeaksAs(%q) = %q, want %q", reply, got, want)
		}
	}
	if !strings.Contains(OwnVoiceReminder("Leo", "Josh"), "you wrote Josh's line") {
		t.Error("reminder text")
	}
}
