package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

func shellCommand(args []string) error {
	flags, server, token, err := accountFlags("rc shell", args)
	if err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: rc shell [--token TOKEN] DEVICE")
	}
	device, err := resolveAccountDevice(*server, *token, flags.Arg(0))
	if err != nil {
		return err
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("shell requires an interactive terminal")
	}

	cols, rows, _ := term.GetSize(int(os.Stdin.Fd()))
	if cols < 2 {
		cols = 80
	}
	if rows < 2 {
		rows = 24
	}
	processID, err := startAccountProcess(*server, *token, device.ID, cols, rows)
	if err != nil {
		return err
	}
	control, err := openRemoteControl(*server, *token, device)
	if err != nil {
		return err
	}
	defer control.close()

	old, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer term.Restore(int(os.Stdin.Fd()), old)
	if err := control.send(wireMessage{Type: "process.start", ID: processID,
		Command: `exec "${SHELL:-sh}" -l`, Cols: cols, Rows: rows}); err != nil {
		return err
	}

	done := make(chan error, 1)
	go readEncryptedShell(control, processID, done)
	go forwardEncryptedShellInput(control, processID)
	resize := make(chan os.Signal, 1)
	signal.Notify(resize, syscall.SIGWINCH)
	defer signal.Stop(resize)
	for {
		select {
		case err := <-done:
			return err
		case <-resize:
			cols, rows, _ := term.GetSize(int(os.Stdin.Fd()))
			if cols >= 2 && rows >= 2 {
				_ = control.send(wireMessage{Type: "process.resize", ID: processID, Cols: cols, Rows: rows})
			}
		}
	}
}

func readEncryptedShell(control *remoteControl, processID string, done chan<- error) {
	for {
		message, err := control.read()
		if err != nil {
			done <- err
			return
		}
		if message.ID != processID {
			continue
		}
		switch message.Type {
		case "process.output":
			fmt.Print(message.Output)
		case "process.exit":
			if message.ExitCode != nil && *message.ExitCode != 0 {
				done <- fmt.Errorf("process exited %d", *message.ExitCode)
			} else {
				done <- nil
			}
			return
		}
	}
}

func forwardEncryptedShellInput(control *remoteControl, processID string) {
	reader := bufio.NewReader(os.Stdin)
	buffer := make([]byte, 4096)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			_ = control.send(wireMessage{Type: "process.input", ID: processID, Input: string(buffer[:n])})
		}
		if err != nil {
			return
		}
	}
}
