//go:build darwin || freebsd || openbsd || netbsd || dragonfly

package tui

import (
	"syscall"
	"unsafe"
)

func ioctlGetTermios(fd int) (syscall.Termios, error) {
	var termios syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCGETA), uintptr(unsafe.Pointer(&termios)))
	if errno != 0 {
		return termios, errno
	}
	return termios, nil
}

func ioctlSetTermios(fd int, termios syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCSETA), uintptr(unsafe.Pointer(&termios)))
	if errno != 0 {
		return errno
	}
	return nil
}
