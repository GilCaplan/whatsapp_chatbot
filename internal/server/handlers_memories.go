package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/crossctx"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/memory"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

// Memory of people (wave 3, owner: Engineer A). What a persona learned about
// the people in a chat, plus what you add yourself; every change publishes
// memories.changed. See WAVE3_PLAN §3.5 and docs/API.md "Memory".

// memoriesChanged tells open pages a chat's memories changed.
func (s *Server) memoriesChanged(chatKey string) {
	if s.d.Hub != nil {
		s.d.Hub.Publish(events.TypeMemoriesChanged, map[string]string{"chatKey": chatKey})
	}
}

// cleanMemoryText validates one line of memory text.
func cleanMemoryText(w http.ResponseWriter, text string) (string, bool) {
	if utf8.RuneCountInString(strings.Join(strings.Fields(text), " ")) > model.MaxMemoryText {
		text = "" // too long: rejected, not silently cut
	}
	text = memory.CleanText(text)
	if text == "" {
		writeError(w, http.StatusBadRequest, "invalid_memory", "Write one line of at most 160 characters")
		return "", false
	}
	return text, true
}

// memoryEnabled is the chat's effective memory switch.
func (s *Server) memoryEnabled(c model.ChatAssignment) bool {
	if c.Memory != nil {
		return *c.Memory
	}
	return s.d.Config == nil || s.d.Config.Get().Memory.Enabled
}

func (s *Server) handleListMemories(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	items, err := s.d.Store.Memories(c.Key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load_failed", err.Error())
		return
	}
	st := s.d.Store.RunnerState(c.Key)
	rules := s.crossRules()
	cross := s.chatShares(c, rules)
	now := time.Now()
	list := make([]model.MemoryItem, 0, len(items))
	for _, m := range items {
		ok, _ := crossctx.Carries(m, rules, now)
		list = append(list, model.MemoryItem{Memory: m, Shares: cross && ok, EffectiveSensitive: crossctx.EffectiveSensitive(m)})
	}
	v := model.MemoriesView{Enabled: s.memoryEnabled(c), CrossEnabled: cross, Items: list, Pending: st.MemoryPending}
	if !st.MemoryCursor.IsZero() {
		t := st.MemoryCursor
		v.LastExtractedAt = &t
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleAddMemory(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	var body struct {
		Text      string `json:"text"`
		Person    string `json:"person"`
		PersonJID string `json:"personJid"`
		Pinned    bool   `json:"pinned"`
		Scope     string `json:"scope"`
		Sensitive string `json:"sensitive"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	text, ok := cleanMemoryText(w, body.Text)
	if !ok {
		return
	}
	if !validScope(body.Scope) || !crossctx.ValidSensitive(body.Sensitive) {
		writeError(w, http.StatusBadRequest, "invalid_memory", "scope must be local or shared; sensitive must be a known topic")
		return
	}
	sens := body.Sensitive
	if sens == "" {
		sens = crossctx.Classify(text)
	}
	now := time.Now()
	m, err := s.d.Store.UpsertMemory(model.Memory{
		ChatKey: c.Key, Person: strings.TrimSpace(body.Person), PersonJID: strings.TrimSpace(body.PersonJID),
		Text: text, Kind: model.MemoryFact, Source: model.MemorySourceUser, Pinned: body.Pinned,
		Confidence: 100, CreatedAt: now, UpdatedAt: now, Scope: body.Scope, Sensitive: sens,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.memoriesChanged(c.Key)
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handlePatchMemory(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	var body struct {
		Text      *string         `json:"text"`
		Pinned    *bool           `json:"pinned"`
		Person    *string         `json:"person"`
		Scope     json.RawMessage `json:"scope"`     // "local"|"shared"|null (null = follow the settings)
		Sensitive *string         `json:"sensitive"` // ""|category
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	var scope *string
	if len(body.Scope) > 0 {
		var sc *string
		if err := json.Unmarshal(body.Scope, &sc); err != nil || (sc != nil && (*sc == "" || !validScope(*sc))) {
			writeError(w, http.StatusBadRequest, "invalid_memory", "scope must be \"local\", \"shared\" or null")
			return
		}
		if sc == nil {
			sc = new(string)
		}
		scope = sc
	}
	if body.Sensitive != nil && !crossctx.ValidSensitive(*body.Sensitive) {
		writeError(w, http.StatusBadRequest, "invalid_memory", "sensitive must be empty or one of "+strings.Join(model.SensitiveCategories, ", "))
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	var text string
	if body.Text != nil {
		if text, ok = cleanMemoryText(w, *body.Text); !ok {
			return
		}
	}
	id := r.PathValue("id")
	var out model.Memory
	_, err := s.d.Store.UpdateMemories(c.Key, func(mems []model.Memory) ([]model.Memory, error) {
		for i := range mems {
			if mems[i].ID != id {
				continue
			}
			m := &mems[i]
			if body.Text != nil && text != m.Text {
				// Your edit makes it yours: the extractor won't rewrite it.
				m.Text, m.Source, m.Confidence = text, model.MemorySourceUser, 100
			}
			if body.Pinned != nil {
				m.Pinned = *body.Pinned
			}
			if body.Person != nil {
				m.Person = strings.TrimSpace(*body.Person)
			}
			if scope != nil {
				m.Scope = *scope
			}
			if body.Sensitive != nil {
				m.Sensitive = *body.Sensitive
			}
			m.UpdatedAt = time.Now()
			out = *m
			return mems, nil
		}
		return nil, store.ErrNotFound
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "No such memory")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.memoriesChanged(c.Key)
	writeJSON(w, http.StatusOK, out)
}

// validScope: "" (follow the settings), "local" or "shared".
func validScope(sc string) bool {
	return sc == "" || sc == model.MemoryScopeLocal || sc == model.MemoryScopeShared
}

func (s *Server) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	err := s.d.Store.DeleteMemory(c.Key, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "No such memory")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.memoriesChanged(c.Key)
	writeOK(w)
}

func (s *Server) handleClearMemories(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	if err := s.d.Store.ClearMemories(c.Key); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.memoriesChanged(c.Key)
	writeOK(w)
}

func (s *Server) handleExtractMemories(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	res, err := s.d.Engine.ExtractMemories(ctx, c.Key)
	if err != nil {
		writeError(w, http.StatusBadGateway, "extract_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}
