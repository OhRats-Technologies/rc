package main

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func processCwd(value string) string {
	home, _ := os.UserHomeDir()
	if value == "" || value == "~" {
		return home
	}
	if strings.HasPrefix(value, "~/") && home != "" {
		return filepath.Join(home, strings.TrimPrefix(value, "~/"))
	}
	return value
}

func processExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return 1
	}
	status, ok := exitError.Sys().(syscall.WaitStatus)
	if ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return exitError.ExitCode()
}

func stopSession(sessionID int, signal syscall.Signal) {
	signalSession(sessionID, signal)
	if signal != syscall.SIGKILL {
		time.Sleep(350 * time.Millisecond)
		signalSession(sessionID, syscall.SIGKILL)
	}
}

func runProcessRunner() int {
	command := os.Getenv("OHRATS_PROCESS_COMMAND")
	if command == "" {
		return 127
	}
	lifeline := os.NewFile(3, "rc-lifeline")
	if lifeline == nil {
		return 127
	}
	defer lifeline.Close()
	sessionID := os.Getpid()
	cmd := exec.Command("sh", "-lc", command)
	if cwd := processCwd(os.Getenv("OHRATS_PROCESS_CWD")); cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = os.Environ()
	if os.Getenv("OHRATS_PROCESS_TERMINAL") == "1" {
		cmd.Env = append(cmd.Env, "COLORTERM=truecolor")
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return 127
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	lost := make(chan struct{}, 1)
	go func() {
		buffer := []byte{0}
		if _, err := lifeline.Read(buffer); err != nil {
			lost <- struct{}{}
		}
	}()
	terminate := make(chan os.Signal, 1)
	signal.Notify(terminate, syscall.SIGTERM)
	defer signal.Stop(terminate)

	select {
	case err := <-done:
		stopSession(sessionID, syscall.SIGTERM)
		return processExitCode(err)
	case <-terminate:
		stopSession(sessionID, syscall.SIGTERM)
		return 143
	case <-lost:
		stopSession(sessionID, syscall.SIGKILL)
		return 137
	}
}
