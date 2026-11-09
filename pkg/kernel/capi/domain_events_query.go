//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

func eventStrings(value *C.wasmtime_component_val_t) []string {
	if value == nil {
		return nil
	}
	values := make([]string, 0, int(C.rc_list_len(value)))
	for index := 0; index < int(C.rc_list_len(value)); index++ {
		values = append(values, valueString(C.rc_list_get(value, C.size_t(index))))
	}
	return values
}

func eventKinds(value *C.wasmtime_component_val_t) (map[uint8]bool, error) {
	if value == nil {
		return nil, nil
	}
	kinds := make(map[uint8]bool, int(C.rc_list_len(value)))
	for index := 0; index < int(C.rc_list_len(value)); index++ {
		kind, ok := lifecycleKind(valueEnum(C.rc_list_get(value, C.size_t(index))))
		if !ok {
			return nil, domainError("invalid event kind filter")
		}
		kinds[kind] = true
	}
	return kinds, nil
}

func domainEventsQuery(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 4 {
		setStringResult(results, false, "invalid event query")
		return
	}
	auth, after := argValue(args, 0), uint64(C.rc_u64(argValue(args, 1)))
	limit := uint32(C.rc_u32(argValue(args, 2)))
	filter := argValue(args, 3)
	if limit == 0 || limit > 500 {
		setStringResult(results, false, "invalid event query limit")
		return
	}
	requester := fieldString(auth, "requester-account-id")
	allowed := make(map[string]bool)
	for _, workspace := range eventStrings(field(auth, "workspace-ids")) {
		allowed[workspace] = true
	}
	includeOwn := fieldBool(auth, "include-own-account-events")
	workspaceFilter, hasWorkspace := optionString(field(filter, "workspace-id"))
	deviceFilter, hasDevice := optionString(field(filter, "device-id"))
	accountFilter, hasAccount := optionString(field(filter, "account-id"))
	kinds, err := eventKinds(field(filter, "kinds"))
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	if hasWorkspace && !allowed[workspaceFilter] {
		setStringResult(results, false, "event query workspace is not authorized")
		return
	}
	state, _, err := domainLoad[eventDomainState](domainStore(), "events")
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	state.ensure()
	reset := false
	if len(state.Events) > 0 && after < state.Events[0].Cursor-1 {
		reset = true
	}
	events := make([]*C.wasmtime_component_val_t, 0, limit)
	next, lastReturned := after, after
	for _, event := range state.Events {
		if event.Cursor <= after {
			continue
		}
		next = event.Cursor
		if !eventVisible(event, requester, allowed, includeOwn) {
			continue
		}
		if hasWorkspace && event.Workspace != workspaceFilter {
			continue
		}
		if hasDevice && event.Device != deviceFilter {
			continue
		}
		if hasAccount && event.Actor != accountFilter {
			continue
		}
		if len(kinds) > 0 && !kinds[event.Kind] {
			continue
		}
		if uint32(len(events)) < limit {
			events = append(events, eventValue(event))
			lastReturned = event.Cursor
		}
	}
	if uint32(len(events)) == limit && limit > 0 {
		next = lastReturned
	}
	setOK(results, newOrderedRecord([]fieldValue{
		{name: "events", value: newListValue(events)},
		{name: "next-cursor", value: newU64Value(next)},
		{name: "reset-required", value: newBoolValue(reset)},
	}))
}

func eventVisible(event eventRecord, requester string, allowed map[string]bool, includeOwn bool) bool {
	if event.Workspace != "" {
		return allowed[event.Workspace]
	}
	return includeOwn && requester != "" && event.Actor == requester
}
