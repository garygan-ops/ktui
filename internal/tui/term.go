package tui

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"unsafe"
)

type terminalState struct {
	termios syscall.Termios
}

func enterRawMode() (*terminalState, error) {
	fd := int(os.Stdin.Fd())
	old, err := ioctlGetTermios(fd)
	if err != nil {
		return nil, err
	}
	raw := old
	raw.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Cflag |= syscall.CS8
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN | syscall.ISIG
	raw.Cc[syscall.VMIN] = 0
	raw.Cc[syscall.VTIME] = 1
	if err := ioctlSetTermios(fd, raw); err != nil {
		return nil, err
	}
	fmt.Print("\x1b[?1049h\x1b[?25l\x1b[?7l")
	return &terminalState{termios: old}, nil
}

func (s *terminalState) restore() {
	if s == nil {
		return
	}
	_ = ioctlSetTermios(int(os.Stdin.Fd()), s.termios)
	fmt.Print("\x1b[?7h\x1b[?25h\x1b[?1049l\x1b[0m")
}

func installSignalRestore(state *terminalState) func() {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case sig := <-sigCh:
			state.restore()
			signal.Reset(os.Interrupt, syscall.SIGTERM)
			_ = syscall.Kill(syscall.Getpid(), sig.(syscall.Signal))
		case <-done:
		}
	}()
	return func() {
		close(done)
		signal.Stop(sigCh)
	}
}

func installResizeHandler(render func()) func() {
	sigCh := make(chan os.Signal, 4)
	signal.Notify(sigCh, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-sigCh:
				render()
			case <-done:
				return
			}
		}
	}()
	return func() {
		close(done)
		signal.Stop(sigCh)
	}
}

func terminalSize() (int, int) {
	type winsize struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}
	ws := winsize{}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno != 0 || ws.Row == 0 || ws.Col == 0 {
		return 100, 30
	}
	return int(ws.Col), int(ws.Row)
}

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
