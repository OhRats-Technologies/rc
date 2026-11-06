//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"errors"
	"net"
	"strconv"
)

// RunNetProbe connects to an allowlisted TCP endpoint and echoes one payload.
func (h *Host) RunNetProbe(address string) (string, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", err
	}
	endpoint := newOrderedRecord([]fieldValue{
		{"host", newStringValue(host)},
		{"port", newU16Value(uint16(port))},
	})
	limits := newOrderedRecord([]fieldValue{
		{"connect-timeout-ms", newU64Value(5000)},
		{"idle-timeout-ms", newU64Value(5000)},
		{"max-lifetime-ms", newU64Value(30000)},
		{"max-pending-write-bytes", newU64Value(65536)},
	})
	var result C.wasmtime_component_val_t
	args := []C.wasmtime_component_val_t{*endpoint, *limits}
	netConnect(h.ctx, h.netSlot, &args[0], C.size_t(len(args)), &result, 1)
	if !bool(C.rc_result_ok(&result)) {
		return "", errors.New(valueString(C.rc_result_val(&result)))
	}
	rep, ty := resourceRep(h.ctx, C.rc_result_val(&result))
	fresh, err := hostResource(h.ctx, rep, ty)
	if err != nil {
		return "", err
	}
	writeArgs := []C.wasmtime_component_val_t{*resourceValue(fresh), *newBytesValue([]byte("ping"))}
	var writeResult C.wasmtime_component_val_t
	netWrite(h.ctx, &writeArgs[0], 2, &writeResult, 1)
	if !bool(C.rc_result_ok(&writeResult)) {
		return "", errors.New(valueString(C.rc_result_val(&writeResult)))
	}
	freshRead, err := hostResource(h.ctx, rep, ty)
	if err != nil {
		return "", err
	}
	readArgs := []C.wasmtime_component_val_t{*resourceValue(freshRead), *newU64Value(16)}
	var readResult C.wasmtime_component_val_t
	netRead(h.ctx, &readArgs[0], 2, &readResult, 1)
	if !bool(C.rc_result_ok(&readResult)) {
		return "", errors.New(valueString(C.rc_result_val(&readResult)))
	}
	variant := C.rc_result_val(&readResult)
	data := valueBytes(C.rc_variant_val(variant))
	return string(data), nil
}

func newU16Value(value uint16) *C.wasmtime_component_val_t {
	item := newValue()
	C.rc_set_u16(item, C.uint16_t(value))
	return item
}
