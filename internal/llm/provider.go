// Package llm talks to the language-model backends (Ollama, Anthropic, OpenAI,
// plus a scripted fake) behind one small Provider interface.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"whatsappdoppel/internal/model"
)

// Message roles.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Per-call timeouts. The shared http.Client has no timeout of its own.
const (
	TimeoutReply   = 90 * time.Second
	TimeoutDecide  = 10 * time.Second
	TimeoutBuilder = 120 * time.Second
	TimeoutPing    = 5 * time.Second
)

// Default models per provider (used when settings leave the model empty).
const (
	DefaultOllamaModel    = "llama3.1:8b"
	DefaultAnthropicModel = "claude-sonnet-5-5"
	FallbackOpenAIModel   = "gpt-4o-mini"
)

var (
	// ErrRefused means the model declined to answer (Anthropic stop_reason "refusal").
	ErrRefused = errors.New("the model refused to answer")
	// ErrNoAPIKey means the provider needs an API key that is not configured.
	ErrNoAPIKey = errors.New("no API key configured")
	// ErrInvalidKey means the provider rejected the API key (HTTP 401).
	ErrInvalidKey = errors.New("invalid API key")
	// ErrEmptyReply means the provider answered with no text.
	ErrEmptyReply = errors.New("the model returned an empty reply")
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Request is a provider-neutral chat request. Temperature < 0 means "provider default".
// Schema optionally constrains JSON mode (used by Ollama structured outputs).
type Request struct {
	Model       string
	System      string
	Messages    []Message
	MaxTokens   int
	Temperature float64
	JSON        bool
	Schema      json.RawMessage
}

type Response struct {
	Text         string
	Model        string
	InputTokens  int
	OutputTokens int
	Raw          string
}

type Provider interface {
	Name() string
	Chat(ctx context.Context, req Request) (Response, error)
	ListModels(ctx context.Context) ([]model.ModelInfo, error)
	Ping(ctx context.Context) error
}

// APIError is a non-2xx answer from an HTTP provider.
type APIError struct {
	Provider string
	Status   int
	Message  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: HTTP %d: %s", e.Provider, e.Status, e.Message)
}

// Normalize prepares messages for providers that require strict user/assistant
// alternation: system turns and empty messages are dropped, consecutive same-role
// messages are merged with "\n", leading assistant turns are removed and the
// conversation always ends with a user turn.
func Normalize(msgs []Message) []Message {
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		text := strings.TrimSpace(m.Content)
		if text == "" || (m.Role != RoleUser && m.Role != RoleAssistant) {
			continue
		}
		if len(out) == 0 && m.Role == RoleAssistant {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Role == m.Role {
			out[n-1].Content += "\n" + text
			continue
		}
		out = append(out, Message{Role: m.Role, Content: text})
	}
	if len(out) == 0 {
		return []Message{{Role: RoleUser, Content: "Hi"}}
	}
	if out[len(out)-1].Role != RoleUser {
		out = append(out, Message{Role: RoleUser, Content: "(continue the conversation)"})
	}
	return out
}

// snippet shortens provider error bodies for messages.
func snippet(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
