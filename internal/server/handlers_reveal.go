package server

import (
	"context"
	"net/http"
	"time"

	"whatsappdoppel/internal/model"
)

// Reveal (wave 3, owner: Engineer B): send the "it was a persona" message,
// pause the chat and mark it revealed. See WAVE3_PLAN §3.8.

// maxRevealText caps an edited reveal message (characters).
const maxRevealText = 2000

func (s *Server) handleReveal(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	var body struct {
		Text  string `json:"text"`
		Force bool   `json:"force"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	if len([]rune(body.Text)) > maxRevealText {
		writeError(w, http.StatusBadRequest, "text_too_long", "The message is too long")
		return
	}
	if c.RevealedAt != nil && !c.Enabled && !body.Force {
		writeError(w, http.StatusConflict, "already_revealed", "You already revealed the persona in this chat. Send it again?")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	text, err := s.d.Engine.Reveal(ctx, c.Key, body.Text, body.Force)
	if writeNotImplemented(w, err, "Reveal") {
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "reveal_failed", err.Error())
		return
	}
	s.chatsChanged()
	writeJSON(w, http.StatusOK, model.RevealResult{OK: true, Text: text})
}
