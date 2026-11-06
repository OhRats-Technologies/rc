package schedule

import (
	"fmt"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	processTypes "wit_component/ohrats_rc_process_types"
	types "wit_component/ohrats_rc_scheduler_types"
)

// storedDefinition mirrors the Rust serde model byte for byte so durable state
// written before the Go port stays readable.
type storedDefinition struct {
	ID           string            `json:"id"`
	Name         *string           `json:"name"`
	Cron         string            `json:"cron"`
	Timezone     string            `json:"timezone"`
	Mode         storedMode        `json:"mode"`
	Cwd          *string           `json:"cwd"`
	Environment  storedEnvironment `json:"environment"`
	Enabled      bool              `json:"enabled"`
	Overlap      string            `json:"overlap"`
	Misfire      string            `json:"misfire"`
	MaxRuntimeMs *uint64           `json:"maxRuntimeMs"`
	PermitHash   string            `json:"permitHash"`
	CreatedBy    string            `json:"createdBy"`
	CreatedAtMs  uint64            `json:"createdAtMs"`
	ExpiresAtMs  *uint64           `json:"expiresAtMs"`
}

type storedEnvironment struct {
	Clean   bool         `json:"clean"`
	Changes [][2]*string `json:"changes"`
}

type storedCursor struct {
	CheckedAtMs      uint64  `json:"checkedAtMs"`
	LastOccurrenceID *string `json:"lastOccurrenceId"`
}

func definitionFromStored(value storedDefinition) (types.Definition, error) {
	definition := types.Definition{
		Id:           value.ID,
		Name:         optionString(value.Name),
		Cron:         value.Cron,
		Timezone:     value.Timezone,
		Mode:         executionModeFromStored(value.Mode),
		Cwd:          optionString(value.Cwd),
		Environment:  environmentFromStored(value.Environment),
		Enabled:      value.Enabled,
		Overlap:      types.OverlapPolicy_Forbid,
		MaxRuntimeMs: optionUint64(value.MaxRuntimeMs),
		PermitHash:   value.PermitHash,
		CreatedBy:    value.CreatedBy,
		CreatedAtMs:  value.CreatedAtMs,
		ExpiresAtMs:  optionUint64(value.ExpiresAtMs),
	}
	switch value.Overlap {
	case "forbid":
	default:
		return types.Definition{}, fmt.Errorf("invalid stored overlap policy")
	}
	switch value.Misfire {
	case "skip":
		definition.Misfire = types.MisfirePolicy_Skip
	case "run-once":
		definition.Misfire = types.MisfirePolicy_RunOnce
	default:
		return types.Definition{}, fmt.Errorf("invalid stored misfire policy")
	}
	return definition, nil
}

func definitionFromValue(value types.Definition) storedDefinition {
	misfire := "skip"
	if value.Misfire == types.MisfirePolicy_RunOnce {
		misfire = "run-once"
	}
	return storedDefinition{
		ID:           value.Id,
		Name:         stringOption(value.Name),
		Cron:         value.Cron,
		Timezone:     value.Timezone,
		Mode:         storedModeFromExecutionMode(value.Mode),
		Cwd:          stringOption(value.Cwd),
		Environment:  environmentFromValue(value.Environment),
		Enabled:      value.Enabled,
		Overlap:      "forbid",
		Misfire:      misfire,
		MaxRuntimeMs: uint64Option(value.MaxRuntimeMs),
		PermitHash:   value.PermitHash,
		CreatedBy:    value.CreatedBy,
		CreatedAtMs:  value.CreatedAtMs,
		ExpiresAtMs:  uint64Option(value.ExpiresAtMs),
	}
}

func environmentFromValue(value processTypes.Environment) storedEnvironment {
	changes := make([][2]*string, 0, len(value.Changes))
	for index := range value.Changes {
		change := &value.Changes[index]
		name := change.Name
		var filled *string
		if change.Value.IsSome() {
			text := change.Value.Some()
			filled = &text
		}
		changes = append(changes, [2]*string{&name, filled})
	}
	return storedEnvironment{
		Clean:   value.Base == processTypes.EnvironmentBase_Clean,
		Changes: changes,
	}
}

func environmentFromStored(value storedEnvironment) processTypes.Environment {
	environment := processTypes.Environment{
		Base:    processTypes.EnvironmentBase_Inherit,
		Changes: make([]processTypes.EnvironmentChange, 0, len(value.Changes)),
	}
	if value.Clean {
		environment.Base = processTypes.EnvironmentBase_Clean
	}
	for _, pair := range value.Changes {
		change := processTypes.EnvironmentChange{Value: witTypes.None[string]()}
		if pair[0] != nil {
			change.Name = *pair[0]
		}
		if pair[1] != nil {
			change.Value = witTypes.Some(*pair[1])
		}
		environment.Changes = append(environment.Changes, change)
	}
	return environment
}

func optionString(value *string) witTypes.Option[string] {
	if value == nil {
		return witTypes.None[string]()
	}
	return witTypes.Some(*value)
}

func stringOption(value witTypes.Option[string]) *string {
	if value.IsNone() {
		return nil
	}
	text := value.Some()
	return &text
}

func optionUint64(value *uint64) witTypes.Option[uint64] {
	if value == nil {
		return witTypes.None[uint64]()
	}
	return witTypes.Some(*value)
}

func uint64Option(value witTypes.Option[uint64]) *uint64 {
	if value.IsNone() {
		return nil
	}
	number := value.Some()
	return &number
}
