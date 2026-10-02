// Package app wires every layer together for `serve`: config, store, events,
// WhatsApp (real or fake), LLM registry, engine and the HTTP server, and owns
// the process lifecycle (single-instance lock, instance.json, graceful exit).
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/contract"
	"whatsappdoppel/internal/engine"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/launcher"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/notify"
	"whatsappdoppel/internal/persona"
	"whatsappdoppel/internal/server"
	"whatsappdoppel/internal/store"
	"whatsappdoppel/internal/wa"
	"whatsappdoppel/web"
)

// Options are the `serve` flags.
type Options struct {
	DataDir     string // "" = default (~/Library/Application Support/WhatsappDoppel or $DOPPEL_DATA_DIR)
	Port        int    // 0 = automatic (DOPPEL_PORT > settings.port > well-known list)
	LegacyDB    string // explicit legacy bot.db to import on first start
	FakeWA      bool
	FakeLLM     bool
	OpenBrowser bool
	Version     string
	// DevProjectDir is the source checkout compiled in via -ldflags (used to
	// find the legacy bot.db when running the installed .app).
	DevProjectDir string
	FromLauncher  bool
}

// ErrAlreadyRunning is returned when another server holds the data dir's lock.
var ErrAlreadyRunning = errors.New("already running")

// fallbackPorts are tried (in order) when neither the flag, DOPPEL_PORT nor
// settings.port can be bound.
var fallbackPorts = []int{7788, 7789, 8080, 8787, 9000, 9001, 9002, 9003, 9004, 9005}

// Run serves until SIGINT/SIGTERM or POST /api/system/quit.
func Run(opts Options) error {
	paths, err := config.NewPaths(opts.DataDir)
	if err != nil {
		return fmt.Errorf("data folder: %w", err)
	}

	// Single instance per data dir, before touching any database.
	lock, err := launcher.AcquireLock(paths)
	if errors.Is(err, launcher.ErrLocked) {
		if inst, ok := launcher.Running(context.Background(), paths); ok {
			fmt.Fprintf(os.Stderr, "WhatsApp Doppel is already running at %s\n", inst.URL())
			if opts.OpenBrowser {
				_ = launcher.OpenBrowser(inst.URL())
			}
		} else {
			fmt.Fprintf(os.Stderr, "Another WhatsApp Doppel server is starting for %s\n", paths.Dir)
		}
		return ErrAlreadyRunning
	}
	if err != nil {
		return err
	}
	defer lock.Release()

	log.SetFlags(log.LstdFlags)
	log.Printf("WhatsApp Doppel %s starting (data: %s, fakeWA=%v, fakeLLM=%v, fromLauncher=%v)",
		opts.Version, paths.Dir, opts.FakeWA, opts.FakeLLM, opts.FromLauncher)

	cfg, err := config.Load(paths)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	st, err := store.Open(paths, persona.Seeds())
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	hub := events.NewHub()
	// Activity goes to the log file and to macOS notifications (notify).
	var baseURL atomic.Value // "http://127.0.0.1:<port>/", for notification clicks
	baseURL.Store("")
	notifier := notify.New(notify.Options{
		Config:  cfg,
		URL:     func() string { s, _ := baseURL.Load().(string); return s },
		Events:  notifyEvents(opts),
		Backend: notifyBackend(),
	})
	fileSink := events.FileSink(paths.LogsDir())
	hub.SetActivitySink(func(a model.ActivityEvent) {
		fileSink(a)
		notifier.OnActivity(a)
	})

	var waSvc contract.WhatsApp
	if opts.FakeWA {
		waSvc = wa.NewFake(hub)
	} else {
		m, err := wa.New(wa.Options{Paths: paths, Hub: hub, LegacyDBCandidates: legacyCandidates(opts),
			CollectSelf: selfSampleConsent(cfg), OnSelfMessage: selfSampleSink(cfg, st)}) // Clone yourself (selfsamples.go)
		if err != nil {
			return fmt.Errorf("WhatsApp: %w", err)
		}
		waSvc = m
	}

	llmReg := llm.NewRegistry(cfg, hub)
	if opts.FakeLLM {
		llmReg.UseFake(llm.NewFake())
	}

	eng := engine.New(engine.Deps{Config: cfg, Store: st, Hub: hub, WA: waSvc, LLM: llmReg})
	waSvc.SetMessageHandler(eng.HandleIncoming)

	ln, err := listen(opts.Port, cfg.Get().Port)
	if err != nil {
		waSvc.Close()
		return err
	}

	quit := make(chan struct{})
	var quitOnce sync.Once
	requestQuit := func() { quitOnce.Do(func() { close(quit) }) }

	var srv *server.Server
	startedAt := time.Now()
	writeInstance := func(port int) {
		baseURL.Store(fmt.Sprintf("http://127.0.0.1:%d/", port))
		inst := launcher.Instance{
			PID: os.Getpid(), Port: port, Token: srv.Token(),
			StartedAt: startedAt, Version: opts.Version,
		}
		if err := launcher.WriteInstance(paths, inst); err != nil {
			log.Printf("write instance.json: %v", err)
		}
	}
	srv = server.New(server.Deps{
		Config:     cfg,
		Store:      st,
		Hub:        hub,
		WA:         waSvc,
		Engine:     eng,
		LLM:        llmReg,
		Playground: eng.Playground(),
		Builder:    eng.Builder(),
		Notifier:   notifier,
		Seed:       persona.Seed,
		Normalize:  persona.Normalize,
		Web:        web.FS,
		Version:    opts.Version,
		FakeWA:     opts.FakeWA,
		FakeLLM:    opts.FakeLLM,
		OnQuit:     requestQuit,
		OnPortChange: func(port int) {
			writeInstance(port)
		},
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	port := ln.Addr().(*net.TCPAddr).Port
	writeInstance(port)
	defer launcher.RemoveInstance(paths)

	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	log.Printf("serving on %s", url)
	hub.Activity(model.ActivityEvent{Type: model.ActSystem, Text: "Server started on " + url})

	eng.Reload()
	waCtx, waCancel := context.WithCancel(context.Background())
	defer waCancel()
	go func() {
		if err := waSvc.Start(waCtx); err != nil {
			log.Printf("WhatsApp start: %v", err)
		}
	}()

	if opts.OpenBrowser {
		go func() {
			if err := launcher.OpenBrowser(url); err != nil {
				log.Printf("open browser: %v", err)
			}
		}()
	}

	var runErr error
	select {
	case <-ctx.Done():
		log.Printf("signal received, shutting down")
	case <-quit:
		log.Printf("quit requested, shutting down")
	case err := <-serveErr:
		if err != nil {
			runErr = fmt.Errorf("server: %w", err)
			log.Printf("%v", runErr)
		}
	}

	stop() // a second Ctrl+C now kills the process immediately
	eng.Stop()
	notifier.Close()
	waCancel()
	waSvc.Close()
	shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	if err := launcher.RemoveInstance(paths); err != nil {
		log.Printf("remove instance.json: %v", err)
	}
	log.Printf("stopped")
	return runErr
}

// listen picks the port: explicit flag (must succeed) > $DOPPEL_PORT >
// settings.port > the well-known fallbacks > any free port in 7700-9999.
func listen(flagPort, settingsPort int) (net.Listener, error) {
	try := func(p int) (net.Listener, error) {
		return net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
	}
	if flagPort > 0 {
		ln, err := try(flagPort)
		if err != nil {
			return nil, fmt.Errorf("port %d is not available: %w", flagPort, err)
		}
		return ln, nil
	}
	var candidates []int
	if v := os.Getenv("DOPPEL_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			candidates = append(candidates, p)
		}
	}
	if settingsPort > 0 {
		candidates = append(candidates, settingsPort)
	}
	candidates = append(candidates, fallbackPorts...)
	for _, p := range candidates {
		if ln, err := try(p); err == nil {
			return ln, nil
		}
	}
	for _, p := range server.FreePorts(7700, 9999, 5, 0) {
		if ln, err := try(p); err == nil {
			return ln, nil
		}
	}
	return nil, errors.New("no free port found between 7700 and 9999")
}

// legacyCandidates lists old bot session databases to import on first start,
// in priority order (wa.New copies the first one that exists).
func legacyCandidates(opts Options) []string {
	var c []string
	add := func(p string) {
		if p == "" {
			return
		}
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		for _, x := range c {
			if x == p {
				return
			}
		}
		c = append(c, p)
	}
	add(opts.LegacyDB)
	add(os.Getenv("DOPPEL_LEGACY_DB"))
	if wd, err := os.Getwd(); err == nil && wd != "/" {
		add(filepath.Join(wd, "bot.db"))
	}
	if opts.DevProjectDir != "" {
		add(filepath.Join(opts.DevProjectDir, "bot.db"))
	}
	if wd, err := os.Getwd(); err == nil && wd != "/" {
		add(filepath.Join(wd, "whatsapp_session.db"))
	}
	if opts.DevProjectDir != "" {
		add(filepath.Join(opts.DevProjectDir, "whatsapp_session.db"))
	}
	return c
}

// notifyEvents: activity notifications are on for real WhatsApp. With fake
// WhatsApp (demos, smoke and test runs) only "Send a test" shows one, unless
// DOPPEL_NOTIFY=on.
func notifyEvents(opts Options) bool {
	return !opts.FakeWA || os.Getenv("DOPPEL_NOTIFY") == "on"
}

// notifyBackend: DOPPEL_NOTIFY=dry logs notifications instead of showing
// them, DOPPEL_NOTIFY=off turns them off; otherwise the backend is detected.
func notifyBackend() string {
	switch os.Getenv("DOPPEL_NOTIFY") {
	case "dry":
		return notify.BackendDryRun
	case "off":
		return notify.BackendNone
	}
	return ""
}
