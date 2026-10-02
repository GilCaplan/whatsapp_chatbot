package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
)

// settingsView is Settings plus the masked secrets (GET/PUT /api/settings).
type settingsView struct {
	config.Settings
	Secrets map[string]config.SecretView `json:"secrets"`
}

func (s *Server) settingsView() settingsView {
	return settingsView{Settings: s.d.Config.Get(), Secrets: s.d.Config.Secrets().View()}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if s.d.Config == nil {
		unavailable(w, "Settings")
		return
	}
	writeJSON(w, http.StatusOK, s.settingsView())
}

// mergeJSON deep-merges patch into dst: objects merge key by key, everything
// else (scalars, arrays, null) replaces.
func mergeJSON(dst, patch map[string]any) {
	for k, pv := range patch {
		if pm, ok := pv.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				mergeJSON(dm, pm)
				continue
			}
		}
		dst[k] = pv
	}
}

// MergeSettings applies a partial JSON object onto cur and returns the result.
// The port and version are never changed here (use POST /api/system/port).
func MergeSettings(cur config.Settings, patch []byte) (config.Settings, error) {
	var p map[string]any
	if err := json.Unmarshal(patch, &p); err != nil {
		return cur, fmt.Errorf("invalid JSON: %w", err)
	}
	if p == nil {
		return cur, nil
	}
	delete(p, "secrets") // GET returns it; tolerate it being echoed back
	// v1 blocks: migrated on load, never accepted again.
	delete(p, "replies")
	delete(p, "approvals")
	b, err := json.Marshal(cur)
	if err != nil {
		return cur, err
	}
	var base map[string]any
	if err := json.Unmarshal(b, &base); err != nil {
		return cur, err
	}
	mergeJSON(base, p)
	merged, err := json.Marshal(base)
	if err != nil {
		return cur, err
	}
	// Decode into a fresh value: decoding into cur would reuse (and race on)
	// the backing arrays of its slices.
	var next config.Settings
	if err := json.Unmarshal(merged, &next); err != nil {
		return cur, fmt.Errorf("invalid settings: %w", err)
	}
	next.Port = cur.Port
	next.Version = cur.Version

	// "defaultModel is derived from the per-provider model": if the client only
	// sent defaultModel, store it as that provider's model.
	if llm, ok := p["llm"].(map[string]any); ok {
		if dm, ok := llm["defaultModel"].(string); ok && dm != "" {
			key := map[string]string{"ollama": "ollamaModel", "anthropic": "anthropicModel", "openai": "openaiModel"}[next.LLM.DefaultProvider]
			if _, explicit := llm[key]; key != "" && !explicit {
				switch next.LLM.DefaultProvider {
				case "ollama":
					next.LLM.OllamaModel = dm
				case "anthropic":
					next.LLM.AnthropicModel = dm
				case "openai":
					next.LLM.OpenAIModel = dm
				}
			}
		}
	}
	return next, validateSettings(next)
}

func validateSettings(s config.Settings) error {
	oneOf := func(field, v string, allowed ...string) error {
		for _, a := range allowed {
			if v == a {
				return nil
			}
		}
		return fmt.Errorf("%s must be one of %s", field, strings.Join(allowed, ", "))
	}
	inRange := func(field string, v, lo, hi int) error {
		if v < lo || v > hi {
			return fmt.Errorf("%s must be between %d and %d", field, lo, hi)
		}
		return nil
	}
	checks := []error{
		oneOf("theme", s.Theme, "system", "light", "dark"),
		oneOf("llm.defaultProvider", s.LLM.DefaultProvider, "ollama", "anthropic", "openai"),
		inRange("llm.replyMaxTokens", s.LLM.ReplyMaxTokens, 0, 32000),
		inRange("llm.ollamaNumCtx", s.LLM.OllamaNumCtx, 0, 1_048_576),
		prefixed("behavior.private.", behavior.Validate(s.Behavior.Private)),
		prefixed("behavior.group.", behavior.Validate(s.Behavior.Group)),
	}
	if utf8.RuneCountInString(s.Behavior.TriggerPrefix) > 8 || strings.ContainsAny(s.Behavior.TriggerPrefix, " \t\n") {
		checks = append(checks, fmt.Errorf("behavior.triggerPrefix must be at most 8 characters without spaces"))
	}
	if s.LLM.Temperature < 0 || s.LLM.Temperature > 2 {
		checks = append(checks, fmt.Errorf("llm.temperature must be between 0 and 2"))
	}
	// v4 blocks.
	if !validHHMM(s.Recap.Time) {
		checks = append(checks, fmt.Errorf("recap.time must be a time like 21:00"))
	}
	checks = append(checks,
		inRange("recap.keepDays", s.Recap.KeepDays, 1, 365),
		inRange("clone.maxSamples", s.SelfClone.MaxSamples, 50, 5000),
	)
	// v5: cross-chat context.
	cr := s.Memory.Cross
	checks = append(checks,
		oneOf("memory.cross.groupMode", cr.GroupMode, model.CrossModes...),
		oneOf("memory.cross.dmMode", cr.DMMode, model.CrossModes...),
		inRange("memory.cross.freshDays", cr.FreshDays, 1, 90),
		inRange("memory.cross.maxPeople", cr.MaxPeople, 1, 8),
		inRange("memory.cross.maxItems", cr.MaxItems, 1, 16),
	)
	if utf8.RuneCountInString(s.Safety.Reveal.Template) > config.MaxRevealTemplate {
		checks = append(checks, fmt.Errorf("safety.reveal.template must be at most %d characters", config.MaxRevealTemplate))
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	return nil
}

// validHHMM reports whether v is "HH:MM" (00:00–23:59).
func validHHMM(v string) bool {
	_, err := time.Parse("15:04", v)
	return err == nil && len(v) == 5
}

func prefixed(prefix string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s%w", prefix, err)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	if s.d.Config == nil {
		unavailable(w, "Settings")
		return
	}
	var raw json.RawMessage
	if !decodeJSON(w, r, &raw) {
		return
	}
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	prev := s.d.Config.Get()
	var verr error
	next, err := s.d.Config.Update(func(st *config.Settings) {
		merged, err := MergeSettings(*st, raw)
		if err != nil {
			verr = err
			return
		}
		*st = merged
	})
	if verr != nil {
		writeError(w, http.StatusBadRequest, "invalid_settings", verr.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", "Could not save settings: "+err.Error())
		return
	}
	if prev.LLM != next.LLM && s.d.LLM != nil {
		s.d.LLM.Rebuild()
	}
	if (prev.LLM != next.LLM || !reflect.DeepEqual(prev.Behavior, next.Behavior)) && s.d.Engine != nil {
		s.d.Engine.Reload()
	}
	if s.d.Hub != nil {
		s.d.Hub.Publish(events.TypeSettingsChanged, next)
	}
	writeJSON(w, http.StatusOK, s.settingsView())
}

func (s *Server) handlePutSecrets(w http.ResponseWriter, r *http.Request) {
	if s.d.Config == nil {
		unavailable(w, "Settings")
		return
	}
	var body struct {
		AnthropicKey *string `json:"anthropicKey"`
		OpenAIKey    *string `json:"openaiKey"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	sec, err := s.d.Config.UpdateSecrets(func(sc *config.Secrets) {
		if body.AnthropicKey != nil {
			sc.AnthropicKey = strings.TrimSpace(*body.AnthropicKey)
		}
		if body.OpenAIKey != nil {
			sc.OpenAIKey = strings.TrimSpace(*body.OpenAIKey)
		}
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", "Could not save keys: "+err.Error())
		return
	}
	if s.d.LLM != nil && (body.AnthropicKey != nil || body.OpenAIKey != nil) {
		s.d.LLM.Rebuild()
	}
	writeJSON(w, http.StatusOK, sec.View())
}

// ─── LLM ─────────────────────────────────────────────────────

func validProvider(p string) bool {
	return p == "ollama" || p == "anthropic" || p == "openai"
}

func (s *Server) handleLLMTest(w http.ResponseWriter, r *http.Request) {
	if s.d.LLM == nil {
		unavailable(w, "LLM")
		return
	}
	var body struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Provider == "" && s.d.Config != nil {
		body.Provider = s.d.Config.Get().LLM.DefaultProvider
	}
	if !validProvider(body.Provider) {
		writeError(w, http.StatusBadRequest, "invalid_provider", "provider must be ollama, anthropic or openai")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.d.LLM.Test(ctx, body.Provider, body.Model))
}

func (s *Server) handleLLMModels(w http.ResponseWriter, r *http.Request) {
	if s.d.LLM == nil {
		unavailable(w, "LLM")
		return
	}
	provider := r.URL.Query().Get("provider")
	if provider == "" && s.d.Config != nil {
		provider = s.d.Config.Get().LLM.DefaultProvider
	}
	if !validProvider(provider) {
		writeError(w, http.StatusBadRequest, "invalid_provider", "provider must be ollama, anthropic or openai")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	models, err := s.d.LLM.ListModels(ctx, provider)
	if err != nil {
		writeError(w, http.StatusBadGateway, "llm_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": nonNilSlice(models)})
}

func (s *Server) handleOllamaStatus(w http.ResponseWriter, r *http.Request) {
	if s.d.LLM == nil {
		unavailable(w, "LLM")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	st := s.d.LLM.OllamaStatus(ctx)
	st.Models = nonNilSlice(st.Models)
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleOllamaPull(w http.ResponseWriter, r *http.Request) {
	if s.d.LLM == nil {
		unavailable(w, "LLM")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "Model name is required")
		return
	}
	id, err := s.d.LLM.OllamaPull(body.Name)
	if err != nil {
		writeError(w, http.StatusBadGateway, "pull_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"jobId": id})
}

func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
