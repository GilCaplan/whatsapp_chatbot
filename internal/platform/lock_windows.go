package platform

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// The locked range is one byte far beyond the end of the file: Windows range
// locks are mandatory, so locking the PID text itself would stop other
// processes from reading it.
const (
	lockOffsetLow  = 0xFFFFFFFF
	lockOffsetHigh = 0x7FFFFFFF
)

// TryLockFile takes an exclusive lock on f without blocking; ErrLockHeld if
// another handle (in any process) holds it. Windows drops the lock when the
// process dies.
func TryLockFile(f *os.File) error {
	ol := &windows.Overlapped{Offset: lockOffsetLow, OffsetHigh: lockOffsetHigh}
	err := windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if err != nil {
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
			return ErrLockHeld
		}
		return err
	}
	return nil
}

// UnlockFile releases a lock taken with TryLockFile.
func UnlockFile(f *os.File) error {
	ol := &windows.Overlapped{Offset: lockOffsetLow, OffsetHigh: lockOffsetHigh}
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
}
