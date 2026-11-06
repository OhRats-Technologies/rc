package events

import (
	"errors"

	"wit_component/ohrats_rc_events_types"
)

const maxPage = 500

// Run authorizes and pages through stored lifecycle events.
func Run(auth ohrats_rc_events_types.Authorization, after uint64, limit uint32, filter ohrats_rc_events_types.Filter) (ohrats_rc_events_types.Batch, error) {
	if err := validateID(auth.RequesterAccountId, "requester account id"); err != nil {
		return ohrats_rc_events_types.Batch{}, err
	}
	if limit == 0 || limit > maxPage {
		return ohrats_rc_events_types.Batch{}, errors.New("event page limit is outside supported bounds")
	}
	for _, workspace := range auth.WorkspaceIds {
		if err := validateID(workspace, "authorized workspace id"); err != nil {
			return ohrats_rc_events_types.Batch{}, err
		}
	}
	if filter.DeviceId.IsSome() {
		if err := validateID(filter.DeviceId.Some(), "device filter"); err != nil {
			return ohrats_rc_events_types.Batch{}, err
		}
	}
	if filter.AccountId.IsSome() {
		if err := validateID(filter.AccountId.Some(), "account filter"); err != nil {
			return ohrats_rc_events_types.Batch{}, err
		}
	}
	if filter.WorkspaceId.IsSome() {
		workspace := filter.WorkspaceId.Some()
		if err := validateID(workspace, "workspace filter"); err != nil {
			return ohrats_rc_events_types.Batch{}, err
		}
		authorized := false
		for _, allowed := range auth.WorkspaceIds {
			if allowed == workspace {
				authorized = true
				break
			}
		}
		if !authorized {
			return ohrats_rc_events_types.Batch{}, errors.New("workspace is not authorized")
		}
	}
	entries, err := scan(EventsBucket)
	if err != nil {
		return ohrats_rc_events_types.Batch{}, err
	}
	var oldest uint64
	if len(entries) > 0 {
		if stored, err := decodeStoredEvent(entries[0].Value); err == nil {
			oldest = stored.Cursor
		}
	}
	resetRequired := after != 0 && oldest != 0 && after < ^uint64(0) && after+1 < oldest
	events := make([]ohrats_rc_events_types.Event, 0, limit)
	nextCursor := after
	for _, entry := range entries {
		stored, err := decodeStoredEvent(entry.Value)
		if err != nil {
			return ohrats_rc_events_types.Batch{}, err
		}
		event, err := stored.Wire()
		if err != nil {
			return ohrats_rc_events_types.Batch{}, err
		}
		if event.Cursor <= after {
			continue
		}
		nextCursor = event.Cursor
		if !authorizedEvent(event, auth) || !matchesFilter(event, filter) {
			continue
		}
		events = append(events, event)
		if len(events) == int(limit) {
			break
		}
	}
	return ohrats_rc_events_types.Batch{
		Events:        events,
		NextCursor:    nextCursor,
		ResetRequired: resetRequired,
	}, nil
}

func authorizedEvent(event ohrats_rc_events_types.Event, auth ohrats_rc_events_types.Authorization) bool {
	switch event.Detail.Tag() {
	case ohrats_rc_events_types.Detail_Account:
		return auth.IncludeOwnAccountEvents && event.Detail.Account().AccountId == auth.RequesterAccountId
	case ohrats_rc_events_types.Detail_Workspace:
		return contains(auth.WorkspaceIds, event.Detail.Workspace().WorkspaceId)
	case ohrats_rc_events_types.Detail_Membership:
		return contains(auth.WorkspaceIds, event.Detail.Membership().WorkspaceId)
	case ohrats_rc_events_types.Detail_Invitation:
		return contains(auth.WorkspaceIds, event.Detail.Invitation().WorkspaceId)
	case ohrats_rc_events_types.Detail_Device:
		return contains(auth.WorkspaceIds, event.Detail.Device().WorkspaceId)
	default:
		return false
	}
}

func matchesFilter(event ohrats_rc_events_types.Event, filter ohrats_rc_events_types.Filter) bool {
	if len(filter.Kinds) > 0 && !containsKind(filter.Kinds, event.Kind) {
		return false
	}
	var workspace, device, account *string
	switch event.Detail.Tag() {
	case ohrats_rc_events_types.Detail_Account:
		value := event.Detail.Account().AccountId
		account = &value
	case ohrats_rc_events_types.Detail_Workspace:
		value := event.Detail.Workspace().WorkspaceId
		workspace = &value
	case ohrats_rc_events_types.Detail_Membership:
		detail := event.Detail.Membership()
		workspace = &detail.WorkspaceId
		account = &detail.AccountId
	case ohrats_rc_events_types.Detail_Invitation:
		value := event.Detail.Invitation().WorkspaceId
		workspace = &value
	case ohrats_rc_events_types.Detail_Device:
		detail := event.Detail.Device()
		workspace = &detail.WorkspaceId
		device = &detail.DeviceId
	default:
		return false
	}
	if filter.WorkspaceId.IsSome() && !sameValue(filter.WorkspaceId.Some(), workspace) {
		return false
	}
	if filter.DeviceId.IsSome() && !sameValue(filter.DeviceId.Some(), device) {
		return false
	}
	if filter.AccountId.IsSome() && !sameValue(filter.AccountId.Some(), account) {
		return false
	}
	return true
}

func sameValue(expected string, value *string) bool {
	return value != nil && *value == expected
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func containsKind(values []ohrats_rc_events_types.LifecycleKind, expected ohrats_rc_events_types.LifecycleKind) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
