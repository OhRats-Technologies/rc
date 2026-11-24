package server

import (
	"sort"
	"sync"
	"time"
)

// ProcessStatus is the monotonic lifecycle state of a hosted remote process.
type ProcessStatus string

const (
	ProcessStarting ProcessStatus = "starting"
	ProcessRunning  ProcessStatus = "running"
	ProcessExited   ProcessStatus = "exited"
	ProcessLost     ProcessStatus = "lost"
)

func (s ProcessStatus) isTerminal() bool {
	return s == ProcessExited || s == ProcessLost
}

func canTransitionProcess(from, to ProcessStatus) bool {
	if from == to {
		return true
	}
	if from.isTerminal() {
		return false
	}
	switch to {
	case ProcessRunning, ProcessExited, ProcessLost:
		return true
	default:
		return false
	}
}

type browserProcess struct {
	ID        string
	DeviceID  string
	UserID    string
	UserName  string
	Origin    string
	Terminal  bool
	Status    ProcessStatus
	ExitCode  *int32
	Signal    string
	Error     string
	CreatedAt time.Time
}

type processHub struct {
	mu      sync.Mutex
	process map[string]browserProcess
}

func newProcessHub() *processHub {
	return &processHub{process: make(map[string]browserProcess)}
}

func (h *processHub) add(process browserProcess) {
	if process.Status == "" {
		process.Status = ProcessStarting
	}
	h.mu.Lock()
	h.process[process.ID] = process
	h.mu.Unlock()
}

func (h *processHub) update(id string, status ProcessStatus, exitCode *int32, signal, message string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	process, ok := h.process[id]
	if !ok || !canTransitionProcess(process.Status, status) {
		return false
	}
	process.Status = status
	process.ExitCode = exitCode
	process.Signal = signal
	process.Error = message
	h.process[id] = process
	return true
}

// reconcile marks any active hosted process for deviceID that is absent from
// activeIDs as lost. Called on every Node process_sync so reconnects never
// leave ghost running processes behind.
func (h *processHub) reconcile(deviceID string, activeIDs []string) []browserProcess {
	active := make(map[string]bool, len(activeIDs))
	for _, id := range activeIDs {
		active[id] = true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	var lost []browserProcess
	for id, process := range h.process {
		if process.DeviceID != deviceID || process.Status.isTerminal() || active[id] {
			continue
		}
		process.Status = ProcessLost
		if process.Error == "" {
			process.Error = "Node reconnected without this process"
		}
		h.process[id] = process
		lost = append(lost, process)
	}
	return lost
}

func (h *processHub) get(id string) (browserProcess, bool) {
	h.mu.Lock()
	process, ok := h.process[id]
	h.mu.Unlock()
	return process, ok
}

func (h *processHub) forDevice(deviceID string) []browserProcess {
	h.mu.Lock()
	processes := make([]browserProcess, 0)
	for _, process := range h.process {
		if process.DeviceID == deviceID {
			processes = append(processes, process)
		}
	}
	h.mu.Unlock()
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].CreatedAt.After(processes[j].CreatedAt)
	})
	return processes
}

func processJSON(process browserProcess) map[string]any {
	return map[string]any{
		"id": process.ID, "device_id": process.DeviceID, "origin": process.Origin,
		"terminal": process.Terminal, "status": process.Status,
		"exit_code": process.ExitCode, "signal": process.Signal, "error": process.Error,
		"created_by": process.UserID, "created_by_name": process.UserName,
		"created_at": process.CreatedAt.UnixMilli(),
	}
}
