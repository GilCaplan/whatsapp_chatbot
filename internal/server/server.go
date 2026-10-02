// Package server implements the local HTTP + SSE API (docs/API.md) and serves
// the embedded web UI. It talks to the rest of the app only through the
// interfaces in internal/contract plus the config/store/events packages.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/contract"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

// Deps is everything the server needs. Nil interface fields make the
// corresponding endpoints answer 503 "unavailable".
type Deps struct {
	Config     *config.Manager
	Store      *store.Store
	Hub        *events.Hub
	WA         contract.WhatsApp
	Engine     contract.Engine
	LLM        contract.LLM
	Playground contract.Playground
	Builder    contract.Builder
	// Notifier shows macOS notifications (wave 3); nil = POST
	// /api/system/notify-test answers 501.
	Notifier contract.Notifier
	// Seed returns the factory version of a built-in persona (for reset).
	Seed func(id string) (model.Persona, bool)
	// Normalize fills persona defaults (avatar gradient, enums, ...) and
	// validates; an error is reported to the client as 400 invalid_persona.
	Normalize func(*model.Persona) error
	// Web is the embedded UI (index.html at its root).
	Web     fs.FS
	Version string
	FakeWA  bool
	FakeLLM bool
	// OnQuit is called (in its own goroutine) after POST /api/system/quit has been answered.
	OnQuit func()
	// OnPortChange is called after a successful Rebind (e.g. to rewrite instance.json).
	OnPortChange func(port int)
	// Logf receives request and error logs; defaults to log.Printf (stderr).
	Logf func(format string, args ...any)
}

// Errors returned by Rebind.
var (
	ErrInvalidPort = errors.New("invalid port")
	ErrPortInUse   = errors.New("port is already in use")
)

// rebindGrace is how long the old listener keeps serving after a port change
// (long enough for the page to receive port_changed and navigate away).
var rebindGrace = 3 * time.Second

// listener is one http.Server bound to one port. During a port change two are alive.
type listener struct {
	srv    *http.Server
	port   int
	cancel context.CancelFunc // cancels BaseContext → ends SSE streams on this listener
}

type Server struct {
	d         Deps
	token     string
	startedAt time.Time
	handler   http.Handler
	logf      func(string, ...any)
	etags     etagCache

	mu        sync.Mutex
	port      int
	listeners []*listener
	closed    bool
	done      chan struct{}
	errCh     chan error
}

// New builds the server and its routes. Call Serve to start accepting connections.
func New(d Deps) *Server {
	s := &Server{
		d:         d,
		token:     newToken(),
		startedAt: time.Now(),
		done:      make(chan struct{}),
		errCh:     make(chan error, 4),
		logf:      d.Logf,
	}
	if s.logf == nil {
		s.logf = log.Printf
	}
	if s.d.Version == "" {
		s.d.Version = "dev"
	}
	s.handler = s.middleware(s.routes())
	return s
}

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Token is the per-process secret required in X-Doppel-Token on mutating requests.
func (s *Server) Token() string { return s.token }

// Port is the port the UI should currently be reached on.
func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

// URL is the browser URL for the current port.
func (s *Server) URL() string { return fmt.Sprintf("http://127.0.0.1:%d/", s.Port()) }

// Handler exposes the full handler chain (useful for tests).
func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) newListener(ln net.Listener) *listener {
	ctx, cancel := context.WithCancel(context.Background())
	l := &listener{port: portOf(ln.Addr()), cancel: cancel}
	l.srv = &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ErrorLog:          log.New(logWriter{s.logf}, "http: ", 0),
	}
	return l
}

func (s *Server) startListener(l *listener, ln net.Listener) {
	go func() {
		err := l.srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			current := l.port == s.port
			s.mu.Unlock()
			if current {
				s.errCh <- err
			} else {
				s.logf("server on old port %d: %v", l.port, err)
			}
		}
	}()
}

// Serve accepts connections on ln and blocks until Shutdown is called (it keeps
// blocking across a Rebind) or the current listener fails.
func (s *Server) Serve(ln net.Listener) error {
	l := s.newListener(ln)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		ln.Close()
		return http.ErrServerClosed
	}
	s.port = l.port
	s.listeners = append(s.listeners, l)
	s.mu.Unlock()
	s.startListener(l, ln)
	select {
	case <-s.done:
		return nil
	case err := <-s.errCh:
		return err
	}
}

// Rebind starts serving on a new port, persists it, notifies the UI and shuts
// the old listener down 3 seconds later. Returns the new URL.
func (s *Server) Rebind(port int) (string, error) {
	if port < 1024 || port > 65535 || port == s.Port() {
		return "", ErrInvalidPort
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return "", ErrPortInUse
	}
	l := s.newListener(ln)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		ln.Close()
		return "", http.ErrServerClosed
	}
	old := append([]*listener(nil), s.listeners...)
	s.port = port
	s.listeners = append(s.listeners, l)
	s.mu.Unlock()
	s.startListener(l, ln)

	if s.d.Config != nil {
		if _, err := s.d.Config.Update(func(st *config.Settings) { st.Port = port }); err != nil {
			s.logf("persist port: %v", err)
		}
	}
	if s.d.OnPortChange != nil {
		s.d.OnPortChange(port)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	if s.d.Hub != nil {
		s.d.Hub.Publish(events.TypeSystem, map[string]any{"kind": "port_changed", "url": url})
	}
	s.logf("port changed → %s (old listener closes in %s)", url, rebindGrace)
	time.AfterFunc(rebindGrace, func() {
		for _, o := range old {
			s.closeListener(o, 10*time.Second)
		}
	})
	return url, nil
}

func (s *Server) closeListener(l *listener, timeout time.Duration) {
	s.mu.Lock()
	for i, x := range s.listeners {
		if x == l {
			s.listeners = append(s.listeners[:i], s.listeners[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	l.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := l.srv.Shutdown(ctx); err != nil {
		l.srv.Close()
	}
}

// Shutdown ends SSE streams, stops all listeners gracefully and makes Serve return.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	ls := append([]*listener(nil), s.listeners...)
	s.listeners = nil
	close(s.done)
	s.mu.Unlock()

	var firstErr error
	for _, l := range ls {
		l.cancel()
	}
	for _, l := range ls {
		if err := l.srv.Shutdown(ctx); err != nil {
			l.srv.Close()
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func portOf(a net.Addr) int {
	if ta, ok := a.(*net.TCPAddr); ok {
		return ta.Port
	}
	_, p, err := net.SplitHostPort(a.String())
	if err != nil {
		return 0
	}
	var n int
	fmt.Sscanf(p, "%d", &n)
	return n
}

type logWriter struct{ logf func(string, ...any) }

func (w logWriter) Write(p []byte) (int, error) {
	w.logf("%s", string(p))
	return len(p), nil
}
