//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package platform

import (
	"os"

	"golang.org/x/sys/unix"
)

// IsTerminal reports whether f is an interactive terminal.
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	_, err := unix.IoctlGetTermios(int(f.Fd()), ioctlReadTermios)
	return err == nil
}
