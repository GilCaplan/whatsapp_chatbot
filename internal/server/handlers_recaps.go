package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"whatsappdoppel/internal/model"
)

// Daily recap (wave 3, owner: Engineer B): list stored recaps and write new
// ones on demand (an empty list = nothing new to recap). See WAVE3_PLAN §3.9.

func (s *Server) handleListRecaps(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	chat := strings.TrimSpace(r.URL.Query().Get("chat"))
	limit := clamp(queryInt(r, "limit", 30), 1, 500)
	writeJSON(w, http.StatusOK, model.RecapsView{Items: nonNilSlice(s.d.Store.Recaps(chat, limit))})
}

func (s *Server) handleGenerateRecaps(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	var body struct {
		ChatKey string `json:"chatKey"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ChatKey != "" {
		if _, ok := s.requireChat(w, body.ChatKey); !ok {
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	items, err := s.d.Engine.GenerateRecap(ctx, body.ChatKey)
	if writeNotImplemented(w, err, "Daily recap") {
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "recap_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, model.RecapsView{Items: nonNilSlice(items)})
}
