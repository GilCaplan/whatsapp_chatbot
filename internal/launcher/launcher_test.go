package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"whatsappdoppel/internal/config"
)

// helperEnv turns the test binary into a stand-in server for spawn tests:
// "exit1" exits with status 1 right away (portable /usr/bin/false).
const helperEnv = "DOPPEL_TEST_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) == "exit1" {
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func testPaths(t *testing.T) config.Paths {
	t.Helper()
	p, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInstanceRoundTrip(t *testing.T) {
	p := testPaths(t)
	if _, err := ReadInstance(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing instance err = %v", err)
	}
	in := Instance{PID: 42, Port: 7788, Token: "tok", StartedAt: time.Now().Truncate(time.Second)}
	if err := WriteInstance(p, in); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p.InstanceFile())
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("instance.json perm = %v %v", fi.Mode(), err)
	}
	got, err := ReadInstance(p)
	if err != nil || got.PID != 42 || got.Port != 7788 || got.Token != "tok" || !got.StartedAt.Equal(in.StartedAt) {
		t.Fatalf("read back = %+v %v", got, err)
	}
	if got.URL() != "http://127.0.0.1:7788/" {
		t.Fatalf("URL = %s", got.URL())
	}
	if err := RemoveInstance(p); err != nil {
		t.Fatal(err)
	}
	if err := RemoveInstance(p); err != nil {
		t.Fatalf("second remove: %v", err)
	}
}

func TestLockExclusive(t *testing.T) {
	p := testPaths(t)
	if Locked(p) {
		t.Fatal("fresh dir reported locked")
	}
	l, err := AcquireLock(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLock(p); !errors.Is(err, ErrLocked) {
		t.Fatalf("second acquire = %v, want ErrLocked", err)
	}
	if !Locked(p) {
		t.Fatal("Locked = false while held")
	}
	if LockHolderPID(p) != os.Getpid() {
		t.Fatalf("holder pid = %d", LockHolderPID(p))
	}
	l.Release()
	l.Release() // idempotent
	l2, err := AcquireLock(p)
	if err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	}
	l2.Release()
}

func fakeHealth(t *testing.T, token string) (*httptest.Server, int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "token": token, "version": "t"})
		case "/api/system/quit":
			if r.Header.Get("X-Doppel-Token") != token {
				w.WriteHeader(401)
				return
			}
			w.Write([]byte(`{"ok":true}`))
		}
	}))
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	n, _ := strconv.Atoi(port)
	return srv, n
}

func TestProbeVerifiesToken(t *testing.T) {
	_, port := fakeHealth(t, "right")
	ctx := context.Background()
	if _, ok := Probe(ctx, Instance{Port: port, Token: "right"}); !ok {
		t.Fatal("probe with right token failed")
	}
	if _, ok := Probe(ctx, Instance{Port: port, Token: "wrong"}); ok {
		t.Fatal("probe accepted a foreign server")
	}
	// Nothing listening.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	start := time.Now()
	if _, ok := Probe(ctx, Instance{Port: dead, Token: "x"}); ok {
		t.Fatal("probe on closed port succeeded")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("probe did not time out quickly")
	}
}

func TestLaunchOpensRunningInstance(t *testing.T) {
	p := testPaths(t)
	_, port := fakeHealth(t, "tok")
	if err := WriteInstance(p, Instance{PID: os.Getpid(), Port: port, Token: "tok"}); err != nil {
		t.Fatal(err)
	}
	var opened string
	url, err := Launch(context.Background(), Options{
		DataDir: p.Dir,
		Exe:     filepath.Join(t.TempDir(), "should-not-spawn"),
		Open:    func(u string) error { opened = u; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if opened != url || url != "http://127.0.0.1:"+strconv.Itoa(port)+"/" {
		t.Fatalf("opened %q url %q", opened, url)
	}
}

func TestLaunchReportsEarlyExit(t *testing.T) {
	p := testPaths(t)
	// Stale instance.json pointing at nothing; lock free → spawn; the test
	// binary itself, as a helper (see TestMain), exits with status 1 at once.
	WriteInstance(p, Instance{Port: 1, Token: "stale"})
	t.Setenv(helperEnv, "exit1")
	_, err := Launch(context.Background(), Options{
		DataDir:      p.Dir,
		Exe:          os.Args[0],
		Open:         func(string) error { t.Fatal("must not open"); return nil },
		StartTimeout: 5 * time.Second,
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if _, err := os.Stat(p.InstanceFile()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("stale instance.json not removed")
	}
}

func TestQuit(t *testing.T) {
	p := testPaths(t)
	if err := Quit(context.Background(), p, time.Second); !IsNotRunning(err) {
		t.Fatalf("quit with nothing running = %v", err)
	}
	srv, port := fakeHealth(t, "tok")
	WriteInstance(p, Instance{Port: port, Token: "tok"})
	go func() {
		time.Sleep(300 * time.Millisecond)
		srv.Close()
	}()
	if err := Quit(context.Background(), p, 3*time.Second); err != nil {
		t.Fatal(err)
	}
}
