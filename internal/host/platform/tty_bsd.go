//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package platform

import "golang.org/x/sys/unix"

const ioctlReadTermios = unix.TIOCGETA
