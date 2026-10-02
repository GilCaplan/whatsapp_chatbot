// Package platform hides the differences between macOS, Linux and Windows:
// where the data folder lives, terminal detection, opening URLs and folders,
// the server's exclusive file lock, detached and window-less child
// processes, native error alerts and (Windows GUI build) attaching to the
// parent console. It is a leaf: stdlib + golang.org/x/sys only.
package platform

import (
	"errors"
	"runtime"
)

// AppDirName is the data folder's name inside the per-user config folder.
const AppDirName = "WhatsappDoppel"

// ErrLockHeld means another process (or another handle) holds the file lock.
var ErrLockHeld = errors.New("file is locked by another process")

// Name is the operating system as Go names it: "darwin", "linux", "windows"…
func Name() string { return runtime.GOOS }
