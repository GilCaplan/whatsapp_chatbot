//go:build !darwin && !windows

package platform

import (
	"errors"
	"os/exec"
)

// OpenURL opens url in the default browser (xdg-open).
func OpenURL(url string) error { return xdgOpen(url) }

// OpenFolder shows dir in the file manager (xdg-open).
func OpenFolder(dir string) error { return xdgOpen(dir) }

// xdgOpen starts xdg-open without waiting for it: some desktops keep it
// running until the browser exits. Output is discarded.
func xdgOpen(target string) error {
	bin, err := exec.LookPath("xdg-open")
	if err != nil {
		return errors.New("xdg-open was not found (install xdg-utils)")
	}
	cmd := exec.Command(bin, target)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
