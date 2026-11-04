package platform

import (
	"os"

	"golang.org/x/sys/windows"
)

// IsTerminal reports whether f is an interactive console.
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil
}
