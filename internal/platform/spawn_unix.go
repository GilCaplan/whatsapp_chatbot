//go:build !windows

package platform

import (
	"os/exec"
	"syscall"
)

// DetachAttrs makes cmd a background process in its own session, so it
// survives the launcher and the terminal that started it.
func DetachAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// HideWindow is a no-op outside Windows (helper commands have no window).
func HideWindow(cmd *exec.Cmd) {}
