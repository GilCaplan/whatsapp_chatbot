package launcher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/platform"
)

// Options configures Launch.
type Options struct {
	// DataDir is passed to the spawned server (empty = default data dir).
	DataDir string
	// Exe is the binary to spawn as `<Exe> serve --from-launcher ...`; default os.Executable().
	Exe string
	// ExtraArgs are appended to the serve command line (e.g. --legacy-db F).
	ExtraArgs []string
	// Open opens a URL in the browser; default OpenBrowser. Nil-safe.
	Open func(url string) error
	// StartTimeout bounds how long to wait for the new server to become healthy (default 30 s).
	StartTimeout time.Duration
	// Logf receives progress messages (default: discard).
	Logf func(format string, args ...any)
}

// Launch implements the default (double-click) action: if a healthy server is
// already running for the data dir, open the browser on it; otherwise spawn a
// detached `serve` process, wait until it is healthy and then open the browser.
// It returns the URL that was opened.
func Launch(ctx context.Context, opts Options) (string, error) {
	logf := opts.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	open := opts.Open
	if open == nil {
		open = OpenBrowser
	}
	timeout := opts.StartTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	paths, err := config.NewPaths(opts.DataDir)
	if err != nil {
		return "", fmt.Errorf("data folder: %w", err)
	}

	if inst, ok := Running(ctx, paths); ok {
		logf("already running at %s", inst.URL())
		return inst.URL(), open(inst.URL())
	}

	// Nothing healthy answered. If the lock is free, any instance.json is stale.
	var exited chan error
	if !Locked(paths) {
		_ = RemoveInstance(paths)
		exited, err = spawn(paths, opts)
		if err != nil {
			return "", err
		}
		logf("started server (log: %s)", paths.ServerLog())
	} else {
		// A server holds the lock but isn't answering yet (still starting up).
		logf("a server is starting; waiting for it")
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if inst, ok := Running(ctx, paths); ok {
			return inst.URL(), open(inst.URL())
		}
		select {
		case err := <-exited:
			exited = nil
			// Lost a race with another launcher? Then its server holds the lock: keep waiting for it.
			if !Locked(paths) {
				return "", fmt.Errorf("the server stopped during startup (%v)%s", err, logTail(paths.ServerLog()))
			}
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return "", fmt.Errorf("the server did not become ready within %s%s", timeout, logTail(paths.ServerLog()))
}

// spawn starts `<exe> serve --from-launcher --data-dir <dir>` detached from
// the launcher (own session; on Windows no console, own process group), with output appended to logs/server.log.
// The returned channel receives the exit status if it dies early.
func spawn(paths config.Paths, opts Options) (chan error, error) {
	exe := opts.Exe
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return nil, err
		}
	}
	logFile, err := os.OpenFile(paths.ServerLog(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open server log: %w", err)
	}
	defer logFile.Close() // the child has its own copy of the descriptor
	fmt.Fprintf(logFile, "\n=== launcher %s: starting server ===\n", time.Now().Format(time.RFC3339))

	args := append([]string{"serve", "--from-launcher", "--data-dir", paths.Dir}, opts.ExtraArgs...)
	cmd := exec.Command(exe, args...)
	cmd.Dir = paths.Dir
	cmd.Stdin = nil // the null device
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	platform.DetachAttrs(cmd)
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start server: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	return exited, nil
}

// OpenBrowser opens url in the default browser.
func OpenBrowser(url string) error { return platform.OpenURL(url) }

func logTail(path string) string {
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return ""
	}
	if len(b) > 1500 {
		b = b[len(b)-1500:]
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	return "\n\nLast lines of " + path + ":\n" + strings.TrimSpace(string(b))
}

// IsNotRunning reports whether err means no server is running.
func IsNotRunning(err error) bool { return errors.Is(err, ErrNotRunning) }
