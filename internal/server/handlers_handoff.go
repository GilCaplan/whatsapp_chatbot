package server

import "net/http"

// Hand-off (wave 3, owner: Engineer B): resume a chat that paused because a
// message needed you. See WAVE3_PLAN §3.6.

func (s *Server) handleResumeHandoff(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	err := s.d.Engine.ResumeHandoff(c.Key)
	if writeNotImplemented(w, err, "Hand-off") {
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "resume_failed", err.Error())
		return
	}
	c, _ = s.d.Store.Chat(c.Key)
	s.chatsChanged()
	writeJSON(w, http.StatusOK, c)
}
