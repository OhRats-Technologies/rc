//go:build windows

// Command windows-service-child is the native fixture used by
// scripts/windows-service-host-check.ps1. The service host must launch it
// without a console window, capture both output streams, preserve its exit
// status, and terminate its descendant when the host stops.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const createNoWindow = 0x08000000

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	getConsoleWindow = kernel32.NewProc("GetConsoleWindow")
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: windows-service-child <exit|stop|terminate|recover> <info-file>")
		os.Exit(2)
	}
	mode := os.Args[1]
	if mode == "child" {
		for {
			time.Sleep(time.Second)
		}
	}
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: windows-service-child <exit|stop|terminate|recover> <info-file>")
		os.Exit(2)
	}
	console, _, _ := getConsoleWindow.Call()

	child := exec.Command(os.Args[0], "child")
	child.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
	child.Stdin, child.Stdout, child.Stderr = nil, nil, nil
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "start child:", err)
		os.Exit(2)
	}
	fmt.Println("fixture stdout ✓")
	fmt.Fprintln(os.Stderr, "fixture stderr ✓")
	record := fmt.Sprintf("%d %d %d", os.Getpid(), console, child.Process.Pid)
	if err := os.WriteFile(os.Args[2], []byte(record), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "write info:", err)
		os.Exit(2)
	}
	if mode == "exit" {
		os.Exit(17)
	}
	if err := child.Wait(); err != nil {
		fmt.Fprintln(os.Stderr, "wait child:", err)
	}
}
