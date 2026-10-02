package server

import (
	"context"
	"net/http"
	"time"
)

// Notifications (wave 3, owner: Engineer B). POST /api/system/notify-test
// shows "Notifications are working"; with ?dry=1 it only reports the backend
// (Settings uses it for the hint). 501 when no Notifier is wired.
// See WAVE3_PLAN §3.12.

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	if s.d.Notifier == nil {
		notImplemented(w, "Notifications")
		return
	}
	events := true
	if ev, ok := s.d.Notifier.(interface{ EventsEnabled() bool }); ok {
		events = ev.EventsEnabled()
	}
	out := map[string]any{"ok": true, "backend": s.d.Notifier.Backend(), "events": events}
	if r.URL.Query().Get("dry") == "1" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.d.Notifier.Test(ctx); err != nil {
		if writeNotImplemented(w, err, "Notifications") {
			return
		}
		writeError(w, http.StatusBadGateway, "notify_failed", "Couldn't show a notification: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}
