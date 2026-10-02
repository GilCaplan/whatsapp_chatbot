package platform

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// DetachAttrs starts cmd without a console and in its own process group, so
// it keeps running after the launcher exits and ignores the terminal's Ctrl+C.
// (DETACHED_PROCESS and CREATE_NO_WINDOW are mutually exclusive.)
func DetachAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
}

// HideWindow stops a short-lived console helper (powershell.exe) from
// flashing a console window.
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}
