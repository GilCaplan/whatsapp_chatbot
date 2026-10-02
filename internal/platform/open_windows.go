package platform

import "golang.org/x/sys/windows"

// OpenURL opens url in the default browser.
func OpenURL(url string) error { return shellOpen(url) }

// OpenFolder shows dir in File Explorer.
func OpenFolder(dir string) error { return shellOpen(dir) }

// shellOpen hands target to the shell's "open" verb: no command line, so no
// quoting issues with & or spaces.
func shellOpen(target string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}
