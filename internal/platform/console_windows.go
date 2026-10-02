package platform

import (
	"log"
	"os"

	"golang.org/x/sys/windows"
)

var procAttachConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")

// attachParentProcess is ATTACH_PARENT_PROCESS ((DWORD)-1).
const attachParentProcess = ^uint32(0)

// AttachParentConsole lets the GUI-subsystem build (-H=windowsgui, no console
// window on double-click) print to the console it was started from, so
// `WhatsappDoppel.exe status` works in cmd or PowerShell. Standard handles
// that already point somewhere (a pipe, a file, a console) are left alone.
// Started from Explorer there is no parent console and nothing changes.
func AttachParentConsole() {
	outOK, errOK := usableStd(windows.STD_OUTPUT_HANDLE), usableStd(windows.STD_ERROR_HANDLE)
	if outOK && errOK {
		return
	}
	if procAttachConsole.Find() != nil {
		return
	}
	if r, _, _ := procAttachConsole.Call(uintptr(attachParentProcess)); r == 0 {
		return
	}
	name, err := windows.UTF16PtrFromString("CONOUT$")
	if err != nil {
		return
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return
	}
	con := os.NewFile(uintptr(h), "CONOUT$")
	if !outOK {
		_ = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, h)
		os.Stdout = con
	}
	if !errOK {
		_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, h)
		os.Stderr = con
		log.SetOutput(os.Stderr)
	}
}

// usableStd reports whether a standard handle points at something real.
func usableStd(std uint32) bool {
	h, err := windows.GetStdHandle(std)
	if err != nil || h == 0 || h == windows.InvalidHandle {
		return false
	}
	t, err := windows.GetFileType(h)
	return err == nil && t != windows.FILE_TYPE_UNKNOWN
}
