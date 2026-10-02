package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"whatsappdoppel/internal/contract"
)

const maxJSONBody = 2 << 20 // 2 MB

// apiError is the JSON error body: {"error": "...", "code": "..."}.
type apiError struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, apiError{Error: msg, Code: code})
}

func writeOK(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// unavailable answers 503 when an optional dependency is not wired.
func unavailable(w http.ResponseWriter, what string) {
	writeError(w, http.StatusServiceUnavailable, "unavailable", what+" is not available")
}

// notImplemented answers 501 for a wave 3 feature whose contract exists but
// whose implementation hasn't landed yet.
func notImplemented(w http.ResponseWriter, what string) {
	writeError(w, http.StatusNotImplemented, "not_implemented", what+" isn't available yet")
}

// writeNotImplemented answers 501 and returns true when err is
// contract.ErrNotImplemented.
func writeNotImplemented(w http.ResponseWriter, err error, what string) bool {
	if errors.Is(err, contract.ErrNotImplemented) {
		notImplemented(w, what)
		return true
	}
	return false
}

// decodeJSON reads a JSON body into v. An empty body leaves v untouched.
// On failure it writes a 400/413 and returns false.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	b, err := io.ReadAll(r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "Request body is too large")
			return false
		}
		writeError(w, http.StatusBadRequest, "bad_request", "Could not read request body")
		return false
	}
	if len(b) == 0 {
		return true
	}
	if err := json.Unmarshal(b, v); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", "Invalid JSON: "+err.Error())
		return false
	}
	return true
}

// queryInt parses an integer query parameter, returning def when absent/invalid.
func queryInt(r *http.Request, name string, def int) int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
