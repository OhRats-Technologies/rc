package export_wit_world

import (
	"errors"
	"fmt"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_events_feed"
	"wit_component/ohrats_rc_events_query"
	"wit_component/ohrats_rc_events_types"
)

func verify(args []string) (uint32, error) {
	if len(args) != 1 {
		return 0, errors.New("usage: rc events-verify <fixture>")
	}
	fixture := args[0]
	if err := validFixture(fixture); err != nil {
		return 0, err
	}
	workspace := "workspace-" + fixture
	auth := ohrats_rc_events_types.Authorization{
		RequesterAccountId:      "account-fixture",
		WorkspaceIds:            []string{workspace},
		IncludeOwnAccountEvents: true,
	}
	empty := ohrats_rc_events_types.Filter{
		Kinds:       []uint8{},
		WorkspaceId: witTypes.Some(workspace),
		DeviceId:    witTypes.None[string](),
		AccountId:   witTypes.None[string](),
	}
	page1, err := queryEvents(auth, 0, 2, empty)
	if err != nil {
		return 0, err
	}
	page2, err := pollEvents(auth, page1.NextCursor, 2, empty)
	if err != nil {
		return 0, err
	}
	if len(page1.Events) != 2 ||
		len(page2.Events) != 2 ||
		page1.Events[0].Cursor >= page1.Events[1].Cursor ||
		page1.NextCursor >= page2.NextCursor {
		return 0, errors.New("event order or pagination cursor is invalid")
	}
	filtered := ohrats_rc_events_types.Filter{
		Kinds:       []uint8{ohrats_rc_events_types.LifecycleKind_DeviceOnline},
		WorkspaceId: witTypes.Some(workspace),
		DeviceId:    witTypes.Some("device-a-" + fixture),
		AccountId:   witTypes.None[string](),
	}
	matched, err := queryEvents(auth, 0, 10, filtered)
	if err != nil {
		return 0, err
	}
	if len(matched.Events) != 1 {
		return 0, errors.New("event filtering failed")
	}
	noMatch := ohrats_rc_events_types.Filter{
		Kinds:       []uint8{ohrats_rc_events_types.LifecycleKind_AccountDeleted},
		WorkspaceId: witTypes.None[string](),
		DeviceId:    witTypes.None[string](),
		AccountId:   witTypes.None[string](),
	}
	skipped, err := pollEvents(auth, 0, 10, noMatch)
	if err != nil {
		return 0, err
	}
	if len(skipped.Events) != 0 || skipped.NextCursor != page2.NextCursor {
		return 0, errors.New("filtered feed did not advance its ordered cursor")
	}
	denied := ohrats_rc_events_types.Authorization{
		RequesterAccountId:      "account-fixture",
		WorkspaceIds:            []string{},
		IncludeOwnAccountEvents: true,
	}
	if query := ohrats_rc_events_query.Query(denied, 0, 10, empty); query.IsOk() {
		return 0, errors.New("unauthorized workspace filter was accepted")
	}
	hidden := ohrats_rc_events_types.Filter{
		Kinds:       []uint8{},
		WorkspaceId: witTypes.None[string](),
		DeviceId:    witTypes.Some("not-visible"),
		AccountId:   witTypes.None[string](),
	}
	skipped, err = pollEvents(auth, 0, 10, hidden)
	if err != nil {
		return 0, err
	}
	if len(skipped.Events) != 0 || skipped.NextCursor == 0 {
		return 0, errors.New("feed cursor did not advance over filtered events")
	}
	if err := configureRetention(2); err != nil {
		return 0, err
	}
	offline := deviceEvent(ohrats_rc_events_types.LifecycleKind_DeviceOffline, workspace, "device-a-"+fixture)
	if _, err := appendEvent(offline); err != nil {
		return 0, err
	}
	revoked := deviceEvent(ohrats_rc_events_types.LifecycleKind_DeviceRevoked, workspace, "device-b-"+fixture)
	newest, err := appendEvent(revoked)
	if err != nil {
		return 0, err
	}
	retained, err := queryEvents(auth, 1, 10, empty)
	if err != nil {
		return 0, err
	}
	if len(retained.Events) != 2 ||
		!retained.ResetRequired ||
		retained.Events[len(retained.Events)-1].Cursor != newest.Cursor {
		return 0, errors.New("retention pruning or reset cursor failed")
	}
	if configureRetention(0) == nil {
		return 0, errors.New("invalid retention limit was accepted")
	}
	if err := verifyIdempotency(workspace); err != nil {
		return 0, err
	}
	fmt.Println("events state: ok")
	return 0, nil
}

func queryEvents(auth ohrats_rc_events_types.Authorization, after uint64, limit uint32, filter ohrats_rc_events_types.Filter) (ohrats_rc_events_types.Batch, error) {
	result := ohrats_rc_events_query.Query(auth, after, limit, filter)
	if result.IsErr() {
		return ohrats_rc_events_types.Batch{}, errors.New(result.Err())
	}
	return result.Ok(), nil
}

func pollEvents(auth ohrats_rc_events_types.Authorization, after uint64, limit uint32, filter ohrats_rc_events_types.Filter) (ohrats_rc_events_types.Batch, error) {
	result := ohrats_rc_events_feed.Poll(auth, after, limit, filter)
	if result.IsErr() {
		return ohrats_rc_events_types.Batch{}, errors.New(result.Err())
	}
	return result.Ok(), nil
}
