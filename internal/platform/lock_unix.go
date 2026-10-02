//go:build !windows

package platform

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// TryLockFile takes an exclusive advisory lock on f without blocking;
// ErrLockHeld if another descriptor holds it. The kernel drops the lock when
// the process dies.
func TryLockFile(f *os.File) error {
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			return ErrLockHeld
		}
		return err
	}
	return nil
}

// UnlockFile releases a lock taken with TryLockFile.
func UnlockFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }
