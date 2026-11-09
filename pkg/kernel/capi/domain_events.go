//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import "fmt"

var lifecycleKinds = []string{
	"account-created", "account-renamed", "account-deleted",
	"workspace-created", "workspace-renamed", "workspace-deleted",
	"workspace-member-joined", "workspace-member-role-changed", "workspace-member-left",
	"workspace-invite-created", "workspace-invite-revoked",
	"device-enrolled", "device-renamed", "device-online", "device-offline", "device-revoked",
}

type eventRecord struct {
	Cursor       uint64 `json:"cursor"`
	Kind         uint8  `json:"kind"`
	OccurredAtMs uint64 `json:"occurredAtMs"`
	Actor        string `json:"actor,omitempty"`
	Workspace    string `json:"workspace,omitempty"`
	Device       string `json:"device,omitempty"`
	Account      string `json:"account,omitempty"`
	DetailKind   string `json:"detailKind"`
	Name         string `json:"name,omitempty"`
	Role         string `json:"role,omitempty"`
	Invitation   string `json:"invitation,omitempty"`
}

type eventIdempotency struct {
	Fingerprint string      `json:"fingerprint"`
	Event       eventRecord `json:"event"`
}

type eventDomainState struct {
	Next      uint64                      `json:"next"`
	Retention uint32                      `json:"retention"`
	Events    []eventRecord               `json:"events"`
	Idem      map[string]eventIdempotency `json:"idem"`
}

func (s *eventDomainState) ensure() {
	if s.Retention == 0 {
		s.Retention = 10_000
	}
	if s.Idem == nil {
		s.Idem = make(map[string]eventIdempotency)
	}
}

func lifecycleKind(name string) (uint8, bool) {
	for index, value := range lifecycleKinds {
		if value == name {
			return uint8(index), true
		}
	}
	return 0, false
}

func lifecycleClass(kind uint8) string {
	switch {
	case kind <= 2:
		return "account"
	case kind <= 5:
		return "workspace"
	case kind <= 8:
		return "membership"
	case kind <= 10:
		return "invitation"
	default:
		return "device"
	}
}

func domainEventsConfigure(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid event retention policy")
		return
	}
	maximum := fieldU32(argValue(args, 0), "maximum-events")
	if maximum == 0 || maximum > 1_000_000 {
		setStringResult(results, false, "invalid event retention policy")
		return
	}
	err := mutateEvents(func(state *eventDomainState) error {
		state.Retention = maximum
		pruneEvents(state)
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, nil)
}

func domainEventsCurrent(_ *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t) {
	state, _, err := domainLoad[eventDomainState](domainStore(), "events")
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	state.ensure()
	setOK(results, newOrderedRecord([]fieldValue{{name: "maximum-events", value: newU32Value(state.Retention)}}))
}

func mutateEvents(fn func(*eventDomainState) error) error {
	_, err := domainMutate(domainStore(), "events", func(state *eventDomainState) error {
		state.ensure()
		return fn(state)
	})
	return err
}

func domainEventsAppend(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid event")
		return
	}
	request := argValue(args, 0)
	event, key, err := parseEventRequest(request)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	var appended eventRecord
	err = mutateEvents(func(state *eventDomainState) error {
		if key != "" {
			if existing, ok := state.Idem[key]; ok {
				if existing.Fingerprint != eventFingerprint(event) {
					return domainError("event idempotency key was reused")
				}
				appended = existing.Event
				return nil
			}
		}
		state.Next++
		event.Cursor = state.Next
		state.Events = append(state.Events, event)
		pruneEvents(state)
		if key != "" {
			state.Idem[key] = eventIdempotency{Fingerprint: eventFingerprint(event), Event: event}
			if len(state.Idem) > 4096 {
				for oldest := range state.Idem {
					delete(state.Idem, oldest)
					break
				}
			}
		}
		appended = event
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, eventValue(appended))
}

func parseEventRequest(value *C.wasmtime_component_val_t) (eventRecord, string, error) {
	kind, ok := lifecycleKind(valueEnum(field(value, "kind")))
	if !ok {
		return eventRecord{}, "", domainError("invalid event kind")
	}
	detail := field(value, "detail")
	detailKind := variantName(detail)
	payload := variantValue(detail)
	if detailKind == "" || lifecycleClass(kind) != detailKind {
		return eventRecord{}, "", domainError("event detail does not match its kind")
	}
	event := eventRecord{Kind: kind, DetailKind: detailKind, OccurredAtMs: fieldU64(value, "occurred-at-ms")}
	if actor, ok := optionString(field(value, "actor-account-id")); ok {
		event.Actor = actor
	}
	event.Workspace = fieldString(payload, "workspace-id")
	event.Account = fieldString(payload, "account-id")
	event.Device = fieldString(payload, "device-id")
	event.Invitation = fieldString(payload, "invitation-id")
	if name, ok := optionString(field(payload, "name")); ok {
		event.Name = name
	}
	if display, ok := optionString(field(payload, "display-name")); ok {
		event.Name = display
	}
	if role, ok := optionString(field(payload, "role")); ok {
		event.Role = role
	}
	if detailKind == "account" {
		if event.Account == "" {
			return eventRecord{}, "", domainError("invalid account event")
		}
	} else if event.Workspace == "" {
		return eventRecord{}, "", domainError("invalid workspace event")
	}
	if len(event.Name) > 120 || len(event.Workspace) > 128 || len(event.Device) > 128 ||
		len(event.Account) > 128 || len(event.Invitation) > 128 || len(event.Actor) > 128 {
		return eventRecord{}, "", domainError("event detail exceeds its bound")
	}
	key := ""
	if value, ok := optionString(field(value, "idempotency-key")); ok {
		if value == "" || len(value) > 128 {
			return eventRecord{}, "", domainError("invalid event idempotency key")
		}
		key = value
	}
	return event, key, nil
}

func eventFingerprint(event eventRecord) string {
	return fmt.Sprintf("%d|%d|%s|%s|%s|%s|%s|%s|%s|%s", event.Kind, event.OccurredAtMs,
		event.Actor, event.Workspace, event.Device, event.Account, event.Name, event.Role,
		event.Invitation, event.DetailKind)
}

func pruneEvents(state *eventDomainState) {
	if uint32(len(state.Events)) <= state.Retention {
		return
	}
	state.Events = append([]eventRecord(nil), state.Events[len(state.Events)-int(state.Retention):]...)
}

func eventValue(event eventRecord) *C.wasmtime_component_val_t {
	name := optionStringValue(event.Name)
	role := optionStringValue(event.Role)
	var detail *C.wasmtime_component_val_t
	switch event.DetailKind {
	case "account":
		detail = newVariantValue("account", newOrderedRecord([]fieldValue{
			{name: "account-id", value: newStringValue(event.Account)},
			{name: "display-name", value: name},
		}))
	case "membership":
		detail = newVariantValue("membership", newOrderedRecord([]fieldValue{
			{name: "workspace-id", value: newStringValue(event.Workspace)},
			{name: "account-id", value: newStringValue(event.Account)},
			{name: "role", value: role},
		}))
	case "invitation":
		detail = newVariantValue("invitation", newOrderedRecord([]fieldValue{
			{name: "workspace-id", value: newStringValue(event.Workspace)},
			{name: "invitation-id", value: newStringValue(event.Invitation)},
		}))
	case "device":
		detail = newVariantValue("device", newOrderedRecord([]fieldValue{
			{name: "workspace-id", value: newStringValue(event.Workspace)},
			{name: "device-id", value: newStringValue(event.Device)},
			{name: "name", value: name},
		}))
	default:
		detail = newVariantValue("workspace", newOrderedRecord([]fieldValue{
			{name: "workspace-id", value: newStringValue(event.Workspace)},
			{name: "name", value: name},
		}))
	}
	return newOrderedRecord([]fieldValue{
		{name: "cursor", value: newU64Value(event.Cursor)},
		{name: "kind", value: newEnumValue(lifecycleKinds[event.Kind])},
		{name: "occurred-at-ms", value: newU64Value(event.OccurredAtMs)},
		{name: "actor-account-id", value: optionStringValue(event.Actor)},
		{name: "detail", value: detail},
	})
}
