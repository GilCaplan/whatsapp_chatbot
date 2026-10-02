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

// Ollama talks to a local Ollama server over its native REST API.
type Ollama struct {
	base   string
	numCtx int
	hc     *http.Client
}

func NewOllama(baseURL string, numCtx int, hc *http.Client) *Ollama {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Ollama{base: strings.TrimRight(baseURL, "/"), numCtx: numCtx, hc: hc}
}

func (o *Ollama) Name() string { return "ollama" }

type ollamaChatReq struct {
	Model    string         `json:"model"`
	Messages []Message      `json:"messages"`
	Stream   bool           `json:"stream"`
	Think    *bool          `json:"think,omitempty"`
	Format   any            `json:"format,omitempty"`
	Options  map[string]any `json:"options,omitempty"`
}

type ollamaChatResp struct {
	Model   string `json:"model"`
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
	Error           string `json:"error"`
}

func (o *Ollama) Chat(ctx context.Context, req Request) (Response, error) {
	if req.Model == "" {
		req.Model = DefaultOllamaModel
	}
	msgs := make([]Message, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, Message{Role: RoleSystem, Content: req.System})
	}
	msgs = append(msgs, req.Messages...)
	opts := map[string]any{}
	if o.numCtx > 0 {
		opts["num_ctx"] = o.numCtx
	}
	if req.MaxTokens > 0 {
		opts["num_predict"] = req.MaxTokens
	}
	if req.Temperature >= 0 {
		opts["temperature"] = req.Temperature
	}
	think := false
	body := ollamaChatReq{Model: req.Model, Messages: msgs, Think: &think, Options: opts}
	if req.JSON {
		body.Format = "json"
		if len(req.Schema) > 0 {
			body.Format = req.Schema
		}
	}
	raw, err := o.post(ctx, "/api/chat", body)
	var apiErr *APIError
	if err != nil && errors.As(err, &apiErr) && apiErr.Status == http.StatusBadRequest &&
		strings.Contains(strings.ToLower(apiErr.Message), "think") {
		// Some models cannot toggle thinking; retry with the model default.
		body.Think = nil
		raw, err = o.post(ctx, "/api/chat", body)
	}
	if err != nil {
		return Response{}, err
	}
	var r ollamaChatResp
	if err := json.Unmarshal(raw, &r); err != nil {
		return Response{}, fmt.Errorf("ollama: bad response: %w", err)
	}
	if r.Error != "" {
		return Response{}, fmt.Errorf("ollama: %s", r.Error)
	}
	text := strings.TrimSpace(r.Message.Content)
	if text == "" {
		return Response{}, ErrEmptyReply
	}
	return Response{Text: text, Model: r.Model, InputTokens: r.PromptEvalCount, OutputTokens: r.EvalCount, Raw: string(raw)}, nil
}

type ollamaTag struct {
	Name         string   `json:"name"`
	Size         int64    `json:"size"`
	Capabilities []string `json:"capabilities"`
	Details      struct {
		Family            string `json:"family"`
		ParameterSize     string `json:"parameter_size"`
		QuantizationLevel string `json:"quantization_level"`
	} `json:"details"`
}

// ListModels returns installed chat-capable models (embedding models are hidden).
func (o *Ollama) ListModels(ctx context.Context) ([]model.ModelInfo, error) {
	raw, err := o.get(ctx, "/api/tags")
	if err != nil {
		return nil, err
	}
	var r struct {
		Models []ollamaTag `json:"models"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("ollama: bad /api/tags response: %w", err)
	}
	out := []model.ModelInfo{}
	for _, t := range r.Models {
		if !ollamaChatCapable(t) {
			continue
		}
		label := t.Name
		if t.Details.ParameterSize != "" {
			label += " (" + t.Details.ParameterSize + ")"
		}
		out = append(out, model.ModelInfo{ID: t.Name, Label: label, Meta: map[string]any{
			"size":          t.Size,
			"family":        t.Details.Family,
			"parameterSize": t.Details.ParameterSize,
			"quantization":  t.Details.QuantizationLevel,
		}})
	}
	return out, nil
}

func ollamaChatCapable(t ollamaTag) bool {
	if len(t.Capabilities) > 0 {
		return slices.Contains(t.Capabilities, "completion")
	}
	return !strings.Contains(strings.ToLower(t.Name), "embed")
}

func (o *Ollama) Ping(ctx context.Context) error {
	_, err := o.Version(ctx)
	return err
}

// Version returns the Ollama server version (GET /api/version).
func (o *Ollama) Version(ctx context.Context) (string, error) {
	raw, err := o.get(ctx, "/api/version")
	if err != nil {
		return "", err
	}
	var r struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", fmt.Errorf("ollama: bad /api/version response: %w", err)
	}
	return r.Version, nil
}

// PullUpdate is one NDJSON line of /api/pull progress.
type PullUpdate struct {
	Status    string `json:"status"`
	Completed int64  `json:"completed"`
	Total     int64  `json:"total"`
	Error     string `json:"error"`
}

// Pull downloads a model, calling fn for each progress line.
func (o *Ollama) Pull(ctx context.Context, name string, fn func(PullUpdate)) error {
	b, _ := json.Marshal(map[string]any{"name": name, "model": name, "stream": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.base+"/api/pull", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.hc.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &APIError{Provider: "ollama", Status: resp.StatusCode, Message: ollamaErrMsg(body)}
	}
	dec := json.NewDecoder(resp.Body)
	for {
		var u PullUpdate
		if err := dec.Decode(&u); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("ollama: pull stream: %w", err)
		}
		if u.Error != "" {
			return fmt.Errorf("ollama: %s", u.Error)
		}
		if fn != nil {
			fn(u)
		}
	}
}

func (o *Ollama) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.base+path, nil)
	if err != nil {
		return nil, err
	}
	return o.do(req)
}

func (o *Ollama) post(ctx context.Context, path string, body any) ([]byte, error) {
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

func (o *Ollama) do(req *http.Request) ([]byte, error) {
	resp, err := o.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, &APIError{Provider: "ollama", Status: resp.StatusCode, Message: ollamaErrMsg(body)}
	}
	return body, nil
}

func ollamaErrMsg(body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		return e.Error
	}
	return snippet(string(body), 300)
}
