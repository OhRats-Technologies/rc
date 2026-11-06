package protocol

import (
	"encoding/json"
	"fmt"
)

type ControlMessage struct {
	Type        string               `json:"type"`
	ID          string               `json:"id,omitempty"`
	RequestID   string               `json:"requestId,omitempty"`
	Mode        *ExecutionMode       `json:"mode,omitempty"`
	Cwd         *string              `json:"cwd,omitempty"`
	Environment *EnvironmentSpec     `json:"environment,omitempty"`
	Terminal    *TerminalSpec        `json:"terminal,omitempty"`
	Data        string               `json:"data,omitempty"`
	Cols        uint16               `json:"cols,omitempty"`
	Rows        uint16               `json:"rows,omitempty"`
	Signal      string               `json:"signal,omitempty"`
	ExitCode    *int32               `json:"exitCode,omitempty"`
	Error       string               `json:"error,omitempty"`
	Output      string               `json:"output,omitempty"`
	Enabled     *bool                `json:"enabled,omitempty"`
	Schedule    *ScheduleDefinition  `json:"schedule,omitempty"`
	Schedules   []ScheduleDefinition `json:"schedules,omitempty"`
}

// ControlMessage constants for Type field
const (
	TypeProcessStart       = "process.start"
	TypeProcessAttach      = "process.attach"
	TypeProcessStdin       = "process.stdin"
	TypeProcessStdinClose  = "process.stdin.close"
	TypeProcessResize      = "process.resize"
	TypeProcessSignal      = "process.signal"
	TypeProcessStdout      = "process.stdout"
	TypeProcessStderr      = "process.stderr"
	TypeProcessStarted     = "process.started"
	TypeProcessExit        = "process.exit"
	TypeScheduleList       = "schedule.list"
	TypeScheduleUpsert     = "schedule.upsert"
	TypeScheduleRemove     = "schedule.remove"
	TypeScheduleSetEnabled = "schedule.setEnabled"
	TypeScheduleResult     = "schedule.result"
	TypeNodeUpdate         = "node.update"
	TypeControlResult      = "control.result"
	TypeControlRevoked     = "control.revoked"
)

func (m *ControlMessage) Validate() error {
	if m.Type == "" {
		return fmt.Errorf("control message missing type")
	}
	return nil
}

func (m *ControlMessage) MarshalJSON() ([]byte, error) {
	type Alias ControlMessage
	return json.Marshal((*Alias)(m))
}

func (m *ControlMessage) UnmarshalJSON(data []byte) error {
	type Alias ControlMessage
	aux := (*Alias)(m)
	return json.Unmarshal(data, aux)
}
