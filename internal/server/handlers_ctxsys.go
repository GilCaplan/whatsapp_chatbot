package server

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/ctxsys"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

// Context systems (ctxsys): the three built-in ones (locked) and yours, the
// building blocks of their charts, which chat uses which, drafting one from
// a description and the replay sandbox.

func (s *Server) contextSystemsChanged() {
	if s.d.Hub != nil {
		s.d.Hub.Publish(events.TypeContextChanged, struct{}{})
	}
}

// systemView is a system with its estimate and how many use it.
type systemView struct {
	model.ContextSystem
	Estimate ctxsys.Estimate `json:"estimate"`
	Chats    int             `json:"chats"`    // chats running it now
	Personas int             `json:"personas"` // personas set to it
}

// GET /api/context/catalog — the building blocks, stages and knobs.
func (s *Server) handleContextCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Blocks []model.ChartBlock `json:"blocks"`
		Stages []string           `json:"stages"`
		Params []ctxsys.ParamSpec `json:"params"`
		Bases  []string           `json:"bases"`
		NodeW  int                `json:"nodeW"`
		NodeH  int                `json:"nodeH"`
	}{ctxsys.Blocks(), ctxsys.Stages, ctxsys.Specs(), ctxsys.Bases, ctxsys.NodeW, ctxsys.NodeH})
}

// GET /api/context/systems
func (s *Server) handleContextSystems(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	insts := s.instances()
	items := []systemView{}
	for _, cs := range s.d.Store.ContextSystems() {
		v := systemView{ContextSystem: cs, Estimate: ctxsys.Estimates(ctxsys.Clamp(cs.Params, cs.Base))}
		for _, in := range insts {
			if in.Choice.SystemID == cs.ID {
				v.Chats++
			}
		}
		for _, p := range s.d.Store.Personas() {
			if p.ContextSystem == cs.ID {
				v.Personas++
			}
		}
		items = append(items, v)
	}
	app := s.d.Config.Get().Context.System
	if _, ok := s.d.Store.ContextSystem(app); !ok {
		app = ctxsys.Default
	}
	writeJSON(w, http.StatusOK, struct {
		Items      []systemView `json:"items"`
		AppDefault string       `json:"appDefault"`
	}{items, app})
}

// POST /api/context/systems — {copyFrom} copies a system; otherwise the
// body is the new system.
func (s *Server) handleCreateContextSystem(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	var body struct {
		model.ContextSystem
		CopyFrom string `json:"copyFrom"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	cs := body.ContextSystem
	if body.CopyFrom != "" {
		src, ok := s.d.Store.ContextSystem(body.CopyFrom)
		if !ok {
			writeError(w, http.StatusNotFound, "not_found", "No such context system")
			return
		}
		name := strings.TrimSpace(cs.Name)
		cs = src
		cs.Name = name
		if cs.Name == "" {
			cs.Name = copyName(src.Name)
		}
		if src.BuiltIn {
			cs.Base = src.ID
		}
	}
	cs.ID, cs.BuiltIn = "", false
	saved, err := s.d.Store.SaveContextSystem(cs)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_system", err.Error())
		return
	}
	s.contextSystemsChanged()
	writeJSON(w, http.StatusCreated, saved)
}

func copyName(name string) string {
	n := name + " (copy)"
	if utf8.RuneCountInString(n) > ctxsys.MaxName {
		n = string([]rune(n)[:ctxsys.MaxName])
	}
	return n
}

// PUT /api/context/systems/{id} — replace one of yours.
func (s *Server) handlePutContextSystem(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	id := r.PathValue("id")
	if ctxsys.IsBase(id) {
		writeError(w, http.StatusForbidden, "built_in", "The built-in systems can't be changed; copy one first")
		return
	}
	var cs model.ContextSystem
	if !decodeJSON(w, r, &cs) {
		return
	}
	cs.ID = id
	saved, err := s.d.Store.SaveContextSystem(cs)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "No such context system")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, "invalid_system", err.Error())
		return
	}
	s.contextSystemsChanged()
	s.reloadEngine()
	writeJSON(w, http.StatusOK, saved)
}

// DELETE /api/context/systems/{id}
func (s *Server) handleDeleteContextSystem(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	id := r.PathValue("id")
	if ctxsys.IsBase(id) {
		writeError(w, http.StatusForbidden, "built_in", "The built-in systems can't be deleted")
		return
	}
	err := s.d.Store.DeleteContextSystem(id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "No such context system")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	if s.d.Config.Get().Context.System == id { // the app default goes back to Classic
		if next, err := s.d.Config.Update(func(st *config.Settings) { st.Context.System = "" }); err == nil && s.d.Hub != nil {
			s.d.Hub.Publish(events.TypeSettingsChanged, next)
		}
	}
	s.contextSystemsChanged()
	s.chatsChanged()
	s.personasChanged()
	s.reloadEngine()
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/context/infer — {chart, params, base}: the knobs a chart like
// this stands for, and what that changes.
func (s *Server) handleInferContext(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Chart  model.ContextChart  `json:"chart"`
		Params model.ContextParams `json:"params"`
		Base   string              `json:"base"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := ctxsys.NormalizeChart(&body.Chart); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_chart", err.Error())
		return
	}
	p, changes := ctxsys.Infer(body.Chart, ctxsys.Clamp(body.Params, body.Base))
	writeJSON(w, http.StatusOK, struct {
		Params  model.ContextParams `json:"params"`
		Changes []string            `json:"changes"`
	}{p, nonNilSlice(changes)})
}

// POST /api/context/layout — {chart}: the chart tidied up (auto layout).
func (s *Server) handleLayoutContext(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Chart model.ContextChart `json:"chart"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := ctxsys.NormalizeChart(&body.Chart); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_chart", err.Error())
		return
	}
	ctxsys.Layout(&body.Chart)
	writeJSON(w, http.StatusOK, body.Chart)
}

// POST /api/context/draft — {text, base}: an unsaved system drafted by the AI.
func (s *Server) handleDraftContext(w http.ResponseWriter, r *http.Request) {
	if s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	var body struct {
		Text string `json:"text"`
		Base string `json:"base"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Text = strings.TrimSpace(body.Text)
	if body.Text == "" || utf8.RuneCountInString(body.Text) > 2000 {
		writeError(w, http.StatusBadRequest, "invalid_text", "Describe the system in 1 to 2000 characters")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	cs, changes, err := s.d.Engine.DraftSystem(ctx, body.Text, body.Base)
	if err != nil {
		writeError(w, http.StatusBadGateway, "draft_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, struct {
		System  model.ContextSystem `json:"system"`
		Changes []string            `json:"changes"`
	}{cs, nonNilSlice(changes)})
}

// instanceView is one chat + persona and the system it runs.
type instanceView struct {
	ChatKey       string              `json:"chatKey"`
	Name          string              `json:"name"`
	Kind          string              `json:"kind"`
	Enabled       bool                `json:"enabled"`
	PersonaID     string              `json:"personaId"`
	PersonaName   string              `json:"personaName"`
	ChatSystem    string              `json:"chatSystem"`    // "" = inherit
	PersonaSystem string              `json:"personaSystem"` // "" = the app's
	Choice        model.ContextChoice `json:"choice"`
	SystemName    string              `json:"systemName"`
}

func (s *Server) instances() []instanceView {
	app := s.d.Config.Get().Context.System
	out := []instanceView{}
	for _, c := range s.d.Store.Chats() {
		p, _ := s.d.Store.Persona(c.PersonaID)
		sys, ch := ctxsys.Resolve(c.ContextSystem, p.ContextSystem, app, s.d.Store.ContextSystem)
		out = append(out, instanceView{ChatKey: c.Key, Name: c.Name, Kind: c.Kind, Enabled: c.Enabled, PersonaID: c.PersonaID, PersonaName: p.Name,
			ChatSystem: c.ContextSystem, PersonaSystem: p.ContextSystem, Choice: ch, SystemName: sys.Name})
	}
	return out
}

// GET /api/context/instances
func (s *Server) handleContextInstances(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []instanceView `json:"items"`
	}{s.instances()})
}

// PUT /api/personas/{id}/context — {systemId}: the persona's context system
// ("" = the app's).
func (s *Server) handlePersonaContext(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requirePersona(w, r.PathValue("id"))
	if !ok {
		return
	}
	var body struct {
		SystemID string `json:"systemId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.SystemID != "" {
		if _, ok := s.d.Store.ContextSystem(body.SystemID); !ok {
			writeError(w, http.StatusBadRequest, "unknown_system", "Unknown context system")
			return
		}
	}
	p.ContextSystem = body.SystemID
	saved, err := s.d.Store.UpsertPersona(p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.personasChanged()
	s.contextSystemsChanged()
	s.reloadEngine()
	writeJSON(w, http.StatusOK, saved)
}

// POST /api/personas/{id}/card — {chars?}: write the card now.
// PUT  /api/personas/{id}/card — {card}: your own text ("" = make it again when needed).
func (s *Server) handleMakeCard(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requirePersona(w, r.PathValue("id"))
	if !ok {
		return
	}
	if s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	var body struct {
		Chars int `json:"chars"`
	}
	if r.ContentLength > 0 && !decodeJSON(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	saved, err := s.d.Engine.MakeCard(ctx, p.ID, body.Chars)
	if err != nil {
		writeError(w, http.StatusBadGateway, "card_failed", err.Error())
		return
	}
	s.personasChanged()
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handlePutCard(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requirePersona(w, r.PathValue("id"))
	if !ok {
		return
	}
	var body struct {
		Card string `json:"card"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	card := strings.TrimSpace(body.Card)
	if utf8.RuneCountInString(card) > 2000 {
		writeError(w, http.StatusBadRequest, "invalid_card", "A card can be at most 2000 characters")
		return
	}
	saved, err := s.d.Store.SetPersonaCard(p.ID, card)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.personasChanged()
	writeJSON(w, http.StatusOK, saved)
}

// characterChanged: an edit that changes what the card is written from.
func characterChanged(a, b model.Persona) bool {
	return a.Name != b.Name || a.Tagline != b.Tagline || a.Bio != b.Bio || a.Personality != b.Personality || a.Style != b.Style ||
		a.Vocabulary != b.Vocabulary || a.Language != b.Language || a.Rules != b.Rules || a.MessageLength != b.MessageLength ||
		a.AdvancedPrompt != b.AdvancedPrompt || a.Emoji.Usage != b.Emoji.Usage || !slices.Equal(a.Emoji.Favorites, b.Emoji.Favorites)
}

// ─── Sandbox ─────────────────────────────────────────────────

// POST /api/context/replay — {chatKey, systems, messages}
func (s *Server) handleStartReplay(w http.ResponseWriter, r *http.Request) {
	if s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	var req model.ReplayRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if _, ok := s.requireChat(w, req.ChatKey); !ok {
		return
	}
	run, err := s.d.Engine.StartReplay(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "replay_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

// GET /api/context/replay/{id}
func (s *Server) handleGetReplay(w http.ResponseWriter, r *http.Request) {
	if s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	run, ok := s.d.Engine.Replay(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "No such replay")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// DELETE /api/context/replay/{id} — stop a run.
func (s *Server) handleCancelReplay(w http.ResponseWriter, r *http.Request) {
	if s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	if !s.d.Engine.CancelReplay(r.PathValue("id")) {
		writeError(w, http.StatusNotFound, "not_found", "No such replay")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
