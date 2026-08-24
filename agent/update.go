package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

func replaceExecutable(serverURL string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	resp, err := http.Get(strings.TrimRight(serverURL, "/") + "/downloads/ohrats-relay-" + runtime.GOOS + "-" + runtime.GOARCH)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: %s", resp.Status)
	}
	temp, err := os.CreateTemp(filepath.Dir(executable), ".ohrats-relay-update-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err = io.Copy(temp, io.LimitReader(resp.Body, 100<<20)); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(name, 0755); err != nil {
		return err
	}
	output, err := exec.Command(name, "version").CombinedOutput()
	if err != nil || !strings.HasPrefix(string(output), "OhRats Relay Node ") {
		return fmt.Errorf("downloaded file is not an OhRats Relay Node")
	}
	return os.Rename(name, executable)
}

func syscallExecCurrent() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(executable, os.Args, os.Environ())
}
