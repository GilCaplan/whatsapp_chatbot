//go:build !windows

package platform

// AttachParentConsole is a no-op outside Windows.
func AttachParentConsole() {}
