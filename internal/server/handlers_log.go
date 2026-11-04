package server

import (
	"net/http"

	"whatsappdoppel/internal/model"
)

// Decision log (store/decisions.go, kept by the engine): per chat, what the
// persona did about each message, why, and how long it took.

func (s *Server) handleChatLog(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	items, err := s.d.Store.Decisions(c.Key, min(max(queryInt(r, "limit", 200), 1), 500))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []model.LogEntry `json:"items"`
	}{items})
}

func (s *Server) handleClearChatLog(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	if err := s.d.Store.ClearDecisions(c.Key); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
