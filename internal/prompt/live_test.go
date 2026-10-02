package prompt

import (
	"context"
	"os"
	"testing"
	"time"

	"whatsappdoppel/internal/guard"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
)

// TestLivePrompts runs Leo and the persona builder against a real local Ollama.
// Run with DOPPEL_LIVE_OLLAMA=1.
func TestLivePrompts(t *testing.T) {
	if os.Getenv("DOPPEL_LIVE_OLLAMA") == "" {
		t.Skip("set DOPPEL_LIVE_OLLAMA=1 to run against a local Ollama")
	}
	o := llm.NewOllama("http://127.0.0.1:11434", 8192, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	req := Compose(leo(t), model.ChatAssignment{}, []model.Message{
		{Speaker: "them", Text: "hey! what do you think of beige sofas?"},
	}, false)
	req.Model, req.MaxTokens, req.Temperature = llm.DefaultOllamaModel, 400, 0.8
	resp, err := o.Chat(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Leo: %q (broke character: %v)", resp.Text, guard.BrokeCharacter(resp.Text, "Leo"))

	b := Builder("a sarcastic barista from Tel Aviv who loves techno", "")
	b.Model = llm.DefaultOllamaModel
	resp, err = o.Chat(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ParseDraft(resp.Text)
	if err != nil {
		t.Fatalf("ParseDraft: %v\nraw: %s", err, resp.Text)
	}
	t.Logf("raw: %s", resp.Text)
	t.Logf("draft: name=%q tagline=%q glyph=%q usage=%q len=%q\nrules:\n%s", p.Name, p.Tagline, p.Avatar.Glyph, p.Emoji.Usage, p.MessageLength, p.Rules)
}
