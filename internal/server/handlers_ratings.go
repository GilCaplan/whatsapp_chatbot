package server

import (
	"context"
	"net/http"
	"time"

	"whatsappdoppel/internal/model"
)

// Connection ratings (engine/ratings.go): how connected the persona felt to
// each person in a group, once a day. Private: nothing goes to WhatsApp.

func (s *Server) handleRatings(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	items, err := s.d.Store.Ratings(c.Key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []model.RatingDay `json:"items"`
	}{items})
}

// POST /api/chats/{key}/ratings — rate the last 24 hours now.
func (s *Server) handleRateNow(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	d, err := s.d.Engine.RateNow(ctx, c.Key)
	if err != nil {
		writeError(w, http.StatusBadGateway, "rating_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}
