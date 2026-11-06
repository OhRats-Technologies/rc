package export_wit_world

import (
	"errors"
	"fmt"
	"strings"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_events_append"
	"wit_component/ohrats_rc_events_retention"
	"wit_component/ohrats_rc_events_types"
)

func seed(args []string) (uint32, error) {
	if len(args) != 1 {
		return 0, errors.New("usage: rc events-seed <fixture>")
	}
	fixture := args[0]
	if err := validFixture(fixture); err != nil {
		return 0, err
	}
	if err := configureRetention(8); err != nil {
		return 0, err
	}
	workspace := "workspace-" + fixture
	created := workspaceEvent(
		ohrats_rc_events_types.LifecycleKind_WorkspaceCreated,
		workspace,
		"created",
		witTypes.Some("retry-create"),
	)
	first, err := appendEvent(created)
	if err != nil {
		return 0, err
	}
	retry, err := appendEvent(created)
	if err != nil {
		return 0, err
	}
	if first.Cursor != retry.Cursor {
		return 0, errors.New("idempotent retry allocated another cursor")
	}
	conflict := workspaceEvent(
		ohrats_rc_events_types.LifecycleKind_WorkspaceCreated,
		workspace,
		"different",
		witTypes.Some("retry-create"),
	)
	if ohrats_rc_events_append.Append(conflict).IsOk() {
		return 0, errors.New("idempotency key accepted a different event")
	}
	devices := []struct {
		kind   uint8
		device string
	}{
		{ohrats_rc_events_types.LifecycleKind_DeviceEnrolled, "device-a"},
		{ohrats_rc_events_types.LifecycleKind_DeviceOnline, "device-a"},
		{ohrats_rc_events_types.LifecycleKind_DeviceEnrolled, "device-b"},
	}
	for _, entry := range devices {
		request := deviceEvent(entry.kind, workspace, entry.device+"-"+fixture)
		if _, err := appendEvent(request); err != nil {
			return 0, err
		}
	}
	invalid := workspaceEvent(
		ohrats_rc_events_types.LifecycleKind_WorkspaceRenamed,
		workspace,
		strings.Repeat("x", 121),
		witTypes.None[string](),
	)
	if ohrats_rc_events_append.Append(invalid).IsOk() {
		return 0, errors.New("oversize event detail was accepted")
	}
	invalid.Detail = ohrats_rc_events_types.MakeDetailDevice(ohrats_rc_events_types.DeviceDetail{
		WorkspaceId: workspace,
		DeviceId:    "device-bad",
		Name:        witTypes.None[string](),
	})
	if ohrats_rc_events_append.Append(invalid).IsOk() {
		return 0, errors.New("mismatched event detail was accepted")
	}
	fmt.Printf("seed cursor: %d\n", first.Cursor)
	return 0, nil
}

func configureRetention(maximumEvents uint32) error {
	result := ohrats_rc_events_retention.Configure(ohrats_rc_events_retention.Policy{MaximumEvents: maximumEvents})
	if result.IsErr() {
		return errors.New(result.Err())
	}
	return nil
}
