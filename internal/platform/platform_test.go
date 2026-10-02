package platform

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTryLockFileExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.lock")
	open := func() *os.File {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		return f
	}
	a, b := open(), open()
	if err := TryLockFile(a); err != nil {
		t.Fatalf("first lock: %v", err)
	}
	// The holder can still write its PID, and anyone can read it.
	if _, err := a.WriteAt([]byte("123\n"), 0); err != nil {
		t.Fatalf("write while locked: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "123\n" {
		t.Fatalf("read while locked = %q, %v", got, err)
	}
	if err := TryLockFile(b); !errors.Is(err, ErrLockHeld) {
		t.Fatalf("second lock = %v, want ErrLockHeld", err)
	}
	if err := UnlockFile(a); err != nil {
		t.Fatal(err)
	}
	if err := TryLockFile(b); err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	_ = UnlockFile(b)
}

func TestIsTerminalPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if IsTerminal(r) || IsTerminal(w) {
		t.Error("a pipe is not a terminal")
	}
	if IsTerminal(nil) {
		t.Error("nil is not a terminal")
	}
}

func TestDefaultDataDir(t *testing.T) {
	d, err := DefaultDataDir()
	if err != nil {
		t.Skipf("no config dir here: %v", err)
	}
	if filepath.Base(d) != AppDirName || !filepath.IsAbs(d) {
		t.Errorf("DefaultDataDir = %q", d)
	}
	if runtime.GOOS == "darwin" && filepath.Base(filepath.Dir(d)) != "Application Support" {
		t.Errorf("macOS data dir moved: %q", d)
	}
}

func TestProcessAttrs(t *testing.T) {
	cmd := exec.Command("x")
	DetachAttrs(cmd)
	if cmd.SysProcAttr == nil {
		t.Error("DetachAttrs set nothing")
	}
	HideWindow(exec.Command("x")) // must not panic
	if Name() != runtime.GOOS {
		t.Error("Name")
	}
}
