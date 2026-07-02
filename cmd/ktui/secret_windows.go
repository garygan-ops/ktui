//go:build windows

package main

import (
	"bufio"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"unsafe"
)

const secretEnableEchoInput = 0x0004

var (
	secretKernel32           = syscall.NewLazyDLL("kernel32.dll")
	secretProcGetConsoleMode = secretKernel32.NewProc("GetConsoleMode")
	secretProcSetConsoleMode = secretKernel32.NewProc("SetConsoleMode")
)

func readSecretFromTerminal(file *os.File) (string, error) {
	handle := syscall.Handle(file.Fd())
	oldMode, err := secretGetConsoleMode(handle)
	if err != nil {
		return "", err
	}
	secretMode := oldMode &^ secretEnableEchoInput
	if err := secretSetConsoleMode(handle, secretMode); err != nil {
		return "", err
	}
	stopSignals := installSecretSignalRestore(handle, oldMode)
	defer stopSignals()
	defer func() {
		_ = secretSetConsoleMode(handle, oldMode)
	}()

	value, err := bufio.NewReader(file).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if err == io.EOF && value == "" {
		return "", io.EOF
	}
	return strings.TrimRight(value, "\r\n"), nil
}

func installSecretSignalRestore(handle syscall.Handle, mode uint32) func() {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt)
	done := make(chan struct{})
	go func() {
		select {
		case <-sigCh:
			_ = secretSetConsoleMode(handle, mode)
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

func secretGetConsoleMode(handle syscall.Handle) (uint32, error) {
	var mode uint32
	r1, _, err := secretProcGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	if r1 == 0 {
		if err != syscall.Errno(0) {
			return 0, err
		}
		return 0, syscall.EINVAL
	}
	return mode, nil
}

func secretSetConsoleMode(handle syscall.Handle, mode uint32) error {
	r1, _, err := secretProcSetConsoleMode.Call(uintptr(handle), uintptr(mode))
	if r1 == 0 {
		if err != syscall.Errno(0) {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}
