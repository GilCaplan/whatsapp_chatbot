package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
)

func TestNormalize(t *testing.T) {
	in := []Message{
		{RoleAssistant, "leading assistant dropped"},
		{RoleSystem, "system dropped"},
		{RoleUser, "a"},
		{RoleUser, "b"},
		{RoleUser, "  "},
		{RoleAssistant, "c"},
		{RoleAssistant, "d"},
	}
	got := Normalize(in)
	want := []Message{{RoleUser, "a\nb"}, {RoleAssistant, "c\nd"}, {RoleUser, "(continue the conversation)"}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Normalize = %+v", got)
	}
	if got := Normalize(nil); len(got) != 1 || got[0].Role != RoleUser {
		t.Errorf("empty → %+v", got)
	}
}

type capture struct {
	mu     sync.Mutex
	bodies []map[string]any
	paths  []string
}

func (c *capture) add(r *http.Request) map[string]any {
	b, _ := io.ReadAll(r.Body)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	c.mu.Lock()
	c.bodies = append(c.bodies, m)
	c.paths = append(c.paths, r.URL.Path)
	c.mu.Unlock()
	return m
}

func TestOllama(t *testing.T) {
	var cap capture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/chat":
			cap.add(r)
			fmt.Fprint(w, `{"model":"llama3.1:8b","message":{"role":"assistant","content":"  hey darling "},"prompt_eval_count":12,"eval_count":4}`)
		case "/api/tags":
			fmt.Fprint(w, `{"models":[
				{"name":"llama3.1:8b","size":4900000000,"capabilities":["completion","tools"],"details":{"family":"llama","parameter_size":"8.0B"}},
				{"name":"nomic-embed-text:latest","capabilities":["embedding"]},
				{"name":"old-embed:latest"},
				{"name":"mistral:7b"}]}`)
		case "/api/version":
			fmt.Fprint(w, `{"version":"0.33.0"}`)
		case "/api/pull":
			cap.add(r)
			fmt.Fprintln(w, `{"status":"pulling manifest"}`)
			fmt.Fprintln(w, `{"status":"pulling abc","completed":50,"total":100}`)
			fmt.Fprintln(w, `{"status":"success"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	o := NewOllama(srv.URL, 8192, srv.Client())
	resp, err := o.Chat(context.Background(), Request{
		Model: "llama3.1:8b", System: "sys", Messages: []Message{{RoleUser, "hi"}},
		MaxTokens: 5, Temperature: 0, JSON: true,
	})
	if err != nil || resp.Text != "hey darling" || resp.InputTokens != 12 || resp.OutputTokens != 4 {
		t.Fatalf("chat: %+v %v", resp, err)
	}
	b := cap.bodies[0]
	opts := b["options"].(map[string]any)
	if b["think"] != false || b["stream"] != false || b["format"] != "json" ||
		opts["num_ctx"] != 8192.0 || opts["num_predict"] != 5.0 || opts["temperature"] != 0.0 {
		t.Errorf("chat body = %v", b)
	}
	msgs := b["messages"].([]any)
	if msgs[0].(map[string]any)["role"] != "system" || len(msgs) != 2 {
		t.Errorf("system message should come first: %v", msgs)
	}

	if _, err := o.Chat(context.Background(), Request{Messages: []Message{{RoleUser, "x"}}, JSON: true, Schema: json.RawMessage(`{"type":"object"}`)}); err != nil {
		t.Fatal(err)
	}
	if f, ok := cap.bodies[1]["format"].(map[string]any); !ok || f["type"] != "object" || cap.bodies[1]["model"] != DefaultOllamaModel {
		t.Errorf("schema should be sent as format: %v", cap.bodies[1])
	}

	models, err := o.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range models {
		ids = append(ids, m.ID)
	}
	if strings.Join(ids, ",") != "llama3.1:8b,mistral:7b" {
		t.Errorf("models = %v (embedding models must be hidden)", ids)
	}
	if v, err := o.Version(context.Background()); err != nil || v != "0.33.0" {
		t.Errorf("version %q %v", v, err)
	}
	var updates []PullUpdate
	if err := o.Pull(context.Background(), "llama3.1:8b", func(u PullUpdate) { updates = append(updates, u) }); err != nil {
		t.Fatal(err)
	}
	if len(updates) != 3 || updates[1].Completed != 50 {
		t.Errorf("pull updates = %+v", updates)
	}
}

func TestOllamaErrorsAndThinkRetry(t *testing.T) {
	var cap capture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := cap.add(r)
		if _, ok := b["think"]; ok {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":"\"gpt-oss\" does not support think=false"}`)
			return
		}
		if b["model"] == "missing" {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":"model \"missing\" not found, try pulling it first"}`)
			return
		}
		fmt.Fprint(w, `{"message":{"content":"ok"}}`)
	}))
	defer srv.Close()
	o := NewOllama(srv.URL, 0, srv.Client())
	if r, err := o.Chat(context.Background(), Request{Model: "gpt-oss", Messages: []Message{{RoleUser, "x"}}, Temperature: -1}); err != nil || r.Text != "ok" {
		t.Errorf("think retry: %+v %v", r, err)
	}
	if opts, _ := cap.bodies[1]["options"].(map[string]any); opts["temperature"] != nil {
		t.Error("negative temperature should be omitted")
	}
	_, err := o.Chat(context.Background(), Request{Model: "missing", Messages: []Message{{RoleUser, "x"}}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 404 || !strings.Contains(apiErr.Message, "not found") {
		t.Errorf("want 404 APIError, got %v", err)
	}
}

func TestOpenAI(t *testing.T) {
	var cap capture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/chat/completions":
			b := cap.add(r)
			if _, ok := b["temperature"]; ok {
				w.WriteHeader(400)
				fmt.Fprint(w, `{"error":{"message":"Unsupported value: 'temperature' does not support 0.8 with this model."}}`)
				return
			}
			fmt.Fprint(w, `{"model":"gpt-5-mini","choices":[{"message":{"content":"yo"}}],"usage":{"prompt_tokens":7,"completion_tokens":2}}`)
		case "/models":
			fmt.Fprint(w, `{"data":[{"id":"gpt-5-mini"},{"id":"text-embedding-3-small"},{"id":"gpt-4o-mini-tts"},{"id":"o3"},{"id":"dall-e-3"},{"id":"gpt-4.1"}]}`)
		}
	}))
	defer srv.Close()
	o := NewOpenAI(srv.URL, "sk-test", srv.Client())
	resp, err := o.Chat(context.Background(), Request{Model: "gpt-5-mini", System: "s", Messages: []Message{{RoleAssistant, "drop"}, {RoleUser, "hi"}}, Temperature: 0.8, MaxTokens: 100, JSON: true})
	if err != nil || resp.Text != "yo" || resp.OutputTokens != 2 {
		t.Fatalf("chat: %+v %v", resp, err)
	}
	if len(cap.bodies) != 2 {
		t.Fatalf("expected a retry without temperature, got %d calls", len(cap.bodies))
	}
	b := cap.bodies[1]
	if b["max_completion_tokens"] != 2048.0 || b["response_format"].(map[string]any)["type"] != "json_object" {
		t.Errorf("body = %v", b)
	}
	if msgs := b["messages"].([]any); len(msgs) != 2 || msgs[1].(map[string]any)["role"] != "user" {
		t.Errorf("messages should be system + normalized: %v", msgs)
	}
	models, err := o.ListModels(context.Background())
	if err != nil || len(models) != 3 || models[0].ID != "gpt-4.1" {
		t.Errorf("models = %+v %v", models, err)
	}
	bad := NewOpenAI(srv.URL, "wrong", srv.Client())
	if err := bad.Ping(context.Background()); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("want ErrInvalidKey, got %v", err)
	}
	if _, err := NewOpenAI(srv.URL, "", nil).Chat(context.Background(), Request{}); !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("want ErrNoAPIKey, got %v", err)
	}
}

func TestAnthropic(t *testing.T) {
	var cap capture
	reply := `{"id":"msg_1","type":"message","role":"assistant","model":"%s","content":[{"type":"text","text":"hello there"}],"stop_reason":"%s","usage":{"input_tokens":10,"output_tokens":3}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "sk-ant-test" {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/messages":
			b := cap.add(r)
			if _, has := b["temperature"]; has && b["model"] == "claude-haiku-4-5-strict" {
				w.WriteHeader(400)
				fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"temperature: not supported for this model"}}`)
				return
			}
			stop := "end_turn"
			if strings.Contains(fmt.Sprint(b["messages"]), "refuse me") {
				stop = "refusal"
			}
			fmt.Fprintf(w, reply, b["model"], stop)
		case "/v1/models":
			fmt.Fprint(w, `{"data":[{"id":"claude-sonnet-5-5","display_name":"Claude Sonnet 5.5","type":"model","created_at":"2026-01-01T00:00:00Z","max_input_tokens":1000000,"max_tokens":128000}],"has_more":false,"first_id":"a","last_id":"a"}`)
		}
	}))
	defer srv.Close()
	a := newAnthropic("sk-ant-test", srv.Client(), option.WithBaseURL(srv.URL))

	resp, err := a.Chat(context.Background(), Request{
		Model: "claude-sonnet-5-5", System: "be Leo",
		Messages:  []Message{{RoleAssistant, "x"}, {RoleUser, "hi"}, {RoleUser, "there"}},
		MaxTokens: 5, Temperature: 0.8,
	})
	if err != nil || resp.Text != "hello there" || resp.InputTokens != 10 {
		t.Fatalf("chat: %+v %v", resp, err)
	}
	b := cap.bodies[0]
	if _, has := b["temperature"]; has {
		t.Error("sonnet 5.5 must not receive temperature")
	}
	if b["output_config"].(map[string]any)["effort"] != "low" {
		t.Errorf("effort = %v", b["output_config"])
	}
	if b["max_tokens"] != 1024.0 {
		t.Errorf("max_tokens floor for thinking models: %v", b["max_tokens"])
	}
	if msgs := b["messages"].([]any); len(msgs) != 1 {
		t.Errorf("messages should be normalized to one user turn: %v", msgs)
	}

	if _, err := a.Chat(context.Background(), Request{Model: "claude-haiku-4-5", Messages: []Message{{RoleUser, "hi"}}, MaxTokens: 5, Temperature: 0}); err != nil {
		t.Fatal(err)
	}
	b = cap.bodies[1]
	if b["temperature"] != 0.0 || b["output_config"] != nil || b["max_tokens"] != 5.0 {
		t.Errorf("haiku body: temperature=%v output_config=%v max_tokens=%v", b["temperature"], b["output_config"], b["max_tokens"])
	}

	n := len(cap.bodies)
	if _, err := a.Chat(context.Background(), Request{Model: "claude-haiku-4-5-strict", Messages: []Message{{RoleUser, "hi"}}, Temperature: 0.5}); err != nil {
		t.Fatalf("temperature retry: %v", err)
	}
	if len(cap.bodies) != n+2 || cap.bodies[n+1]["temperature"] != nil {
		t.Errorf("expected one retry without temperature")
	}
	if _, err := a.Chat(context.Background(), Request{Messages: []Message{{RoleUser, "x"}}, JSON: true, Schema: json.RawMessage(`{"type":"object"}`)}); err != nil {
		t.Fatal(err)
	}
	oc := cap.bodies[len(cap.bodies)-1]["output_config"].(map[string]any)
	if f := oc["format"].(map[string]any); f["type"] != "json_schema" || oc["effort"] != "medium" {
		t.Errorf("output_config = %v", oc)
	}

	if _, err := a.Chat(context.Background(), Request{Messages: []Message{{RoleUser, "refuse me"}}}); !errors.Is(err, ErrRefused) {
		t.Errorf("want ErrRefused, got %v", err)
	}
	models, err := a.ListModels(context.Background())
	if err != nil || len(models) != 1 || models[0].Label != "Claude Sonnet 5.5" {
		t.Errorf("models = %+v %v", models, err)
	}

	bad := newAnthropic("nope", srv.Client(), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
	if _, err := bad.Chat(context.Background(), Request{Messages: []Message{{RoleUser, "hi"}}}); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("want ErrInvalidKey, got %v", err)
	}
	if m, err := NewAnthropic("", nil).ListModels(context.Background()); err != nil || len(m) != 3 || m[0].ID != DefaultAnthropicModel {
		t.Errorf("static list without key: %+v %v", m, err)
	}
	if _, err := NewAnthropic("", nil).Chat(context.Background(), Request{}); !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("want ErrNoAPIKey, got %v", err)
	}
}

func TestAnthropicModelCapabilities(t *testing.T) {
	cases := []struct {
		m              string
		temp, effortOK bool
	}{
		{"claude-sonnet-5-5", false, true},
		{"claude-opus-5-5", false, true},
		{"claude-haiku-4-5", true, false},
		{"claude-sonnet-4-5-20250929", true, false},
		{"claude-opus-4-6", true, true},
		{"claude-opus-4-8", false, true},
	}
	for _, c := range cases {
		if anthropicSamplingAllowed(c.m) != c.temp || anthropicEffortSupported(c.m) != c.effortOK {
			t.Errorf("%s: temp=%v effort=%v", c.m, anthropicSamplingAllowed(c.m), anthropicEffortSupported(c.m))
		}
	}
}

func newTestRegistry(t *testing.T) (*Registry, *config.Manager, *events.Hub) {
	t.Helper()
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	hub := events.NewHub()
	return NewRegistry(cfg, hub), cfg, hub
}

func TestRegistryResolve(t *testing.T) {
	r, cfg, _ := newTestRegistry(t)
	p, m, err := r.Resolve(nil)
	if err != nil || p.Name() != "ollama" || m != "llama3.1:8b" {
		t.Errorf("default: %v %q %v", p, m, err)
	}
	p, m, _ = r.Resolve(&model.LLMChoice{Provider: "anthropic"})
	if p.Name() != "anthropic" || m != "claude-sonnet-5-5" {
		t.Errorf("anthropic default model: %q", m)
	}
	p, m, _ = r.Resolve(&model.LLMChoice{Provider: "anthropic", Model: "claude-opus-5-5"})
	if m != "claude-opus-5-5" {
		t.Errorf("explicit model: %q", m)
	}
	if _, _, err := r.Resolve(&model.LLMChoice{Provider: "bogus"}); err == nil {
		t.Error("unknown provider should fail")
	}

	// Settings changes rebuild providers (new key → new client).
	before, _ := r.Get("anthropic")
	cfg.UpdateSecrets(func(s *config.Secrets) { s.AnthropicKey = "sk-ant-x" })
	after, _ := r.Get("anthropic")
	if before == after {
		t.Error("registry should rebuild on secrets change")
	}
	cfg.Update(func(s *config.Settings) { s.Theme = "dark" })
	if again, _ := r.Get("anthropic"); again != after {
		t.Error("unrelated settings should not rebuild providers")
	}

	f := NewFake()
	r.UseFake(f)
	for _, name := range []string{"ollama", "anthropic", "openai", ""} {
		if p, _ := r.Get(name); p != f {
			t.Errorf("UseFake: %q did not resolve to the fake", name)
		}
	}
}

func TestRegistryContractWithFake(t *testing.T) {
	r, _, hub := newTestRegistry(t)
	f := NewFake()
	r.UseFake(f)
	res := r.Test(context.Background(), "ollama", "")
	if !res.OK || res.Sample == "" || res.Model != "llama3.1:8b" {
		t.Errorf("Test = %+v", res)
	}
	if last := f.Requests()[0]; last.Messages[0].Content != "Say hi in 3 words" {
		t.Errorf("test prompt = %+v", last)
	}
	f.PushError(errors.New("boom"))
	if res := r.Test(context.Background(), "openai", "gpt-x"); res.OK || res.Error != "boom" || res.Model != "gpt-x" {
		t.Errorf("failing Test = %+v", res)
	}
	st := r.OllamaStatus(context.Background())
	if !st.Reachable || len(st.Models) != 1 {
		t.Errorf("status = %+v", st)
	}

	ch, _, cancel := hub.Subscribe(0)
	defer cancel()
	id, err := r.OllamaPull("llama3.1:8b")
	if err != nil || id == "" {
		t.Fatal(err)
	}
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev := <-ch:
			var pp model.PullProgress
			if ev.Type != events.TypeOllamaPull || json.Unmarshal(ev.Data, &pp) != nil || pp.JobID != id {
				continue
			}
			if pp.Done {
				if pp.Error != "" || pp.Percent != 100 {
					t.Errorf("final progress = %+v", pp)
				}
				return
			}
		case <-deadline:
			t.Fatal("pull never finished")
		}
	}
}

func TestRegistryOllamaUnreachable(t *testing.T) {
	r, cfg, _ := newTestRegistry(t)
	cfg.Update(func(s *config.Settings) { s.LLM.OllamaURL = "http://127.0.0.1:1" })
	st := r.OllamaStatus(context.Background())
	if st.Reachable || st.Error == "" || st.Models == nil {
		t.Errorf("status = %+v", st)
	}
}

func TestFakeDefaults(t *testing.T) {
	f := NewFake()
	r, _ := f.Chat(context.Background(), Request{Messages: []Message{{RoleUser, "Would Leo respond? Reply with only YES or NO."}}})
	if r.Text != "YES" {
		t.Errorf("decision = %q", r.Text)
	}
	r, _ = f.Chat(context.Background(), Request{JSON: true})
	if !json.Valid([]byte(r.Text)) {
		t.Errorf("JSON mode should return JSON: %q", r.Text)
	}
	f.Push("scripted")
	if r, _ = f.Chat(context.Background(), Request{}); r.Text != "scripted" {
		t.Errorf("queue = %q", r.Text)
	}
	f.SetDelay(time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := f.Chat(ctx, Request{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("delay should honour ctx: %v", err)
	}
	if len(f.Requests()) != 4 {
		t.Errorf("requests recorded = %d", len(f.Requests()))
	}
}
