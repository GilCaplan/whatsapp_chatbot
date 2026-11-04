package server

import (
	"crypto/subtle"
	"fmt"
	"mime"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// middleware wraps h with (outermost first): panic recovery, request logging,
// the Host/Origin/Sec-Fetch-Site guard and the token check for mutating methods.
func (s *Server) middleware(h http.Handler) http.Handler {
	return s.recoverer(s.logger(s.guard(h)))
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.logf("panic serving %s %s: %v\n%s", r.Method, r.URL.Path, v, debug.Stack())
				writeError(w, http.StatusInternalServerError, "internal", "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach Flush/SetWriteDeadline.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *Server) logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		// Keep the log readable: skip successful picture fetches.
		if status < 400 && r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/avatar") {
			return
		}
		s.logf("%s %s %d %s", r.Method, r.URL.Path, status, time.Since(start).Round(time.Millisecond))
	})
}

func isMutating(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// requestPort is the local port the request arrived on (so that, during a
// port change, each listener accepts its own Host). Falls back to Port().
func (s *Server) requestPort(r *http.Request) int {
	if a, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		if p := portOf(a); p != 0 {
			return p
		}
	}
	return s.Port()
}

func allowedHost(host string, port int) bool {
	h, p, err := net.SplitHostPort(host)
	if err != nil {
		return false
	}
	if p != strconv.Itoa(port) {
		return false
	}
	return h == "127.0.0.1" || strings.EqualFold(h, "localhost")
}

func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		port := s.requestPort(r)
		// DNS-rebinding guard.
		if !allowedHost(r.Host, port) {
			writeError(w, http.StatusForbidden, "bad_host", "Requests must use 127.0.0.1 or localhost")
			return
		}
		isAPI := strings.HasPrefix(r.URL.Path, "/api/")
		// Cross-site requests may only be top-level navigations to the UI.
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			if isAPI || r.Header.Get("Sec-Fetch-Mode") != "navigate" || isMutating(r.Method) {
				writeError(w, http.StatusForbidden, "cross_site", "Cross-site requests are not allowed")
				return
			}
		}
		if origin := r.Header.Get("Origin"); origin != "" && (isAPI || isMutating(r.Method)) {
			if origin != fmt.Sprintf("http://127.0.0.1:%d", port) && origin != fmt.Sprintf("http://localhost:%d", port) {
				writeError(w, http.StatusForbidden, "bad_origin", "Origin not allowed")
				return
			}
		}
		if isMutating(r.Method) {
			got := r.Header.Get("X-Doppel-Token")
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				writeError(w, http.StatusUnauthorized, "bad_token", "Missing or invalid X-Doppel-Token")
				return
			}
			if hasBody(r) {
				mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if mt != "application/json" && mt != "multipart/form-data" {
					writeError(w, http.StatusUnsupportedMediaType, "bad_content_type", "Content-Type must be application/json")
					return
				}
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func hasBody(r *http.Request) bool {
	if r.ContentLength > 0 {
		return true
	}
	return r.ContentLength < 0 && r.Body != nil && r.Body != http.NoBody
}
