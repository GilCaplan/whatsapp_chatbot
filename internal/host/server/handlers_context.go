package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/core/events"
	"whatsappdoppel/internal/core/model"
)

// What the persona reads in a chat (engine/context.go): the full context of
// its next reply, the summary of older messages (edit, compact, forget) and
// — through PATCH /api/chats/{key} — instructions, contextFrom, autoCompact.

func (s *Server) handleChatContext(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	v, err := s.d.Engine.ChatContext(c.Key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "context_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// PUT /api/chats/{key}/summary {text} — your edit of the summary.
func (s *Server) handlePutSummary(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" || utf8.RuneCountInString(text) > model.MaxSummaryText {
		writeError(w, http.StatusBadRequest, "invalid_summary", "Write the summary in at most 4000 characters (or forget it)")
		return
	}
	sm, _ := s.d.Store.Summary(c.Key)
	sm.ChatKey, sm.Text, sm.Edited, sm.UpdatedAt = c.Key, text, true, time.Now()
	if err := s.d.Store.SaveSummary(sm); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.contextChanged(c.Key)
	writeJSON(w, http.StatusOK, sm)
}

// DELETE /api/chats/{key}/summary — forget the summary.
func (s *Server) handleDeleteSummary(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	if err := s.d.Store.DeleteSummary(c.Key); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.contextChanged(c.Key)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/chats/{key}/compact {keep?} — summarise the older messages now.
func (s *Server) handleCompact(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	var body struct {
		Keep int `json:"keep"`
	}
	if r.ContentLength != 0 && !decodeJSON(w, r, &body) {
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	sm, err := s.d.Engine.Compact(ctx, c.Key, min(max(body.Keep, 0), 200))
	if err != nil {
		writeError(w, http.StatusBadGateway, "compact_failed", err.Error())
		return
	}
	s.contextChanged(c.Key)
	writeJSON(w, http.StatusOK, sm)
}

func (s *Server) contextChanged(chatKey string) {
	if s.d.Hub != nil {
		s.d.Hub.Publish(events.TypeMemoriesChanged, map[string]string{"chatKey": chatKey})
	}
}

// parseContextFrom reads PATCH contextFrom: null (all history), "now" or a time.
func parseContextFrom(raw json.RawMessage, now time.Time) (*time.Time, error) {
	t := string(bytes.TrimSpace(raw))
	switch t {
	case "null":
		return nil, nil
	case `"now"`:
		return &now, nil
	}
	var v time.Time
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
