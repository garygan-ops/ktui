//go:build windows

package tui

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"unsafe"
)

const (
	enableProcessedInput            = 0x0001
	enableLineInput                 = 0x0002
	enableEchoInput                 = 0x0004
	enableWindowInput               = 0x0008
	enableVirtualTerminalProcessing = 0x0004
)

var (
	kernel32                   = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode         = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode         = kernel32.NewProc("SetConsoleMode")
	procGetConsoleScreenBuffer = kernel32.NewProc("GetConsoleScreenBufferInfo")
)

type terminalState struct {
	input      syscall.Handle
	output     syscall.Handle
	inputMode  uint32
	outputMode uint32
}

func enterRawMode() (*terminalState, error) {
	input := syscall.Handle(os.Stdin.Fd())
	output := syscall.Handle(os.Stdout.Fd())

	inputMode, err := getConsoleMode(input)
	if err != nil {
		return nil, err
	}
	outputMode, err := getConsoleMode(output)
	if err != nil {
		return nil, err
	}

	rawInput := inputMode
	rawInput &^= enableEchoInput | enableLineInput | enableProcessedInput
	rawInput |= enableWindowInput

	rawOutput := outputMode | enableVirtualTerminalProcessing

	if err := setConsoleMode(input, rawInput); err != nil {
		return nil, err
	}
	if err := setConsoleMode(output, rawOutput); err != nil {
		_ = setConsoleMode(input, inputMode)
		return nil, err
	}

	fmt.Print("\x1b[?1049h\x1b[?25l\x1b[?7l\x1b[?1000h\x1b[?1006h")
	return &terminalState{
		input:      input,
		output:     output,
		inputMode:  inputMode,
		outputMode: outputMode,
	}, nil
}

func (s *terminalState) restore() {
	if s == nil {
		return
	}
	_ = setConsoleMode(s.input, s.inputMode)
	_ = setConsoleMode(s.output, s.outputMode)
	fmt.Print("\x1b[?1006l\x1b[?1000l\x1b[?7h\x1b[?25h\x1b[?1049l\x1b[0m")
}

func installSignalRestore(state *terminalState) func() {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt)
	done := make(chan struct{})
	go func() {
		select {
		case <-sigCh:
			state.restore()
			signal.Reset(os.Interrupt)
			os.Exit(130)
		case <-done:
		}
	}()
	return func() {
		close(done)
		signal.Stop(sigCh)
	}
}

func installResizeHandler(render func()) func() {
	return func() {}
}

func terminalSize() (int, int) {
	type coord struct {
		X int16
		Y int16
	}
	type smallRect struct {
		Left   int16
		Top    int16
		Right  int16
		Bottom int16
	}
	type consoleScreenBufferInfo struct {
		Size              coord
		CursorPosition    coord
		Attributes        uint16
		Window            smallRect
		MaximumWindowSize coord
	}

	var info consoleScreenBufferInfo
	r1, _, _ := procGetConsoleScreenBuffer.Call(os.Stdout.Fd(), uintptr(unsafe.Pointer(&info)))
	if r1 == 0 {
		return 100, 30
	}

	width := int(info.Window.Right - info.Window.Left + 1)
	height := int(info.Window.Bottom - info.Window.Top + 1)
	if width <= 0 || height <= 0 {
		return 100, 30
	}
	return width, height
}

func getConsoleMode(handle syscall.Handle) (uint32, error) {
	var mode uint32
	r1, _, err := procGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	if r1 == 0 {
		if err != syscall.Errno(0) {
			return 0, err
		}
		return 0, syscall.EINVAL
	}
	return mode, nil
}

func setConsoleMode(handle syscall.Handle, mode uint32) error {
	r1, _, err := procSetConsoleMode.Call(uintptr(handle), uintptr(mode))
	if r1 == 0 {
		if err != syscall.Errno(0) {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}
