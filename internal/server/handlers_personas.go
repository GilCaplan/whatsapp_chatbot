package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

const maxAvatarBytes = 5 << 20 // 5 MB

func (s *Server) personasChanged() {
	if s.d.Hub != nil {
		s.d.Hub.Publish(events.TypePersonasChanged, struct{}{})
	}
}

func (s *Server) normalize(p *model.Persona) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return errors.New("a persona needs a name")
	}
	if s.d.Normalize != nil {
		if err := s.d.Normalize(p); err != nil {
			return err
		}
	}
	if p.Avatar.Kind == "" {
		p.Avatar.Kind = "generated"
	}
	return nil
}

func (s *Server) requirePersona(w http.ResponseWriter, id string) (model.Persona, bool) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return model.Persona{}, false
	}
	p, ok := s.d.Store.Persona(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "No such persona")
	}
	return p, ok
}

func (s *Server) handleListPersonas(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	writeJSON(w, http.StatusOK, nonNilSlice(s.d.Store.Personas()))
}

func (s *Server) handleGetPersona(w http.ResponseWriter, r *http.Request) {
	if p, ok := s.requirePersona(w, r.PathValue("id")); ok {
		writeJSON(w, http.StatusOK, p)
	}
}

func (s *Server) handleCreatePersona(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	var p model.Persona
	if !decodeJSON(w, r, &p) {
		return
	}
	p.ID = ""
	p.BuiltIn = false
	if p.Avatar.Kind == "upload" { // no file exists yet for a brand-new persona
		p.Avatar.Kind = "generated"
		p.Avatar.Version = 0
	}
	if err := s.normalize(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_persona", err.Error())
		return
	}
	saved, err := s.d.Store.UpsertPersona(p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.personasChanged()
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handlePutPersona(w http.ResponseWriter, r *http.Request) {
	cur, ok := s.requirePersona(w, r.PathValue("id"))
	if !ok {
		return
	}
	var p model.Persona
	if !decodeJSON(w, r, &p) {
		return
	}
	p.ID = cur.ID
	p.BuiltIn = cur.BuiltIn
	p.CreatedAt = cur.CreatedAt
	// The avatar file is managed by the avatar endpoints: keep its kind/version
	// authoritative so a stale editor can't point at a missing file.
	if cur.Avatar.Kind == "upload" && p.Avatar.Kind != "generated" {
		p.Avatar.Kind = "upload"
		p.Avatar.Version = cur.Avatar.Version
	} else if p.Avatar.Kind == "upload" {
		p.Avatar.Kind = "generated"
		p.Avatar.Version = 0
	}
	if cur.Avatar.Kind == "upload" && p.Avatar.Kind == "generated" {
		_ = os.Remove(s.d.Store.AvatarPath(cur.ID))
	}
	if err := s.normalize(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_persona", err.Error())
		return
	}
	saved, err := s.d.Store.UpsertPersona(p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.personasChanged()
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeletePersona(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requirePersona(w, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.d.Store.DeletePersona(p.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	_ = os.Remove(s.d.Store.AvatarPath(p.ID))
	disabled := 0
	for _, c := range s.d.Store.Chats() {
		if c.PersonaID != p.ID {
			continue
		}
		if _, err := s.d.Store.UpdateChat(c.Key, func(c *model.ChatAssignment) { c.Enabled = false }); err == nil {
			disabled++
			if s.d.Hub != nil {
				s.d.Hub.Activity(model.ActivityEvent{
					Type:        model.ActError,
					ChatKey:     c.Key,
					ChatName:    c.Name,
					PersonaName: p.Name,
					Text:        "Persona " + p.Name + " was deleted — chat paused until you pick another persona",
					Meta:        map[string]any{"reason": "persona_deleted"},
				})
			}
		}
	}
	if disabled > 0 {
		s.reloadEngine()
		s.chatsChanged()
	}
	s.personasChanged()
	writeOK(w)
}

func (s *Server) handleDuplicatePersona(w http.ResponseWriter, r *http.Request) {
	src, ok := s.requirePersona(w, r.PathValue("id"))
	if !ok {
		return
	}
	p := src
	p.ID = ""
	p.BuiltIn = false
	p.Name = strings.TrimSpace(src.Name + " copy")
	p.CreatedAt = time.Time{}
	if src.Avatar.Kind == "upload" {
		p.Avatar.Kind = "generated" // until the file is copied below
	}
	if p.Avatar.Glyph == "" && p.Avatar.Initials == "" {
		p.Avatar.Initials = initials(p.Name)
	}
	if src.LLM != nil {
		l := *src.LLM
		p.LLM = &l
	}
	p.Emoji.Favorites = append([]string(nil), src.Emoji.Favorites...)
	p.Avatar.Gradient = append([]string(nil), src.Avatar.Gradient...)
	saved, err := s.d.Store.UpsertPersona(p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	if src.Avatar.Kind == "upload" {
		if b, err := os.ReadFile(s.d.Store.AvatarPath(src.ID)); err == nil {
			if config.WriteFileAtomic(s.d.Store.AvatarPath(saved.ID), b, 0o600) == nil {
				saved.Avatar.Kind = "upload"
				saved.Avatar.Version = time.Now().UnixMilli()
				if s2, err := s.d.Store.UpsertPersona(saved); err == nil {
					saved = s2
				}
			}
		}
	}
	s.personasChanged()
	writeJSON(w, http.StatusCreated, saved)
}

func initials(name string) string {
	var out []rune
	for _, f := range strings.Fields(name) {
		r := []rune(f)
		if len(r) > 0 {
			out = append(out, r[0])
		}
		if len(out) == 2 {
			break
		}
	}
	return strings.ToUpper(string(out))
}

func (s *Server) handleResetPersona(w http.ResponseWriter, r *http.Request) {
	cur, ok := s.requirePersona(w, r.PathValue("id"))
	if !ok {
		return
	}
	if !cur.BuiltIn || s.d.Seed == nil {
		writeError(w, http.StatusBadRequest, "not_builtin", "Only built-in personas can be reset")
		return
	}
	seed, ok := s.d.Seed(cur.ID)
	if !ok {
		writeError(w, http.StatusNotFound, "no_seed", "No factory version exists for this persona")
		return
	}
	seed.ID = cur.ID
	seed.BuiltIn = true
	seed.CreatedAt = cur.CreatedAt
	if cur.Avatar.Kind == "upload" {
		_ = os.Remove(s.d.Store.AvatarPath(cur.ID))
	}
	_ = s.normalize(&seed)
	saved, err := s.d.Store.UpsertPersona(seed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.personasChanged()
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleUploadAvatar(w http.ResponseWriter, r *http.Request) {
	cur, ok := s.requirePersona(w, r.PathValue("id"))
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes+1<<20) // + multipart overhead
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "Pictures must be 5 MB or smaller")
			return
		}
		writeError(w, http.StatusBadRequest, "bad_upload", "Expected a multipart upload with a \"file\" field")
		return
	}
	defer r.MultipartForm.RemoveAll()
	f, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_upload", "Expected a multipart upload with a \"file\" field")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxAvatarBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_upload", "Could not read the picture")
		return
	}
	if len(data) > maxAvatarBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "Pictures must be 5 MB or smaller")
		return
	}
	png, err := ProcessAvatar(data, 512)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_image", err.Error())
		return
	}
	if err := config.WriteFileAtomic(s.d.Store.AvatarPath(cur.ID), png, 0o600); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	cur.Avatar.Kind = "upload"
	cur.Avatar.Version = max(time.Now().UnixMilli(), cur.Avatar.Version+1)
	saved, err := s.d.Store.UpsertPersona(cur)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.personasChanged()
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteAvatar(w http.ResponseWriter, r *http.Request) {
	cur, ok := s.requirePersona(w, r.PathValue("id"))
	if !ok {
		return
	}
	_ = os.Remove(s.d.Store.AvatarPath(cur.ID))
	cur.Avatar.Kind = "generated"
	cur.Avatar.Version = 0
	_ = s.normalize(&cur)
	saved, err := s.d.Store.UpsertPersona(cur)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.personasChanged()
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleGetAvatar(w http.ResponseWriter, r *http.Request) {
	cur, ok := s.requirePersona(w, r.PathValue("id"))
	if !ok {
		return
	}
	b, err := os.ReadFile(s.d.Store.AvatarPath(cur.ID))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "This persona has no uploaded picture")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	if r.URL.Query().Get("v") != "" {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.Write(b)
}

func (s *Server) handleDraftPersona(w http.ResponseWriter, r *http.Request) {
	if s.d.Builder == nil {
		unavailable(w, "Persona builder")
		return
	}
	var req model.DraftRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Description = strings.TrimSpace(req.Description)
	if req.Description == "" && strings.TrimSpace(req.Samples) == "" {
		writeError(w, http.StatusBadRequest, "missing_description", "Describe the persona (or paste a few sample messages)")
		return
	}
	if req.Provider != "" && !validProvider(req.Provider) {
		writeError(w, http.StatusBadRequest, "invalid_provider", "provider must be ollama, anthropic or openai")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()
	p, raw, err := s.d.Builder.Draft(ctx, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "llm_error", err.Error())
		return
	}
	p.ID = ""
	p.BuiltIn = false
	if err := s.normalize(&p); err != nil {
		writeError(w, http.StatusBadGateway, "llm_error", "The model returned an unusable persona: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"persona": p, "raw": raw})
}

func (s *Server) handlePromptPreview(w http.ResponseWriter, r *http.Request) {
	if s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	cur, ok := s.requirePersona(w, r.PathValue("id"))
	if !ok {
		return
	}
	sys, err := s.d.Engine.PromptPreview(cur.ID, r.URL.Query().Get("chatKey"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "preview_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"system": sys})
}

// handleExpressionPreview writes sample replies at a persona's emoji and
// length settings. {id} is a saved persona or "new"; body.persona (optional
// for saved ones) carries unsaved editor fields.
func (s *Server) handleExpressionPreview(w http.ResponseWriter, r *http.Request) {
	if s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	var body struct {
		Persona    *model.Persona `json:"persona"`
		LengthBias string         `json:"lengthBias"`
		Count      int            `json:"count"`
		Roll       int            `json:"roll"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	var p model.Persona
	if id := r.PathValue("id"); id != "new" {
		cur, ok := s.requirePersona(w, id)
		if !ok {
			return
		}
		p = cur
		if body.Persona != nil {
			p = *body.Persona
			p.ID, p.BuiltIn, p.CreatedAt = cur.ID, cur.BuiltIn, cur.CreatedAt
		}
	} else if body.Persona != nil {
		p = *body.Persona
		p.ID, p.BuiltIn = "", false
	} else {
		writeError(w, http.StatusBadRequest, "missing_persona", "Send the persona's fields to preview it")
		return
	}
	if strings.TrimSpace(p.Name) == "" {
		p.Name = "Them"
	}
	if err := s.normalize(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_persona", err.Error())
		return
	}
	switch body.LengthBias {
	case "", "shorter", "normal", "longer", "match":
	default:
		writeError(w, http.StatusBadRequest, "invalid_length_bias", "lengthBias must be shorter, normal, longer or match")
		return
	}
	if body.Count <= 0 || body.Count > 3 {
		body.Count = 3
	}
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()
	out, err := s.d.Engine.ExpressionPreview(ctx, p, body.LengthBias, body.Count, body.Roll)
	if err != nil {
		writeError(w, http.StatusBadGateway, "preview_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ─── Playground ──────────────────────────────────────────────

func playgroundStatus(err error) int {
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "not found") || strings.Contains(msg, "unknown") || strings.Contains(msg, "expired") {
		return http.StatusNotFound
	}
	return http.StatusBadGateway
}

func (s *Server) handlePlaygroundStart(w http.ResponseWriter, r *http.Request) {
	if s.d.Playground == nil {
		unavailable(w, "Playground")
		return
	}
	var body struct {
		PersonaID string `json:"personaId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if s.d.Store != nil {
		if _, ok := s.d.Store.Persona(body.PersonaID); !ok {
			writeError(w, http.StatusBadRequest, "unknown_persona", "Unknown persona")
			return
		}
	}
	id, err := s.d.Playground.Start(body.PersonaID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "playground_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"sessionId": id})
}

func (s *Server) handlePlaygroundSend(w http.ResponseWriter, r *http.Request) {
	if s.d.Playground == nil {
		unavailable(w, "Playground")
		return
	}
	var body struct {
		Text  string  `json:"text"`
		Group bool    `json:"group"`
		Goal  *string `json:"goal"` // optional "Goal for this test"; "" = the persona's goal
		// Initiate: the persona speaks first (text is then an optional topic hint).
		Initiate bool `json:"initiate"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Text) == "" && !body.Initiate {
		writeError(w, http.StatusBadRequest, "missing_text", "Message text is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()
	if body.Goal != nil {
		if err := s.d.Playground.SetGoal(r.PathValue("id"), *body.Goal); err != nil {
			writeError(w, playgroundStatus(err), "playground_error", err.Error())
			return
		}
	}
	var reply model.PlaygroundReply
	var err error
	if body.Initiate {
		reply, err = s.d.Playground.Initiate(ctx, r.PathValue("id"), body.Group, body.Text)
	} else {
		reply, err = s.d.Playground.Send(ctx, r.PathValue("id"), body.Text, body.Group)
	}
	if err != nil {
		writeError(w, playgroundStatus(err), "playground_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, reply)
}

func (s *Server) handlePlaygroundEnd(w http.ResponseWriter, r *http.Request) {
	if s.d.Playground == nil {
		unavailable(w, "Playground")
		return
	}
	s.d.Playground.End(r.PathValue("id"))
	writeOK(w)
}
