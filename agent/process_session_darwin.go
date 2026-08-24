//go:build darwin

package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func sessionProcessIDs(sessionID int) []int {
	output, err := exec.Command("/bin/ps", "-axo", "pid=").Output()
	if err != nil {
		return nil
	}
	var result []int
	for _, field := range strings.Fields(string(output)) {
		pid, err := strconv.Atoi(field)
		if err != nil || pid == os.Getpid() {
			continue
		}
		session, err := syscall.Getsid(pid)
		if err == nil && session == sessionID {
			result = append(result, pid)
		}
	}
	return result
}
