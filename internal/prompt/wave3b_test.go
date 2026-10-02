package prompt

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

func TestHandoffCheckRequest(t *testing.T) {
	hist := []model.Message{
		{Speaker: "them", Name: "Dana", Text: "hey"},
		{Speaker: "me", Text: "hii", FromBot: true},
	}
	req := HandoffCheck("Leo", []string{model.HandoffMoney, model.HandoffBot}, hist, "Dana", "I still owe you for the pizza")
	if !req.JSON || req.MaxTokens > 60 || req.Temperature != 0 || !json.Valid(req.Schema) {
		t.Fatalf("request = %+v", req)
	}
	q := req.Messages[0].Content
	for _, want := range []string{"money:", "bot:", "Leo: hii", `Latest message from Dana: "I still owe you for the pizza"`, `"none"`} {
		if !strings.Contains(q, want) {
			t.Errorf("prompt lacks %q:\n%s", want, q)
		}
	}
	if strings.Contains(q, "health:") {
		t.Error("disabled categories must not be listed")
	}
}

func TestParseHandoffCheck(t *testing.T) {
	cats := []string{model.HandoffMoney, model.HandoffBot}
	cases := []struct {
		raw     string
		cat     string
		serious bool
	}{
		{`{"category":"money","serious":true}`, "money", true},
		{`sure: {"category": "Money", "serious": "yes"}`, "money", true},
		{`{"category":"money","serious":false}`, "", false},
		{`{"category":"none","serious":true}`, "", false},
		{`{"category":"health","serious":true}`, "", false}, // not enabled
		{`no idea`, "", false},
	}
	for _, c := range cases {
		cat, ok := ParseHandoffCheck(c.raw, cats)
		if cat != c.cat || ok != c.serious {
			t.Errorf("%s = %q %v", c.raw, cat, ok)
		}
	}
}

func TestDraftsRequest(t *testing.T) {
	p := model.Persona{ID: "leo", Name: "Leo"}
	base := Compose(p, model.ChatAssignment{Kind: "dm"}, []model.Message{{Speaker: "them", Text: "dinner tonight?"}}, false)
	req := Drafts(base)
	if !req.JSON || !json.Valid(req.Schema) || !strings.HasSuffix(req.System, DraftsNote) || !strings.HasPrefix(req.System, base.System) {
		t.Fatalf("drafts request = %+v", req)
	}
	if len(req.Messages) != 1 || req.Messages[0].Content != "dinner tonight?" {
		t.Errorf("messages = %+v", req.Messages)
	}
}

func TestParseDrafts(t *testing.T) {
	tones := func(ds []model.Draft) string {
		var out []string
		for _, d := range ds {
			out = append(out, d.Tone+"="+d.Text)
		}
		return strings.Join(out, " | ")
	}
	cases := []struct{ raw, want string }{
		{`{"drafts":[{"tone":"brief","text":"sure"},{"tone":"warm","text":"yes! where?"},{"tone":"playful","text":"only if you cook"}]}`,
			"brief=sure | warm=yes! where? | playful=only if you cook"},
		{"Here you go:\n```json\n{\"drafts\":[{\"tone\":\"warm\",\"text\":\"b\"},{\"text\":\"a\"},{\"tone\":\"warm\",\"text\":\"c\"}]}\n```",
			"warm=b | brief=a | playful=c"},
		{`[{"style":"brief","reply":"x"},"y"]`, "brief=x | warm=y"},
		{`{"brief":"k","warm":"ok sounds good","playful":"race you there"}`, "brief=k | warm=ok sounds good | playful=race you there"},
		{"1) brief: sure\n2) warm: yes! what time?\n3) playful: only if there's dessert", "brief=sure | warm=yes! what time? | playful=only if there's dessert"},
		{`{"drafts":[{"tone":"brief","text":"  "},{"tone":"warm","text":"hi"}]}`, "warm=hi"},
		{`{"drafts":[]}`, ""},
		{`nope`, ""},
	}
	for _, c := range cases {
		if got := tones(ParseDrafts(c.raw)); got != c.want {
			t.Errorf("%q\n got %q\nwant %q", c.raw, got, c.want)
		}
	}
}

func TestRecapRequestAndParse(t *testing.T) {
	ts := time.Date(2026, 10, 1, 18, 5, 0, 0, time.UTC)
	req := Recap(RecapInput{
		ChatName: "Dana", PersonaName: "Leo", Goal: "Find out her favourite band",
		Messages: []model.Message{
			{TS: ts, Speaker: "them", Name: "Dana", Text: "exam on friday, so stressed"},
			{TS: ts, Speaker: "me", FromBot: true, Text: "you'll smash it"},
			{TS: ts, Speaker: "me", Text: "call you later"},
		},
		Memories: []string{"Dana: exam on Friday"},
		Handoff:  &model.HandoffState{Category: model.HandoffHealth, Excerpt: "I'm in hospital"},
		Zone:     time.UTC,
	})
	q := req.Messages[0].Content
	for _, want := range []string{"[18:05] Dana: exam on friday", "Leo: you'll smash it", "Owner: call you later", "secret goal", "Dana: exam on Friday", "paused", `"goalProgress": "one short`} {
		if !strings.Contains(q, want) {
			t.Errorf("prompt lacks %q:\n%s", want, q)
		}
	}
	if !req.JSON || !json.Valid(req.Schema) || req.MaxTokens != 450 {
		t.Errorf("request = %+v", req)
	}

	rc, err := ParseRecap(`ok {"headline":"Dana is stressed about Friday's exam","topics":["exam","exam","weekend","a","b","c"],"goalProgress":"","toKnow":"Wish her luck on Friday; she asked about Sunday","mood":"supportive"}`)
	if err != nil || rc.Headline == "" || len(rc.Topics) != 4 || rc.Topics[1] != "weekend" || len(rc.ToKnow) != 2 || rc.Mood != "supportive" {
		t.Fatalf("ParseRecap = %+v %v", rc, err)
	}
	if _, err := ParseRecap(`{"topics":["x"]}`); err == nil {
		t.Error("a recap without a headline is an error")
	}
	if _, err := ParseRecap("nothing"); err == nil {
		t.Error("no JSON is an error")
	}
}
