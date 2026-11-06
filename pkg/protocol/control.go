package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type TerminalSpec struct {
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
	Term string `json:"term,omitempty"`
}

type ExecutionModeKind string

const (
	ExecutionModeArgv             ExecutionModeKind = "argv"
	ExecutionModeRcShell          ExecutionModeKind = "rcShell"
	ExecutionModeSystemShell      ExecutionModeKind = "systemShell"
	ExecutionModeSystemLoginShell ExecutionModeKind = "systemLoginShell"
)

type ExecutionMode struct {
	Kind    ExecutionModeKind `json:"kind"`
	Program string            `json:"program,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Script  string            `json:"script,omitempty"`
	Command string            `json:"command,omitempty"`
}

type EnvironmentBase string

const (
	EnvironmentBaseInherit EnvironmentBase = "inherit"
	EnvironmentBaseClean   EnvironmentBase = "clean"
)

type EnvironmentChange struct {
	Name  string  `json:"name"`
	Value *string `json:"value"`
}

type EnvironmentSpec struct {
	Base    EnvironmentBase     `json:"base"`
	Changes []EnvironmentChange `json:"changes,omitempty"`
}

type ScheduleMisfirePolicy string

const (
	ScheduleMisfireSkip    ScheduleMisfirePolicy = "skip"
	ScheduleMisfireRunOnce ScheduleMisfirePolicy = "runOnce"
)

type ScheduleDefinition struct {
	ID           string                `json:"id"`
	Name         *string               `json:"name,omitempty"`
	Cron         string                `json:"cron"`
	Timezone     string                `json:"timezone"`
	Mode         ExecutionMode         `json:"mode"`
	Cwd          *string               `json:"cwd,omitempty"`
	Environment  EnvironmentSpec       `json:"environment,omitempty"`
	Enabled      bool                  `json:"enabled"`
	Misfire      ScheduleMisfirePolicy `json:"misfire"`
	MaxRuntimeMs *uint64               `json:"maxRuntimeMs,omitempty"`
	PermitHash   string                `json:"permitHash"`
	CreatedBy    string                `json:"createdBy"`
	CreatedAtMs  uint64                `json:"createdAtMs"`
	ExpiresAtMs  *uint64               `json:"expiresAtMs,omitempty"`
}

// ScheduleSpecHash computes the deterministic SHA-256 hash of immutable schedule fields.
func ScheduleSpecHash(schedule *ScheduleDefinition) string {
	type immutableSpec struct {
		ID           string                `json:"id"`
		Cron         string                `json:"cron"`
		Timezone     string                `json:"timezone"`
		Mode         ExecutionMode         `json:"mode"`
		Cwd          *string               `json:"cwd"`
		Environment  EnvironmentSpec       `json:"environment"`
		Misfire      ScheduleMisfirePolicy `json:"misfire"`
		MaxRuntimeMs *uint64               `json:"maxRuntimeMs"`
		CreatedBy    string                `json:"createdBy"`
		ExpiresAtMs  *uint64               `json:"expiresAtMs"`
	}

	raw, _ := json.Marshal(immutableSpec{
		ID:           schedule.ID,
		Cron:         schedule.Cron,
		Timezone:     schedule.Timezone,
		Mode:         schedule.Mode,
		Cwd:          schedule.Cwd,
		Environment:  schedule.Environment,
		Misfire:      schedule.Misfire,
		MaxRuntimeMs: schedule.MaxRuntimeMs,
		CreatedBy:    schedule.CreatedBy,
		ExpiresAtMs:  schedule.ExpiresAtMs,
	})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

type ControlTransportMessage struct {
	Type       string `json:"type"` // "control.frame"
	SessionID  string `json:"sessionId"`
	Sequence   uint64 `json:"sequence"`
	Ciphertext string `json:"ciphertext"`
}
