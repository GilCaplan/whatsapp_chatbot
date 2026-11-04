package llm

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveOllama talks to a real local Ollama. Run with DOPPEL_LIVE_OLLAMA=1
// (optionally DOPPEL_OLLAMA_MODEL, default llama3.1:8b).
func TestLiveOllama(t *testing.T) {
	if os.Getenv("DOPPEL_LIVE_OLLAMA") == "" {
		t.Skip("set DOPPEL_LIVE_OLLAMA=1 to run against a local Ollama")
	}
	mdl := os.Getenv("DOPPEL_OLLAMA_MODEL")
	if mdl == "" {
		mdl = DefaultOllamaModel
	}
	o := NewOllama("http://127.0.0.1:11434", 8192, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	models, err := o.ListModels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("models: %+v", models)
	resp, err := o.Chat(ctx, Request{Model: mdl, Messages: []Message{{RoleUser, "Say hi in 3 words"}}, MaxTokens: 30, Temperature: 0.8})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("reply: %q (%d in / %d out)", resp.Text, resp.InputTokens, resp.OutputTokens)
	resp, err = o.Chat(ctx, Request{Model: mdl, Messages: []Message{{RoleUser, "Is the sky blue? Reply with only YES or NO."}}, MaxTokens: 5, Temperature: 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("decision: %q", resp.Text)
	resp, err = o.Chat(ctx, Request{Model: mdl, System: "Reply with a JSON object {\"name\": string}.", Messages: []Message{{RoleUser, "Invent a name"}}, MaxTokens: 50, Temperature: 0.7, JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("json: %q", resp.Text)
}
