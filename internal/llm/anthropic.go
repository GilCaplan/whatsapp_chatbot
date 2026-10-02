package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"whatsappdoppel/internal/model"
)

// anthropicStatic is shown when no key is set or the live list is unavailable.
var anthropicStatic = []model.ModelInfo{
	{ID: "claude-sonnet-5-5", Label: "Claude Sonnet 5.5", Meta: map[string]any{"default": true}},
	{ID: "claude-opus-5-5", Label: "Claude Opus 5.5"},
	{ID: "claude-haiku-4-5", Label: "Claude Haiku 4.5"},
}

// Anthropic uses the official SDK (retries, typed errors).
type Anthropic struct {
	key    string
	client anthropic.Client
}

func NewAnthropic(apiKey string, hc *http.Client) *Anthropic {
	return newAnthropic(apiKey, hc)
}

func newAnthropic(apiKey string, hc *http.Client, extra ...option.RequestOption) *Anthropic {
	opts := []option.RequestOption{option.WithAPIKey(apiKey), option.WithMaxRetries(2)}
	if hc != nil {
		opts = append(opts, option.WithHTTPClient(hc))
	}
	return &Anthropic{key: apiKey, client: anthropic.NewClient(append(opts, extra...)...)}
}

func (a *Anthropic) Name() string { return "anthropic" }

func (a *Anthropic) Chat(ctx context.Context, req Request) (Response, error) {
	if a.key == "" {
		return Response{}, fmt.Errorf("anthropic: %w", ErrNoAPIKey)
	}
	if req.Model == "" {
		req.Model = DefaultAnthropicModel
	}
	params := anthropic.MessageNewParams{
		Model:     req.Model,
		MaxTokens: int64(anthropicMaxTokens(req)),
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}
	for _, m := range Normalize(req.Messages) {
		if m.Role == RoleAssistant {
			params.Messages = append(params.Messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
		} else {
			params.Messages = append(params.Messages, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		}
	}
	if req.Temperature >= 0 && anthropicSamplingAllowed(req.Model) {
		params.Temperature = anthropic.Float(min(req.Temperature, 1))
	}
	if anthropicEffortSupported(req.Model) {
		// Short chat replies don't need deep thinking; keep latency and cost low.
		params.OutputConfig.Effort = anthropic.OutputConfigEffortLow
		if req.JSON {
			params.OutputConfig.Effort = anthropic.OutputConfigEffortMedium
		}
	}
	if req.JSON && len(req.Schema) > 0 {
		var schema map[string]any
		if json.Unmarshal(req.Schema, &schema) == nil {
			params.OutputConfig.Format = anthropic.JSONOutputFormatParam{Schema: schema}
		}
	}

	msg, err := a.client.Messages.New(ctx, params)
	// Defensive retries for models that reject a parameter we guessed was supported.
	for range 3 {
		if err == nil || anthropicStatus(err) != http.StatusBadRequest {
			break
		}
		body := strings.ToLower(anthropicErrBody(err))
		switch {
		case strings.Contains(body, "temperature") && !param.IsOmitted(params.Temperature):
			params.Temperature = param.Opt[float64]{}
		case strings.Contains(body, "effort") && params.OutputConfig.Effort != "":
			params.OutputConfig.Effort = ""
		case (strings.Contains(body, "format") || strings.Contains(body, "schema")) && params.OutputConfig.Format.Schema != nil:
			params.OutputConfig.Format = anthropic.JSONOutputFormatParam{}
		default:
			return Response{}, anthropicError(err)
		}
		msg, err = a.client.Messages.New(ctx, params)
	}
	if err != nil {
		return Response{}, anthropicError(err)
	}
	if msg.StopReason == anthropic.StopReasonRefusal {
		return Response{}, ErrRefused
	}
	var sb strings.Builder
	for _, block := range msg.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			sb.WriteString(tb.Text)
		}
	}
	text := strings.TrimSpace(sb.String())
	if text == "" {
		return Response{}, ErrEmptyReply
	}
	return Response{
		Text:         text,
		Model:        msg.Model,
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
		Raw:          msg.RawJSON(),
	}, nil
}

// anthropicMaxTokens leaves headroom for adaptive thinking on models where it is
// always on; max_tokens is only a cap, so this does not raise cost for short replies.
func anthropicMaxTokens(req Request) int {
	n := req.MaxTokens
	if n <= 0 {
		n = 1024
	}
	if anthropicEffortSupported(req.Model) && n < 1024 {
		n = 1024
	}
	return n
}

// anthropicSamplingAllowed reports whether the model accepts a custom temperature.
// The 4.7+ / 5.x generation rejects sampling parameters.
func anthropicSamplingAllowed(m string) bool {
	m = strings.ToLower(m)
	if strings.Contains(m, "haiku") {
		return true
	}
	for _, p := range []string{"claude-3", "claude-sonnet-4-0", "claude-sonnet-4-2", "claude-sonnet-4-5", "claude-sonnet-4-6",
		"claude-opus-4-0", "claude-opus-4-1", "claude-opus-4-2", "claude-opus-4-5", "claude-opus-4-6"} {
		if strings.HasPrefix(m, p) {
			return true
		}
	}
	return false
}

// anthropicEffortSupported reports whether output_config.effort is accepted
// (Opus 4.5+, Sonnet 4.6+, all 5.x; not Haiku 4.5 or Sonnet 4.5).
func anthropicEffortSupported(m string) bool {
	m = strings.ToLower(m)
	if strings.Contains(m, "haiku") {
		return false
	}
	for _, p := range []string{"claude-3", "claude-sonnet-4-0", "claude-sonnet-4-2", "claude-sonnet-4-5",
		"claude-opus-4-0", "claude-opus-4-1", "claude-opus-4-2"} {
		if strings.HasPrefix(m, p) {
			return false
		}
	}
	return true
}

func (a *Anthropic) ListModels(ctx context.Context) ([]model.ModelInfo, error) {
	if a.key == "" {
		return anthropicStatic, nil
	}
	pager := a.client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{Limit: anthropic.Int(100)})
	var out []model.ModelInfo
	for pager.Next() {
		m := pager.Current()
		label := m.DisplayName
		if label == "" {
			label = m.ID
		}
		meta := map[string]any{"contextWindow": m.MaxInputTokens, "maxOutput": m.MaxTokens}
		if m.ID == DefaultAnthropicModel {
			meta["default"] = true
		}
		out = append(out, model.ModelInfo{ID: m.ID, Label: label, Meta: meta})
	}
	if err := pager.Err(); err != nil {
		return nil, anthropicError(err)
	}
	if len(out) == 0 {
		return anthropicStatic, nil
	}
	return out, nil
}

// Ping validates the key with a one-item model listing.
func (a *Anthropic) Ping(ctx context.Context) error {
	if a.key == "" {
		return fmt.Errorf("anthropic: %w", ErrNoAPIKey)
	}
	_, err := a.client.Models.List(ctx, anthropic.ModelListParams{Limit: anthropic.Int(1)})
	if err != nil {
		return anthropicError(err)
	}
	return nil
}

func anthropicStatus(err error) int {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode
	}
	return 0
}

func anthropicErrBody(err error) string {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		return apiErr.RawJSON()
	}
	return ""
}

// anthropicError turns SDK errors into short, user-facing messages.
func anthropicError(err error) error {
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("anthropic: %w", err)
	}
	msg := apiErr.RawJSON()
	var env struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(msg), &env) == nil && env.Error.Message != "" {
		msg = env.Error.Message
	}
	switch apiErr.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("anthropic: %w", ErrInvalidKey)
	case http.StatusForbidden:
		return &APIError{Provider: "anthropic", Status: 403, Message: "this API key is not allowed to use that model (" + snippet(msg, 200) + ")"}
	case http.StatusNotFound:
		return &APIError{Provider: "anthropic", Status: 404, Message: "model not found (" + snippet(msg, 200) + ")"}
	case http.StatusTooManyRequests:
		return &APIError{Provider: "anthropic", Status: 429, Message: "rate limited, try again shortly"}
	case 529:
		return &APIError{Provider: "anthropic", Status: 529, Message: "Anthropic is overloaded, try again shortly"}
	}
	return &APIError{Provider: "anthropic", Status: apiErr.StatusCode, Message: snippet(msg, 300)}
}
