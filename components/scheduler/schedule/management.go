package schedule

import (
	"errors"

	clockHost "wit_component/ohrats_rc_process_clock_host"
	types "wit_component/ohrats_rc_scheduler_types"
)

// ValidateDefinition enforces the authority metadata and cron/timezone
// requirements shared by upsert and activation.
func ValidateDefinition(value types.Definition) error {
	if value.PermitHash == "" || value.CreatedBy == "" {
		return errors.New("schedule authority metadata is required")
	}
	_, err := NextOccurrence(
		value.Id,
		value.Cron,
		value.Timezone,
		clockHost.NowUnixMs(),
		nil,
	)
	return err
}

// Upsert validates and stores a definition.
func Upsert(value types.Definition) error {
	if err := ValidateDefinition(value); err != nil {
		return err
	}
	return Put(value)
}

// SetEnabled flips a schedule's enabled flag, reporting whether it exists.
func SetEnabled(id string, enabled bool) (bool, error) {
	value, exists, err := Get(id)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	value.Enabled = enabled
	if err := Put(value); err != nil {
		return false, err
	}
	return true, nil
}
