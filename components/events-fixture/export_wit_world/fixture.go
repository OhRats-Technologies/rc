package export_wit_world

import (
	"errors"
	"fmt"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_events_append"
	"wit_component/ohrats_rc_events_types"
	"wit_component/ohrats_rc_plugin_types"
)

// invoke dispatches the events fixture commands.
func invoke(command string, args []string) (uint32, error) {
	switch command {
	case "events-seed":
		return seed(args)
	case "events-verify":
		return verify(args)
	}
	return 0, fmt.Errorf("unsupported command %q", command)
}

func workspaceEvent(kind uint8, workspace string, name string, idempotency witTypes.Option[string]) ohrats_rc_events_types.AppendRequest {
	return ohrats_rc_events_types.AppendRequest{
		Kind:           kind,
		OccurredAtMs:   1_000,
		ActorAccountId: witTypes.Some("account-fixture"),
		Detail: ohrats_rc_events_types.MakeDetailWorkspace(ohrats_rc_events_types.WorkspaceDetail{
			WorkspaceId: workspace,
			Name:        witTypes.Some(name),
		}),
		IdempotencyKey: idempotency,
	}
}

func deviceEvent(kind uint8, workspace string, device string) ohrats_rc_events_types.AppendRequest {
	return ohrats_rc_events_types.AppendRequest{
		Kind:           kind,
		OccurredAtMs:   2_000,
		ActorAccountId: witTypes.Some("account-fixture"),
		Detail: ohrats_rc_events_types.MakeDetailDevice(ohrats_rc_events_types.DeviceDetail{
			WorkspaceId: workspace,
			DeviceId:    device,
			Name:        witTypes.None[string](),
		}),
		IdempotencyKey: witTypes.None[string](),
	}
}

func appendEvent(request ohrats_rc_events_types.AppendRequest) (ohrats_rc_events_types.Event, error) {
	result := ohrats_rc_events_append.Append(request)
	if result.IsErr() {
		return ohrats_rc_events_types.Event{}, errors.New(result.Err())
	}
	return result.Ok(), nil
}

func validFixture(value string) error {
	valid := len(value) > 0 && len(value) <= 40
	for index := 0; valid && index < len(value); index++ {
		character := value[index]
		upper := character >= 'A' && character <= 'Z'
		lower := character >= 'a' && character <= 'z'
		digit := character >= '0' && character <= '9'
		valid = upper || lower || digit || character == '-'
	}
	if !valid {
		return errors.New("invalid fixture")
	}
	return nil
}

func command(name string, summary string, usage string) ohrats_rc_plugin_types.Command {
	return ohrats_rc_plugin_types.Command{Name: name, Summary: summary, Usage: usage}
}

func requirement(name string) ohrats_rc_plugin_types.Requirement {
	return ohrats_rc_plugin_types.Requirement{
		Name:      "ohrats:rc-events/" + name,
		Version:   "^0.1",
		Selection: ohrats_rc_plugin_types.Selection_Single,
	}
}
