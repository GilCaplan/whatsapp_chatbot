package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"whatsappdoppel/internal/model"
)

// OpenAIPreferred is the order in which a default model is picked from /models.
var OpenAIPreferred = []string{"gpt-5-mini", "gpt-5", "gpt-4.1-mini", "gpt-4.1", "gpt-4o-mini", "gpt-4o"}

// OpenAI speaks the Chat Completions API (also works with compatible servers).
type OpenAI struct {
	base string
	key  string
	hc   *http.Client
}

func NewOpenAI(baseURL, apiKey string, hc *http.Client) *OpenAI {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if hc == nil {
		hc = http.DefaultClient
	}
	return &OpenAI{base: strings.TrimRight(baseURL, "/"), key: apiKey, hc: hc}
}

func (o *OpenAI) Name() string { return "openai" }

type openaiChatReq struct {
	Model               string         `json:"model"`
	Messages            []Message      `json:"messages"`
	MaxCompletionTokens int            `json:"max_completion_tokens,omitempty"`
	Temperature         *float64       `json:"temperature,omitempty"`
	ResponseFormat      map[string]any `json:"response_format,omitempty"`
}

type openaiChatResp struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
			Refusal string `json:"refusal"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (o *OpenAI) Chat(ctx context.Context, req Request) (Response, error) {
	if o.key == "" {
		return Response{}, fmt.Errorf("openai: %w", ErrNoAPIKey)
	}
	if req.Model == "" {
		req.Model = FallbackOpenAIModel
	}
	msgs := make([]Message, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, Message{Role: RoleSystem, Content: req.System})
	}
	msgs = append(msgs, Normalize(req.Messages)...)
	body := openaiChatReq{Model: req.Model, Messages: msgs, MaxCompletionTokens: openaiMaxTokens(req)}
	if req.Temperature >= 0 {
		t := req.Temperature
		body.Temperature = &t
	}
	if req.JSON {
		body.ResponseFormat = map[string]any{"type": "json_object"}
	}
	raw, err := o.post(ctx, "/chat/completions", body)
	var apiErr *APIError
	if err != nil && body.Temperature != nil && errors.As(err, &apiErr) && apiErr.Status == http.StatusBadRequest &&
		strings.Contains(strings.ToLower(apiErr.Message), "temperature") {
		// Reasoning models only accept the default temperature.
		body.Temperature = nil
		raw, err = o.post(ctx, "/chat/completions", body)
	}
	if err != nil {
		return Response{}, err
	}
	var r openaiChatResp
	if err := json.Unmarshal(raw, &r); err != nil {
		return Response{}, fmt.Errorf("openai: bad response: %w", err)
	}
	if len(r.Choices) == 0 {
		return Response{}, ErrEmptyReply
	}
	if r.Choices[0].Message.Refusal != "" {
		return Response{}, ErrRefused
	}
	text := strings.TrimSpace(r.Choices[0].Message.Content)
	if text == "" {
		return Response{}, ErrEmptyReply
	}
	return Response{Text: text, Model: r.Model, InputTokens: r.Usage.PromptTokens, OutputTokens: r.Usage.CompletionTokens, Raw: string(raw)}, nil
}

// openaiMaxTokens leaves room for hidden reasoning tokens on reasoning models.
func openaiMaxTokens(req Request) int {
	n := req.MaxTokens
	m := strings.ToLower(req.Model)
	reasoning := strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4") || strings.HasPrefix(m, "gpt-5")
	if reasoning && n < 2048 {
		n = 2048
	}
	return n
}

func (o *OpenAI) ListModels(ctx context.Context) ([]model.ModelInfo, error) {
	if o.key == "" {
		return nil, fmt.Errorf("openai: %w", ErrNoAPIKey)
	}
	raw, err := o.get(ctx, "/models")
	if err != nil {
		return nil, err
	}
	var r struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("openai: bad /models response: %w", err)
	}
	out := []model.ModelInfo{}
	for _, m := range r.Data {
		if openaiChatModel(m.ID) {
			out = append(out, model.ModelInfo{ID: m.ID, Label: m.ID, Meta: map[string]any{"ownedBy": m.OwnedBy}})
		}
	}
	slices.SortFunc(out, func(a, b model.ModelInfo) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

// openaiChatModel keeps chat-completion models and drops embeddings, audio, images etc.
func openaiChatModel(id string) bool {
	m := strings.ToLower(id)
	if !(strings.HasPrefix(m, "gpt-") || strings.HasPrefix(m, "chatgpt-") ||
		strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4")) {
		return false
	}
	for _, bad := range []string{"embedding", "tts", "whisper", "dall-e", "image", "audio", "realtime",
		"transcribe", "search", "moderation", "instruct", "codex", "computer-use", "deep-research"} {
		if strings.Contains(m, bad) {
			return false
		}
	}
	return true
}

func (o *OpenAI) Ping(ctx context.Context) error {
	if o.key == "" {
		return fmt.Errorf("openai: %w", ErrNoAPIKey)
	}
	_, err := o.get(ctx, "/models")
	return err
}

func (o *OpenAI) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.base+path, nil)
	if err != nil {
		return nil, err
	}
	return o.do(req)
}

func (o *OpenAI) post(ctx context.Context, path string, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.base+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return o.do(req)
}

func (o *OpenAI) do(req *http.Request) ([]byte, error) {
	req.Header.Set("Authorization", "Bearer "+o.key)
	resp, err := o.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("openai: %w", ErrInvalidKey)
	}
	if resp.StatusCode/100 != 2 {
		msg := snippet(string(body), 300)
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		return nil, &APIError{Provider: "openai", Status: resp.StatusCode, Message: msg}
	}
	return body, nil
}
