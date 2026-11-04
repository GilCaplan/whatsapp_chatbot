package server

import (
	"net/http"

	"whatsappdoppel/internal/core/events"
	"whatsappdoppel/internal/core/model"
)

// Nicknames (store/nicknames.go): names you give people, used in every chat
// instead of their WhatsApp or contact name.

func (s *Server) handleNicknames(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []model.Nickname `json:"items"`
	}{s.d.Store.Nicknames()})
}

// PUT /api/nicknames {ids:[jid…], name, original?} — name "" removes it.
func (s *Server) handleSetNickname(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	var body struct {
		IDs      []string `json:"ids"`
		Name     string   `json:"name"`
		Original string   `json:"original"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	nk, err := s.d.Store.SetNickname(body.IDs, body.Name, body.Original)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_nickname", err.Error())
		return
	}
	if s.d.Hub != nil {
		s.d.Hub.Publish(events.TypeNicknamesChanged, struct{}{})
		s.d.Hub.Publish(events.TypeChatsChanged, struct{}{})
	}
	writeJSON(w, http.StatusOK, nk)
}
