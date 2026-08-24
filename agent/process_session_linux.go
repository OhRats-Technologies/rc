//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func sessionProcessIDs(sessionID int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var result []int
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			continue
		}
		value := string(data)
		end := strings.LastIndexByte(value, ')')
		if end < 0 {
			continue
		}
		fields := strings.Fields(value[end+1:])
		if len(fields) < 4 {
			continue
		}
		session, err := strconv.Atoi(fields[3])
		if err == nil && session == sessionID {
			result = append(result, pid)
		}
	}
	return result
}
