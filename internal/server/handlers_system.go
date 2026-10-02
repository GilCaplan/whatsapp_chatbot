package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"whatsappdoppel/internal/events"
)

func (s *Server) dataDir() string {
	if s.d.Config == nil {
		return ""
	}
	return s.d.Config.Paths().Dir
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	onboarding := false
	if s.d.Config != nil {
		onboarding = s.d.Config.Get().OnboardingCompleted
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                  true,
		"version":             s.d.Version,
		"port":                s.Port(),
		"token":               s.token,
		"startedAt":           s.startedAt,
		"dataDir":             s.dataDir(),
		"onboardingCompleted": onboarding,
		"fakeWA":              s.d.FakeWA,
		"fakeLLM":             s.d.FakeLLM,
	})
}

func (s *Server) handlePorts(w http.ResponseWriter, r *http.Request) {
	from := clamp(queryInt(r, "from", 7000), 1024, 65535)
	to := clamp(queryInt(r, "to", 9999), from, 65535)
	limit := clamp(queryInt(r, "limit", 20), 1, 100)
	cur := s.Port()
	writeJSON(w, http.StatusOK, map[string]any{
		"current": cur,
		"free":    FreePorts(from, to, limit, cur),
	})
}

func (s *Server) handleChangePort(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Port json.Number `json:"port"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	port, err := strconv.Atoi(strings.TrimSpace(body.Port.String()))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_port", "Port must be a number between 1024 and 65535")
		return
	}
	url, err := s.Rebind(port)
	switch {
	case errors.Is(err, ErrInvalidPort):
		writeError(w, http.StatusBadRequest, "invalid_port", "Port must be between 1024 and 65535 and differ from the current port")
	case errors.Is(err, ErrPortInUse):
		writeError(w, http.StatusConflict, "port_in_use", "Port "+strconv.Itoa(port)+" is already in use")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "rebind_failed", err.Error())
	default:
		writeJSON(w, http.StatusOK, map[string]string{"url": url})
	}
}

func (s *Server) handleQuit(w http.ResponseWriter, r *http.Request) {
	if s.d.Hub != nil {
		s.d.Hub.Publish(events.TypeSystem, map[string]any{"kind": "quitting"})
	}
	writeOK(w)
	_ = http.NewResponseController(w).Flush()
	if s.d.OnQuit != nil {
		go func() {
			// Give the response (and the SSE "quitting" frame) a moment to reach the browser.
			time.Sleep(150 * time.Millisecond)
			s.d.OnQuit()
		}()
	}
}

func (s *Server) handleOpenDataDir(w http.ResponseWriter, r *http.Request) {
	dir := s.dataDir()
	if dir == "" {
		unavailable(w, "Data folder")
		return
	}
	cmd := exec.Command("open", dir)
	if err := cmd.Start(); err != nil {
		writeError(w, http.StatusInternalServerError, "open_failed", "Could not open the data folder: "+err.Error())
		return
	}
	go cmd.Wait()
	writeOK(w)
}

// ─── Activity ────────────────────────────────────────────────

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	if s.d.Hub == nil {
		unavailable(w, "Activity")
		return
	}
	q := r.URL.Query()
	since, _ := strconv.ParseInt(q.Get("since"), 10, 64)
	var types map[string]bool
	if t := strings.TrimSpace(q.Get("types")); t != "" {
		types = map[string]bool{}
		for _, x := range strings.Split(t, ",") {
			if x = strings.TrimSpace(x); x != "" {
				types[x] = true
			}
		}
	}
	items, last := s.d.Hub.ActivitySince(since, q.Get("chat"), types, queryInt(r, "limit", 200))
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "lastId": last})
}

func (s *Server) handleClearActivity(w http.ResponseWriter, r *http.Request) {
	if s.d.Hub == nil {
		unavailable(w, "Activity")
		return
	}
	s.d.Hub.ClearActivity()
	writeOK(w)
}
