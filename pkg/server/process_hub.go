package server

import (
	"sort"
	"sync"
	"time"
)

type browserProcess struct {
	ID        string
	DeviceID  string
	UserID    string
	UserName  string
	Origin    string
	Terminal  bool
	Status    string
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
	h.mu.Lock()
	h.process[process.ID] = process
	h.mu.Unlock()
}

func (h *processHub) update(id, status string, exitCode *int32, signal, message string) {
	h.mu.Lock()
	process, ok := h.process[id]
	if ok {
		process.Status = status
		process.ExitCode = exitCode
		process.Signal = signal
		process.Error = message
		h.process[id] = process
	}
	h.mu.Unlock()
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
