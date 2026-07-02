//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package main

import (
	"bufio"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func readSecretFromTerminal(file *os.File) (string, error) {
	fd := int(file.Fd())
	oldState, err := getTerminalState(fd)
	if err != nil {
		return "", err
	}
	secretState := oldState
	secretState.Lflag &^= syscall.ECHO
	if err := setTerminalState(fd, secretState); err != nil {
		return "", err
	}
	stopSignals := installSecretSignalRestore(fd, oldState)
	defer stopSignals()
	defer func() {
		_ = setTerminalState(fd, oldState)
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

func installSecretSignalRestore(fd int, state syscall.Termios) func() {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case sig := <-sigCh:
			_ = setTerminalState(fd, state)
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
