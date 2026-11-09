//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

func domainMeshValidate(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 3 {
		setRejection(results, "invalid-envelope")
		return
	}
	value := argValue(args, 0)
	now := uint64(C.rc_u64(argValue(args, 1)))
	maximum := uint8(C.rc_u8(argValue(args, 2)))
	if err := meshEnvelopeError(value, now, maximum); err != "" {
		setRejection(results, err)
		return
	}
	setOK(results, nil)
}

func domainMeshForward(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 2 {
		setRejection(results, "route-rejected")
		return
	}
	value := argValue(args, 0)
	relay := valueString(argValue(args, 1))
	if !meshIdentifier(relay) || fieldString(value, "source-peer-id") == relay ||
		fieldString(value, "destination-peer-id") == relay {
		setRejection(results, "route-rejected")
		return
	}
	route := fieldStrings(value, "route")
	for _, peer := range route {
		if peer == relay {
			setRejection(results, "route-rejected")
			return
		}
	}
	if len(route)+1 > int(C.rc_u8(field(value, "max-hops"))) {
		setRejection(results, "route-rejected")
		return
	}
	route = append(route, relay)
	items := make([]*C.wasmtime_component_val_t, len(route))
	for index, peer := range route {
		items[index] = newStringValue(peer)
	}
	setOK(results, newOrderedRecord([]fieldValue{
		{name: "version", value: newU32Value(fieldU32(value, "version"))},
		{name: "realm-id", value: newStringValue(fieldString(value, "realm-id"))},
		{name: "message-id", value: newStringValue(fieldString(value, "message-id"))},
		{name: "source-peer-id", value: newStringValue(fieldString(value, "source-peer-id"))},
		{name: "destination-peer-id", value: newStringValue(fieldString(value, "destination-peer-id"))},
		{name: "issued-at-ms", value: newU64Value(fieldU64(value, "issued-at-ms"))},
		{name: "expires-at-ms", value: newU64Value(fieldU64(value, "expires-at-ms"))},
		{name: "max-hops", value: newU8Value(uint8(C.rc_u8(field(value, "max-hops"))))},
		{name: "ciphertext", value: newBytesValue(fieldBytes(value, "ciphertext"))},
		{name: "route", value: newListValue(items)},
	}))
}

func meshEnvelopeError(value *C.wasmtime_component_val_t, now uint64, maximum uint8) string {
	if fieldU32(value, "version") != 1 || !meshIdentifier(fieldString(value, "realm-id")) ||
		!meshIdentifier(fieldString(value, "message-id")) ||
		!meshIdentifier(fieldString(value, "source-peer-id")) ||
		!meshIdentifier(fieldString(value, "destination-peer-id")) ||
		fieldString(value, "source-peer-id") == fieldString(value, "destination-peer-id") {
		return "invalid-envelope"
	}
	issued, expires := fieldU64(value, "issued-at-ms"), fieldU64(value, "expires-at-ms")
	maxHops := uint8(C.rc_u8(field(value, "max-hops")))
	if expires <= now || expires <= issued || issued > now+60000 || maxHops == 0 || maxHops > maximum {
		return "expired"
	}
	ciphertext := fieldBytes(value, "ciphertext")
	if len(ciphertext) == 0 || len(ciphertext) > 1_048_576 || len(fieldStrings(value, "route")) > int(maxHops) {
		return "invalid-envelope"
	}
	return ""
}
