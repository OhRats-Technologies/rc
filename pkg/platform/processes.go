package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Process is one live process in the local RC service tree.
type Process struct {
	PID       int      `json:"pid"`
	Name      string   `json:"name"`
	Argv      []string `json:"argv,omitempty"`
	Truncated bool     `json:"truncated"`
}

// Processes lists the RC service main process and its descendants. Command
// lines are live OS state and must not leave the machine.
func Processes(commands bool) ([]Process, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("rc ps currently requires Linux/systemd")
	}
	roots, err := serviceRoots()
	if err != nil {
		return nil, err
	}
	if len(roots) == 0 {
		return nil, nil
	}
	parents, names, err := processTable()
	if err != nil {
		return nil, err
	}
	selected := make(map[int]bool)
	for _, root := range roots {
		selected[root] = true
	}
	changed := true
	for changed {
		changed = false
		for pid, parent := range parents {
			if selected[parent] && !selected[pid] {
				selected[pid] = true
				changed = true
			}
		}
	}
	result := make([]Process, 0, len(selected))
	for pid := range selected {
		process := Process{PID: pid, Name: names[pid]}
		if commands {
			argv, truncated := readArgv(pid)
			process.Argv = argv
			process.Truncated = truncated
		}
		result = append(result, process)
	}
	sortProcesses(result)
	return result, nil
}

func serviceRoots() ([]int, error) {
	var roots []int
	if _, err := exec.LookPath("systemctl"); err == nil {
		output, err := exec.Command("systemctl", "--user", "show", "rc.service", "--property", "MainPID", "--value").Output()
		if err == nil {
			if pid, convErr := strconv.Atoi(strings.TrimSpace(string(output))); convErr == nil && pid > 0 {
				roots = append(roots, pid)
			}
		}
	}
	if len(roots) > 0 {
		return roots, nil
	}
	// Fall back to a local `rc run` process owned by this user.
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		argv, _ := readArgv(pid)
		if len(argv) >= 2 && strings.HasSuffix(argv[0], "rc") && argv[1] == "run" {
			roots = append(roots, pid)
		}
	}
	return roots, nil
}

func processTable() (map[int]int, map[int]string, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, nil, err
	}
	parents := make(map[int]int)
	names := make(map[int]string)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + entry.Name() + "/stat")
		if err != nil {
			continue
		}
		// PID (comm) state ppid ...
		text := string(data)
		open := strings.IndexByte(text, '(')
		close := strings.LastIndexByte(text, ')')
		if open < 0 || close < open {
			continue
		}
		names[pid] = text[open+1 : close]
		rest := strings.Fields(text[close+1:])
		if len(rest) < 2 {
			continue
		}
		parent, err := strconv.Atoi(rest[1])
		if err == nil {
			parents[pid] = parent
		}
	}
	return parents, names, nil
}

func readArgv(pid int) ([]string, bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || len(data) == 0 {
		return nil, false
	}
	parts := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
	truncated := false
	total := 0
	for i, part := range parts {
		total += len(part) + 1
		if total > 4096 {
			parts = parts[:i]
			truncated = true
			break
		}
	}
	return parts, truncated
}

func sortProcesses(processes []Process) {
	for i := 1; i < len(processes); i++ {
		for j := i; j > 0 && processes[j].PID < processes[j-1].PID; j-- {
			processes[j], processes[j-1] = processes[j-1], processes[j]
		}
	}
}
