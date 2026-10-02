package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"whatsappdoppel/internal/events"
)

// sseHeartbeat is how often a ": ping" comment is sent to keep the stream alive.
var sseHeartbeat = 15 * time.Second

// handleEvents is the single multiplexed SSE stream (GET /api/events).
// On connect it sends the current wa.status (without an id, so the browser's
// Last-Event-ID is unaffected), then any backlog after Last-Event-ID, then
// live events. It ends when the client goes away or the listener shuts down.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if s.d.Hub == nil {
		unavailable(w, "Events")
		return
	}
	rc := http.NewResponseController(w)
	lastID := parseLastEventID(r)
	ch, backlog, cancel := s.d.Hub.Subscribe(lastID)
	defer cancel()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	write := func(id int64, typ string, data []byte) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Second))
		var b strings.Builder
		if id > 0 {
			fmt.Fprintf(&b, "id: %d\n", id)
		}
		fmt.Fprintf(&b, "event: %s\n", typ)
		// data must not contain raw newlines; compact JSON never does, but be safe.
		for _, line := range strings.Split(string(data), "\n") {
			fmt.Fprintf(&b, "data: %s\n", line)
		}
		b.WriteString("\n")
		if _, err := w.Write([]byte(b.String())); err != nil {
			return false
		}
		return rc.Flush() == nil
	}

	// Ask the browser to wait 2 s before reconnecting after a drop.
	if _, err := w.Write([]byte("retry: 2000\n\n")); err != nil {
		return
	}
	if s.d.WA != nil {
		if b, err := json.Marshal(s.d.WA.Status()); err == nil {
			if !write(0, events.TypeWAStatus, b) {
				return
			}
		}
	} else if rc.Flush() != nil {
		return
	}
	for _, ev := range backlog {
		if !write(ev.ID, ev.Type, ev.Data) {
			return
		}
	}

	ping := time.NewTicker(sseHeartbeat)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if !write(ev.ID, ev.Type, ev.Data) {
				return
			}
		case <-ping.C:
			_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Second))
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			if rc.Flush() != nil {
				return
			}
		}
	}
}

func parseLastEventID(r *http.Request) int64 {
	v := r.Header.Get("Last-Event-ID")
	if v == "" {
		v = r.URL.Query().Get("lastEventId")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
