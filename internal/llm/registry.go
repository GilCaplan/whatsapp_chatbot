package llm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/contract"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
)

// Providers lists the selectable provider names.
var Providers = []string{"ollama", "anthropic", "openai"}

var _ contract.LLM = (*Registry)(nil)

// Registry owns the provider clients, rebuilt whenever settings or secrets change.
type Registry struct {
	cfg *config.Manager
	hub *events.Hub
	hc  *http.Client

	mu            sync.RWMutex
	providers     map[string]Provider
	fake          Provider
	builtFrom     string // fingerprint of the settings the providers were built from
	openaiDefault string // cached pick from /models when settings leave it empty
}

func NewRegistry(cfg *config.Manager, hub *events.Hub) *Registry {
	r := &Registry{
		cfg: cfg,
		hub: hub,
		hc:  &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()},
	}
	r.Rebuild()
	cfg.OnChange(r.Rebuild)
	return r
}

// UseFake makes every provider name resolve to p (tests, --fake-llm).
func (r *Registry) UseFake(p Provider) {
	r.mu.Lock()
	r.fake = p
	r.mu.Unlock()
}

// Rebuild recreates the provider clients if their settings or keys changed.
func (r *Registry) Rebuild() {
	s := r.cfg.Get().LLM
	sec := r.cfg.Secrets()
	fp := fmt.Sprintf("%s|%d|%s|%s|%s", s.OllamaURL, s.OllamaNumCtx, s.OpenAIBaseURL, sec.AnthropicKey, sec.OpenAIKey)
	r.mu.Lock()
	defer r.mu.Unlock()
	if fp == r.builtFrom && r.providers != nil {
		return
	}
	r.builtFrom = fp
	r.openaiDefault = ""
	r.providers = map[string]Provider{
		"ollama":    NewOllama(s.OllamaURL, s.OllamaNumCtx, r.hc),
		"anthropic": NewAnthropic(sec.AnthropicKey, r.hc),
		"openai":    NewOpenAI(s.OpenAIBaseURL, sec.OpenAIKey, r.hc),
	}
}

// Get returns the provider by name ("" = settings default).
func (r *Registry) Get(name string) (Provider, error) {
	if name == "" {
		name = r.cfg.Get().LLM.DefaultProvider
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.fake != nil {
		return r.fake, nil
	}
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown LLM provider %q", name)
	}
	return p, nil
}

// Resolve picks the provider and model for a persona's (or request's) choice,
// falling back to the settings default provider and its configured model.
func (r *Registry) Resolve(choice *model.LLMChoice) (Provider, string, error) {
	s := r.cfg.Get().LLM
	name := s.DefaultProvider
	if choice != nil && choice.Provider != "" {
		name = choice.Provider
	}
	p, err := r.Get(name)
	if err != nil {
		return nil, "", err
	}
	mdl := ""
	if choice != nil && choice.Model != "" && (choice.Provider == "" || choice.Provider == name) {
		mdl = choice.Model
	}
	if mdl == "" {
		mdl = s.ModelFor(name)
	}
	if mdl == "" {
		mdl = r.defaultModel(name)
	}
	return p, mdl, nil
}

func (r *Registry) defaultModel(provider string) string {
	switch provider {
	case "ollama":
		return DefaultOllamaModel
	case "anthropic":
		return DefaultAnthropicModel
	case "openai":
		return r.pickOpenAIDefault()
	}
	return ""
}

// pickOpenAIDefault picks the first preferred model the key can see (cached).
func (r *Registry) pickOpenAIDefault() string {
	r.mu.RLock()
	cached, fake := r.openaiDefault, r.fake
	p := r.providers["openai"]
	r.mu.RUnlock()
	if cached != "" {
		return cached
	}
	if fake != nil || p == nil {
		return FallbackOpenAIModel
	}
	ctx, cancel := context.WithTimeout(context.Background(), TimeoutPing)
	defer cancel()
	models, err := p.ListModels(ctx)
	if err != nil || len(models) == 0 {
		return FallbackOpenAIModel
	}
	pick := models[0].ID
	for _, want := range OpenAIPreferred {
		if slices.ContainsFunc(models, func(m model.ModelInfo) bool { return m.ID == want }) {
			pick = want
			break
		}
	}
	r.mu.Lock()
	r.openaiDefault = pick
	r.mu.Unlock()
	return pick
}

// ─── contract.LLM ────────────────────────────────────────────

func (r *Registry) ListModels(ctx context.Context, provider string) ([]model.ModelInfo, error) {
	p, err := r.Get(provider)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return p.ListModels(ctx)
}

// Test sends a tiny prompt and reports latency and the sample reply.
func (r *Registry) Test(ctx context.Context, provider, mdl string) model.LLMTestResult {
	res := model.LLMTestResult{Provider: provider}
	if provider == "" {
		res.Provider = r.cfg.Get().LLM.DefaultProvider
	}
	p, resolved, err := r.Resolve(&model.LLMChoice{Provider: res.Provider, Model: mdl})
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Model = resolved
	ctx, cancel := context.WithTimeout(ctx, TimeoutReply)
	defer cancel()
	start := time.Now()
	resp, err := p.Chat(ctx, Request{
		Model:       resolved,
		Messages:    []Message{{Role: RoleUser, Content: "Say hi in 3 words"}},
		MaxTokens:   30,
		Temperature: r.cfg.Get().LLM.Temperature,
	})
	res.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.OK = true
	res.Sample = resp.Text
	if resp.Model != "" {
		res.Model = resp.Model
	}
	return res
}

func (r *Registry) OllamaStatus(ctx context.Context) model.OllamaStatus {
	p, err := r.Get("ollama")
	if err != nil {
		return model.OllamaStatus{Error: err.Error(), Models: []model.ModelInfo{}}
	}
	ctx, cancel := context.WithTimeout(ctx, TimeoutPing)
	defer cancel()
	st := model.OllamaStatus{Models: []model.ModelInfo{}}
	if o, ok := p.(*Ollama); ok {
		v, err := o.Version(ctx)
		if err != nil {
			st.Error = "Ollama is not reachable at " + o.base + " — is it running?"
			return st
		}
		st.Version = v
	} else {
		if err := p.Ping(ctx); err != nil {
			st.Error = err.Error()
			return st
		}
		st.Version = p.Name()
	}
	st.Reachable = true
	models, err := p.ListModels(ctx)
	if err != nil {
		st.Error = err.Error()
	} else {
		st.Models = models
	}
	return st
}

// OllamaPull starts a background model download and streams progress over SSE.
func (r *Registry) OllamaPull(name string) (string, error) {
	if name == "" {
		return "", errors.New("model name required")
	}
	p, err := r.Get("ollama")
	if err != nil {
		return "", err
	}
	jobID := newJobID()
	go r.runPull(p, jobID, name)
	return jobID, nil
}

func (r *Registry) runPull(p Provider, jobID, name string) {
	publish := func(pp model.PullProgress) {
		pp.JobID, pp.Name = jobID, name
		if pp.Total > 0 {
			pp.Percent = float64(pp.Completed) * 100 / float64(pp.Total)
		}
		if pp.Done && pp.Error == "" {
			pp.Percent = 100
		}
		r.hub.Publish(events.TypeOllamaPull, pp)
	}
	publish(model.PullProgress{Status: "starting"})
	o, ok := p.(*Ollama)
	if !ok { // fake provider: pretend to download
		for _, c := range []int64{25, 50, 75, 100} {
			time.Sleep(50 * time.Millisecond)
			publish(model.PullProgress{Status: "downloading", Completed: c, Total: 100})
		}
		publish(model.PullProgress{Status: "success", Done: true})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	var last time.Time
	var lastStatus string
	err := o.Pull(ctx, name, func(u PullUpdate) {
		// Throttle byte-progress updates; always forward status changes.
		if u.Status == lastStatus && time.Since(last) < 300*time.Millisecond {
			return
		}
		last, lastStatus = time.Now(), u.Status
		publish(model.PullProgress{Status: u.Status, Completed: u.Completed, Total: u.Total})
	})
	if err != nil {
		publish(model.PullProgress{Status: "error", Done: true, Error: err.Error()})
		return
	}
	publish(model.PullProgress{Status: "success", Done: true})
}

func newJobID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
