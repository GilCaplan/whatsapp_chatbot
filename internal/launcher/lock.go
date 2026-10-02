package launcher

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/platform"
)

// ErrLocked means another `serve` process holds server.lock for this data dir.
var ErrLocked = errors.New("another WhatsApp Doppel server is already running for this data folder")

// Lock is an exclusive lock on <data>/server.lock (flock, or LockFileEx on
// Windows), held by `serve` for its whole lifetime. The OS releases it
// automatically if the process dies. The file itself is never deleted.
type Lock struct {
	f *os.File
}

// AcquireLock takes the lock without blocking; ErrLocked if it is held.
func AcquireLock(p config.Paths) (*Lock, error) {
	f, err := os.OpenFile(p.LockFile(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := platform.TryLockFile(f); err != nil {
		f.Close()
		if errors.Is(err, platform.ErrLockHeld) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("lock %s: %w", p.LockFile(), err)
	}
	// Record our PID for humans (informational only; the flock is what counts).
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	return &Lock{f: f}, nil
}

// Locked reports whether some process currently holds the lock.
func Locked(p config.Paths) bool {
	l, err := AcquireLock(p)
	if err != nil {
		return errors.Is(err, ErrLocked)
	}
	l.Release()
	return false
}

// LockHolderPID returns the PID written by the current holder, if readable.
func LockHolderPID(p config.Paths) int {
	b, err := os.ReadFile(p.LockFile())
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

// Release unlocks and closes the lock file. Safe to call more than once.
func (l *Lock) Release() {
	if l == nil || l.f == nil {
		return
	}
	_ = platform.UnlockFile(l.f)
	l.f.Close()
	l.f = nil
}
