package platform

import "os/exec"

// OpenURL opens url in the default browser.
func OpenURL(url string) error { return exec.Command("open", url).Run() }

// OpenFolder shows dir in Finder.
func OpenFolder(dir string) error { return exec.Command("open", dir).Run() }
