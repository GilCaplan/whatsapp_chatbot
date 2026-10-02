package prompt

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/persona"
)

var update = flag.Bool("update", false, "rewrite golden files")

func leo(t *testing.T) model.Persona {
	t.Helper()
	p, ok := persona.Seed("leo")
	if !ok {
		t.Fatal("no leo seed")
	}
	return p
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run go test -update): %v", path, err)
	}
	if string(want) != got {
		t.Errorf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestSystemPromptLeoGolden(t *testing.T) {
	sys := SystemPrompt(leo(t), model.ChatAssignment{}, false, "hey")
	golden(t, "leo_dm.golden", sys)

	for _, want := range []string{
		"# IDENTITY & BIO\n- Name: Leo",
		"# COMMUNICATION STYLE",
		"# GUIDELINES",
		"be Leo, own it!",
		"- Stay in character at all times. You are Leo.",
		"- Formatting: plain text only, no markdown, no bold.",
		AntiJailbreakRules,
		"\n\nYOUR PRIVATE AGENDA: Catch up and see how their week is going, and show them who you are, darling.",
		"\n\nGUIDANCE: Keep it brief: one sentence, two short ones at most — at most 12 words.",
		"- Emoji: Rarely: about one message in five gets a single emoji, the rest have none.",
		"Favourites: 💅 ✨ 🍸 🛋️.",
		"- Length: Short: a few words to one sentence",
	} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
	if strings.Contains(sys, "CONTEXT: You are chatting in a group") {
		t.Error("DM prompt must not contain the group note")
	}
	if !strings.HasSuffix(sys, "This length rule wins over any length mentioned above.") {
		t.Error("guidance must be last (legacy order)")
	}
	// Serious news: the turn hint holds emoji and jokes back; "none" repeats the rule.
	if sad := SystemPrompt(leo(t), model.ChatAssignment{}, false, "my grandmother passed away this morning"); !strings.HasSuffix(sad, "be kind and genuine, no emoji and no jokes.") {
		t.Errorf("serious turn hint missing: %q", sad[len(sad)-120:])
	}
	p := leo(t)
	p.Emoji.Usage = "none"
	if sys := SystemPrompt(p, model.ChatAssignment{}, false, "lol"); !strings.HasSuffix(sys, " No emoji.") || !strings.Contains(sys, "- Emoji: Never use emoji.") {
		t.Error("emoji none must be stated in the style and every turn")
	}
}

func TestSystemPromptGroupAndOverrides(t *testing.T) {
	p := leo(t)
	chat := model.ChatAssignment{GoalOverride: "Convince them to come to the party"}
	long := "so I was thinking about going to that new place downtown on friday night, are you in?"
	sys := SystemPrompt(p, chat, true, long)
	golden(t, "leo_group.golden", sys)
	if !strings.Contains(sys, GroupNote) {
		t.Error("group note missing")
	}
	if !strings.Contains(sys, "YOUR PRIVATE AGENDA: Convince them to come to the party") {
		t.Error("goal override ignored")
	}
	if !strings.Contains(sys, "GUIDANCE: Keep it brief: one sentence, two short ones at most — at most 12 words.") {
		t.Error("a 17-word message keeps the short budget (only > 25 words earn more)")
	}
	// Anti-jailbreak rules always come right after the identity block.
	if strings.Index(sys, AntiJailbreakRules) > strings.Index(sys, GroupNote) {
		t.Error("order: identity, rules, group note")
	}
}

func TestAdvancedPromptReplacesIdentity(t *testing.T) {
	p := leo(t)
	p.AdvancedPrompt = "You are Leo. Custom everything."
	sys := SystemPrompt(p, model.ChatAssignment{}, false, "")
	if strings.Contains(sys, "# IDENTITY & BIO") {
		t.Error("advanced prompt should replace rendered sections")
	}
	if !strings.HasPrefix(sys, "\nYou are Leo. Custom everything.") || !strings.Contains(sys, AntiJailbreakRules) || !strings.Contains(sys, "YOUR PRIVATE AGENDA: ") {
		t.Error("advanced prompt should keep rules/goal/guidance")
	}
}

func TestGuidanceClamp(t *testing.T) {
	long := strings.Repeat("word ", 30)
	cases := map[[2]string]string{
		{"short", "hi"}:   "Keep it brief: one sentence, two short ones at most — at most 12 words.",
		{"short", long}:   "Short: one or two sentences — at most 15 words.",
		{"medium", "hi"}:  "Two or three sentences — at most 30 words.",
		{"long", long}:    "A full paragraph is fine when there's something to say — at most 75 words.",
		{"", "hi there"}:  "Keep it brief: one sentence, two short ones at most — at most 12 words.",
		{"medium", long}:  "Two or three sentences — at most 38 words.",
		{"long", "hello"}: "Up to a short paragraph (three to five sentences) — at most 60 words.",
	}
	for in, want := range cases {
		if got := Guidance(in[0], in[1], "normal"); !strings.HasPrefix(got, want) {
			t.Errorf("Guidance(%q, …) = %q, want %q", in[0], got, want)
		}
	}
}

func TestMessagesRoleMapping(t *testing.T) {
	hist := []model.Message{
		{Speaker: "them", Name: "Dana", Text: "hey"},
		{Speaker: "me", Text: "darling!"},
		{Speaker: "them", Name: "Avi", Text: "what's up"},
		{Speaker: "me", Text: "tell me about your week"},
	}
	got := Messages(hist, true)
	want := []llm.Message{
		{Role: "user", Content: "Dana: hey"},
		{Role: "assistant", Content: "darling!"},
		{Role: "user", Content: "Avi: what's up"},
		// trailing "me" = trigger → user, attributed, framed as the turn to answer
		{Role: "user", Content: "Someone: tell me about your week" + TurnCue("")},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d messages", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("msg %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	dm := Messages(hist[:2], false)
	if dm[0].Content != "hey" {
		t.Error("DMs must not get name prefixes")
	}
	if dm[1].Role != "user" {
		t.Error("last me message must map to user")
	}

	req := Compose(leo(t), model.ChatAssignment{}, hist, true)
	if req.System == "" || len(req.Messages) != 4 || req.Temperature >= 0 {
		t.Errorf("Compose: %+v", req)
	}
	if got := req.Messages[3].Content; !strings.HasSuffix(got, TurnCue("Leo")) || !strings.Contains(got, "Leo:") {
		t.Errorf("group turn cue = %q", got)
	}
	// A member's last turn is framed too; DMs never are.
	if got := Messages(hist[:3], true)[2].Content; got != "Avi: what's up"+TurnCue("") {
		t.Errorf("member last turn = %q", got)
	}
	if got := Messages(hist[:3], false)[2].Content; got != "what's up" {
		t.Errorf("DM last turn = %q", got)
	}
}

// Typo bubbles: the model sees what the persona meant, never the slip or
// the "*word" correction.
func TestMessagesSkipFixAndUseCorrected(t *testing.T) {
	hist := []model.Message{
		{Speaker: "them", Name: "Dana", Text: "when?"},
		{Speaker: "me", Text: "see you tmorrow", Corrected: "see you tomorrow", FromBot: true},
		{Speaker: "me", Text: "*tomorrow", Kind: model.MsgKindFix, FromBot: true},
		{Speaker: "them", Name: "Dana", Text: "cool"},
	}
	got := Messages(hist, false)
	if len(got) != 3 || got[1].Content != "see you tomorrow" || got[1].Role != "assistant" || got[2].Content != "cool" {
		t.Fatalf("got %+v", got)
	}
	req := Initiate(leo(t), model.ChatAssignment{}, hist, false)
	for _, m := range req.Messages {
		if strings.Contains(m.Content, "*tomorrow") || strings.Contains(m.Content, "tmorrow") {
			t.Errorf("initiate leaked a typo: %+v", req.Messages)
		}
	}
}

func TestDecision(t *testing.T) {
	req := Decision(leo(t), "anyone up for drinks?")
	if req.MaxTokens != 5 || req.Temperature != 0 || len(req.Messages) != 1 {
		t.Fatalf("decision request: %+v", req)
	}
	c := req.Messages[0].Content
	for _, want := range []string{"whether Leo would naturally jump", "Leo's persona: witty, social", `Message: "anyone up for drinks?"`, "Reply with only YES or NO."} {
		if !strings.Contains(c, want) {
			t.Errorf("decision prompt missing %q", want)
		}
	}
	for in, want := range map[string]bool{"YES": true, "yes.": true, " \"Yes\"": true, "NO": false, "Nope": false, "": false, "**YES**": true} {
		if ParseDecision(in) != want {
			t.Errorf("ParseDecision(%q) != %v", in, want)
		}
	}
}

func TestBuilderAndParseDraft(t *testing.T) {
	req := Builder("a grumpy chef from Naples", "ugh. pasta again.\nNO pineapple")
	if !req.JSON || req.MaxTokens < 1000 || !strings.Contains(req.Messages[0].Content, "grumpy chef") ||
		!strings.Contains(req.Messages[0].Content, "NO pineapple") || !strings.Contains(req.System, `"fallbackReply"`) {
		t.Errorf("builder request: %+v", req)
	}

	raw := "Sure! Here you go:\n```json\n" + `{
  "name": "Gino",
  "tagline": "Chef who hates {shortcuts}",
  "avatar": {"glyph": "Coffee"},
  "bio": "Runs a trattoria in Naples.",
  "personality": ["grumpy", "proud", "secretly soft"],
  "style": "Short, blunt, ALL CAPS when angry.",
  "vocabulary": ["mamma mia", "basta"],
  "rules": ["Never accept pineapple on pizza", "- Complain about tourists"],
  "language": "English with Italian words",
  "emoji": {"usage": "some", "favorites": "🍝 🍅"},
  "messageLength": "medium",
  "goal": "Find out what they ate this week.",
  "decisionHint": "jumps in on food talk",
  "fallbackReply": "Basta, what are you saying?"
}` + "\n```\nEnjoy!"
	p, err := ParseDraft(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "" || p.BuiltIn || p.Name != "Gino" || p.Tagline != "Chef who hates {shortcuts}" {
		t.Errorf("basic fields: %+v", p)
	}
	if p.Avatar.Glyph != "coffee" || p.Avatar.Kind != "generated" || len(p.Avatar.Gradient) != 2 {
		t.Errorf("avatar: %+v", p.Avatar)
	}
	if p.Personality != "grumpy\nproud\nsecretly soft" || p.Vocabulary != "mamma mia, basta" {
		t.Errorf("array fields: %q / %q", p.Personality, p.Vocabulary)
	}
	if p.Rules != "- Never accept pineapple on pizza\n- Complain about tourists" {
		t.Errorf("rules: %q", p.Rules)
	}
	if p.Emoji.Usage != "some" || len(p.Emoji.Favorites) != 2 || p.MessageLength != "medium" {
		t.Errorf("emoji/length: %+v %q", p.Emoji, p.MessageLength)
	}

	p, _ = ParseDraft(`{"name":"X","rules":"- Be rude. - Never smile. - Love techno."}`)
	if p.Rules != "- Be rude.\n- Never smile.\n- Love techno." {
		t.Errorf("inline bullets: %q", p.Rules)
	}
	if !json.Valid(Builder("x", "").Schema) {
		t.Error("builder schema must be valid JSON")
	}

	if _, err := ParseDraft("no json here"); err != ErrNoJSON {
		t.Errorf("want ErrNoJSON, got %v", err)
	}
	p, err = ParseDraft(`{"tagline":"x"}`)
	if err != nil || p.Name != "New Persona" || p.Goal == "" || p.FallbackReply == "" {
		t.Errorf("defaults: %+v %v", p, err)
	}
}

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		`x {"a":"}"} y {"b":1}`: `{"a":"}"}`,
		`{"a":{"b":"\"}"}}`:     `{"a":{"b":"\"}"}}`,
		`{"unterminated":`:      "",
		`none`:                  "",
	}
	for in, want := range cases {
		if got := ExtractJSON(in); got != want {
			t.Errorf("ExtractJSON(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGuidanceLengthBias(t *testing.T) {
	long := strings.Repeat("word ", 30)
	cases := []struct{ length, last, bias, want string }{
		{"short", "hi", "longer", "Short: one or two sentences — at most 19 words."},
		{"medium", "hi", "shorter", "Short: one or two sentences — at most 18 words."},
		{"short", "hi", "shorter", "Keep it brief: one sentence, two short ones at most — at most 7 words."},
		{"long", long, "longer", "A full paragraph is fine when there's something to say — at most 120 words."},
		{"medium", long, "longer", "Up to a short paragraph (three to five sentences) — at most 60 words."},
		{"medium", "hi", "", "Two or three sentences — at most 30 words."},
		{"long", "ok", "match", "Ultra brief: a few words, at most one short sentence — at most 4 words."},
	}
	for _, c := range cases {
		if got := Guidance(c.length, c.last, c.bias); !strings.HasPrefix(got, c.want) {
			t.Errorf("Guidance(%q, %q) = %q, want %q", c.length, c.bias, got, c.want)
		}
	}
	p := leo(t)
	p.MessageLength = "medium"
	sys := SystemPrompt(p, model.ChatAssignment{}, false, "hey", Options{LengthBias: "longer"})
	golden(t, "leo_dm_longer.golden", sys)
	if !strings.HasSuffix(sys, "GUIDANCE: Up to a short paragraph (three to five sentences) — at most 48 words. This length rule wins over any length mentioned above.") {
		t.Errorf("bias not applied: %q", sys[len(sys)-60:])
	}
	if SystemPrompt(leo(t), model.ChatAssignment{}, false, "hey", Options{LengthBias: "normal"}) != SystemPrompt(leo(t), model.ChatAssignment{}, false, "hey") {
		t.Error("normal bias must not change the prompt")
	}
}

func TestInitiatePrompt(t *testing.T) {
	hist := []model.Message{
		{Speaker: "them", Name: "Dana", Text: "talk soon!"},
		{Speaker: "me", Text: "bye darling"},
	}
	req := Initiate(leo(t), model.ChatAssignment{}, hist, false)
	if !strings.HasSuffix(req.System, InitiateNote) || !strings.Contains(req.System, "You are Leo") && !strings.Contains(req.System, "- Name: Leo") {
		t.Errorf("system: %q", req.System)
	}
	n := len(req.Messages)
	if n != 3 || req.Messages[1].Role != llm.RoleAssistant || req.Messages[1].Content != "bye darling" ||
		req.Messages[n-1].Role != llm.RoleUser || req.Messages[n-1].Content != InitiateTurn {
		t.Errorf("messages: %+v", req.Messages)
	}
	// History ending with their message: the synthetic turn is merged, last turn stays user.
	req = Initiate(leo(t), model.ChatAssignment{}, hist[:1], true)
	if len(req.Messages) != 1 || !strings.HasSuffix(req.Messages[0].Content, InitiateTurn) || !strings.HasPrefix(req.Messages[0].Content, "Dana: ") {
		t.Errorf("group messages: %+v", req.Messages)
	}
}
