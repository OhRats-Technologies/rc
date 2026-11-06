package export_wit_world

import (
	"errors"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_events_append"
	"wit_component/ohrats_rc_events_types"
)

func verifyIdempotency(workspace string) error {
	retained := workspaceEvent(
		ohrats_rc_events_types.LifecycleKind_WorkspaceRenamed,
		workspace,
		"retained",
		witTypes.Some("retained-retry"),
	)
	first, err := appendEvent(retained)
	if err != nil {
		return err
	}
	replay, err := appendEvent(retained)
	if err != nil {
		return err
	}
	if replay.Cursor != first.Cursor {
		return errors.New("retained idempotency did not replay its event")
	}
	conflict := workspaceEvent(
		ohrats_rc_events_types.LifecycleKind_WorkspaceRenamed,
		workspace,
		"conflict",
		witTypes.Some("retained-retry"),
	)
	if ohrats_rc_events_append.Append(conflict).IsOk() {
		return errors.New("retained idempotency accepted a different event")
	}
	newest, err := appendEvent(workspaceEvent(
		ohrats_rc_events_types.LifecycleKind_WorkspaceRenamed,
		workspace,
		"newest",
		witTypes.Some("newest-retry"),
	))
	if err != nil {
		return err
	}
	finalEvent, err := appendEvent(deviceEvent(
		ohrats_rc_events_types.LifecycleKind_DeviceOnline,
		workspace,
		"device-retention",
	))
	if err != nil {
		return err
	}
	retryRequest := workspaceEvent(
		ohrats_rc_events_types.LifecycleKind_WorkspaceRenamed,
		workspace,
		"newest",
		witTypes.Some("newest-retry"),
	)
	retry, err := appendEvent(retryRequest)
	if err != nil {
		return err
	}
	if retry.Cursor != newest.Cursor || finalEvent.Cursor <= newest.Cursor {
		return errors.New("retained idempotency replay failed after pruning")
	}
	return nil
}
