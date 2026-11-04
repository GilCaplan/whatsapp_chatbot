package server

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/core/events"
	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/core/store"
)

// Personal secretary: the owner's calendars (read-only; .ics links and the
// MACalendar app) and the messages taken for the owner ("Messages for you").
// Calendar links are secret addresses: the API only ever returns them masked.

// calendarView is a calendar source with its link masked.
type calendarView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Where   string `json:"where"` // the link's host (and path start), masked
	HasKey  bool   `json:"hasKey"`
	Enabled bool   `json:"enabled"`
	// SkipWords: entries whose title contains one of these don't count as busy.
	SkipWords []string `json:"skipWords"`
}

func maskURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "…"
	}
	if u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" {
		return u.Host // this Mac: nothing secret
	}
	return u.Scheme + "://" + u.Host + "/…"
}

func calendarViews(list []model.CalendarSource) []calendarView {
	out := make([]calendarView, 0, len(list))
	for _, c := range list {
		out = append(out, calendarView{ID: c.ID, Name: c.Name, Kind: c.Kind, Where: maskURL(c.URL), HasKey: c.APIKey != "", Enabled: c.Enabled,
			SkipWords: append([]string{}, c.SkipWords...)})
	}
	return out
}

func (s *Server) handleCalendars(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": calendarViews(s.d.Store.Calendars())})
}

type calendarBody struct {
	Name      *string   `json:"name"`
	Kind      *string   `json:"kind"`
	URL       *string   `json:"url"`
	APIKey    *string   `json:"apiKey"`
	Enabled   *bool     `json:"enabled"`
	SkipWords *[]string `json:"skipWords"`
}

// apply validates and merges a body into src (new = POST: kind and url required).
func (b calendarBody) apply(src *model.CalendarSource, isNew bool) string {
	if b.Kind != nil {
		src.Kind = strings.TrimSpace(*b.Kind)
	}
	if src.Kind != model.CalendarICS && src.Kind != model.CalendarMACalendar {
		return "kind must be ics or macalendar"
	}
	if b.Name != nil {
		src.Name = strings.TrimSpace(*b.Name)
	}
	if utf8.RuneCountInString(src.Name) > 60 {
		return "a name has at most 60 characters"
	}
	if b.URL != nil {
		src.URL = strings.TrimSpace(*b.URL)
	}
	if src.Kind == model.CalendarMACalendar && src.URL == "" {
		src.URL = "http://127.0.0.1:8080"
	}
	low := strings.ToLower(src.URL)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") && !strings.HasPrefix(low, "webcal://") {
		return "the link must start with https://, http:// or webcal://"
	}
	if b.APIKey != nil {
		src.APIKey = strings.TrimSpace(*b.APIKey)
	}
	if b.Enabled != nil {
		src.Enabled = *b.Enabled
	} else if isNew {
		src.Enabled = true
	}
	if b.SkipWords != nil {
		var words []string
		for _, w := range *b.SkipWords {
			if w = strings.TrimSpace(w); w != "" && !slices.Contains(words, w) {
				words = append(words, w)
			}
		}
		if len(words) > 20 {
			return "at most 20 words to skip"
		}
		for _, w := range words {
			if utf8.RuneCountInString(w) > 40 {
				return "a word to skip has at most 40 characters"
			}
		}
		src.SkipWords = words
	}
	if src.Name == "" {
		src.Name = map[string]string{model.CalendarICS: "Calendar", model.CalendarMACalendar: "MACalendar"}[src.Kind]
	}
	return ""
}

func (s *Server) handleAddCalendar(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	var b calendarBody
	if !decodeJSON(w, r, &b) {
		return
	}
	src := model.CalendarSource{ID: store.NewID()}
	if msg := b.apply(&src, true); msg != "" {
		writeError(w, http.StatusBadRequest, "invalid_calendar", msg)
		return
	}
	list, err := s.d.Store.UpdateCalendars(func(cur []model.CalendarSource) ([]model.CalendarSource, error) {
		return append(cur, src), nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"items": calendarViews(list)})
}

func (s *Server) handlePatchCalendar(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	var b calendarBody
	if !decodeJSON(w, r, &b) {
		return
	}
	id := r.PathValue("id")
	var msg string
	found := false
	list, err := s.d.Store.UpdateCalendars(func(cur []model.CalendarSource) ([]model.CalendarSource, error) {
		for i := range cur {
			if cur[i].ID == id {
				found = true
				msg = b.apply(&cur[i], false)
			}
		}
		return cur, nil
	})
	switch {
	case !found:
		writeError(w, http.StatusNotFound, "not_found", "No such calendar")
	case msg != "":
		writeError(w, http.StatusBadRequest, "invalid_calendar", msg)
	case err != nil:
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
	default:
		writeJSON(w, http.StatusOK, map[string]any{"items": calendarViews(list)})
	}
}

func (s *Server) handleDeleteCalendar(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	id := r.PathValue("id")
	list, err := s.d.Store.UpdateCalendars(func(cur []model.CalendarSource) ([]model.CalendarSource, error) {
		out := cur[:0]
		for _, c := range cur {
			if c.ID != id {
				out = append(out, c)
			}
		}
		return out, nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": calendarViews(list)})
}

// POST /api/calendars/{id}/check — read it now: ok, events ahead, the next few.
func (s *Server) handleCheckCalendar(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	id := r.PathValue("id")
	for _, c := range s.d.Store.Calendars() {
		if c.ID == id {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			writeJSON(w, http.StatusOK, s.d.Engine.CalendarCheck(ctx, c))
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "No such calendar")
}

// GET /api/secretary/notes — messages taken for you, newest first.
func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.d.Store.SecretaryNotes()})
}

// PATCH /api/secretary/notes/{id} {done} — mark one done (or open again).
func (s *Server) handlePatchNote(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	var b struct {
		Done *bool `json:"done"`
	}
	if !decodeJSON(w, r, &b) {
		return
	}
	id := r.PathValue("id")
	found := false
	err := s.d.Store.UpdateSecretaryNotes(func(all []model.SecretaryNote) []model.SecretaryNote {
		for i := range all {
			if all[i].ID == id {
				found = true
				if b.Done != nil {
					all[i].Done = *b.Done
				}
			}
		}
		return all
	})
	switch {
	case err != nil:
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
	case !found:
		writeError(w, http.StatusNotFound, "not_found", "No such message")
	default:
		s.notesChanged()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// DELETE /api/secretary/notes/{id} — forget one ("done" ones: ?done=1 clears them all).
func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	id := r.PathValue("id")
	err := s.d.Store.UpdateSecretaryNotes(func(all []model.SecretaryNote) []model.SecretaryNote {
		out := all[:0]
		for _, n := range all {
			if n.ID != id && !(id == "done" && n.Done) {
				out = append(out, n)
			}
		}
		return out
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.notesChanged()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) notesChanged() {
	if s.d.Hub != nil {
		s.d.Hub.Publish(events.TypeNotesChanged, struct{}{})
	}
}
