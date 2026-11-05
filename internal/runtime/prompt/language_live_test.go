package prompt

import (
	"context"
	"os"
	"testing"
	"time"

	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/logic/lang"
	"whatsappdoppel/internal/runtime/llm"
)

// TestLiveFixedLanguage counts how often a local model keeps a language set in
// the settings when the whole chat is in another one (a Hebrew group, Russian
// rule, a persona whose profile says English only). The old rule is logged next
// to it. Run with DOPPEL_LIVE_OLLAMA=1.
func TestLiveFixedLanguage(t *testing.T) {
	if os.Getenv("DOPPEL_LIVE_OLLAMA") == "" {
		t.Skip("set DOPPEL_LIVE_OLLAMA=1 to run against a local Ollama")
	}
	o := llm.NewOllama("http://127.0.0.1:11434", 8192, nil)
	hist := []model.Message{
		{Speaker: "them", Name: "Ben", Text: "מישהו יודע מתי המבחן בדאטה?"},
		{Speaker: "them", Name: "טל", Text: "אני חושב שזה ביום ראשון הבא"},
		{Speaker: "me", FromBot: true, Text: "זה יפה, אז כולם היכן?"},
		{Speaker: "them", Name: "Ben", Text: "לאו אתה יודע מה יש במבחן?"},
		{Speaker: "them", Name: "טל", Text: "כן תגיד לנו מה עם החומר"},
	}
	const runs = 10
	for _, c := range []struct {
		name  string
		fixed bool
	}{{"old", false}, {"new", true}} {
		ok := 0
		for i := range runs {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			req := Compose(leo(t), model.ChatAssignment{}, hist, true, Options{ReplyLanguage: "Russian", LanguageFixed: c.fixed})
			req.Model, req.MaxTokens, req.Temperature = llm.DefaultOllamaModel, 200, 0.8
			resp, err := o.Chat(ctx, req)
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			code, _ := lang.Detect(resp.Text)
			if code == "ru" {
				ok++
			}
			t.Logf("%s %d [%s] %q", c.name, i, code, resp.Text)
		}
		t.Logf("%s: %d of %d in Russian", c.name, ok, runs)
	}
}
