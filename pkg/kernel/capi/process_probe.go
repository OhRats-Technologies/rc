//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// RunProcessProbe exercises the linked process resource host with a real
// command. It is used by the component-host integration test.
func (h *Host) RunProcessProbe() (string, error) {
	if h == nil || h.ctx == nil {
		return "", errors.New("host is unavailable")
	}
	environment := newOrderedRecord([]fieldValue{
		{"base", newEnumValue("inherit")},
		{"changes", newListValue(nil)},
	})
	request := newOrderedRecord([]fieldValue{
		{"program", newStringValue("/bin/sh")},
		{"args", newListValue([]*C.wasmtime_component_val_t{newStringValue("-c"), newStringValue("printf hi")})},
		{"cwd", newNoneOption()},
		{"environment", environment},
		{"terminal", newNoneOption()},
	})
	groupArgs := []C.wasmtime_component_val_t{}
	groupArg := newStringValue("probe")
	groupArgs = append(groupArgs, *groupArg)
	var groupResult C.wasmtime_component_val_t
	processCreateGroup(h.ctx, &groupArgs[0], C.size_t(len(groupArgs)), &groupResult, 1)
	if !bool(C.rc_result_ok(&groupResult)) {
		return "", errors.New(valueString(C.rc_result_val(&groupResult)))
	}
	groupValue := C.rc_result_val(&groupResult)
	spawnArgs := []C.wasmtime_component_val_t{*groupValue, *request}
	var spawnResult C.wasmtime_component_val_t
	processGroupSpawn(h.ctx, &spawnArgs[0], C.size_t(len(spawnArgs)), &spawnResult, 1)
	if !bool(C.rc_result_ok(&spawnResult)) {
		return "", errors.New(valueString(C.rc_result_val(&spawnResult)))
	}
	spawned := C.rc_result_val(&spawnResult)
	stdout := recordField(spawned, "stdout")
	if stdout == nil || C.rc_val_kind(stdout) != 21 {
		return "", errors.New("spawn did not return a stdout resource")
	}
	stdoutRep, stdoutType := resourceRep(h.ctx, stdout)
	var output []byte
	for {
		fresh, err := hostResource(h.ctx, stdoutRep, stdoutType)
		if err != nil {
			return "", err
		}
		readArgs := []C.wasmtime_component_val_t{*resourceValue(fresh), *newU32Value(64)}
		var readResult C.wasmtime_component_val_t
		processStreamRead(h.ctx, &readArgs[0], C.size_t(len(readArgs)), &readResult, 1)
		if !bool(C.rc_result_ok(&readResult)) {
			return "", errors.New(valueString(C.rc_result_val(&readResult)))
		}
		variant := C.rc_result_val(&readResult)
		name := goString(C.rc_variant_name(variant), C.rc_variant_name_len(variant))
		if name == "eof" {
			break
		}
		if name == "data" {
			output = append(output, valueBytes(C.rc_variant_val(variant))...)
			continue
		}
		break
	}
	child := recordField(spawned, "child")
	childRep, childType := resourceRep(h.ctx, child)
	for attempt := 0; attempt < 100; attempt++ {
		freshChild, err := hostResource(h.ctx, childRep, childType)
		if err != nil {
			return "", err
		}
		pollArgs := []C.wasmtime_component_val_t{*resourceValue(freshChild)}
		var pollResult C.wasmtime_component_val_t
		processChildPollExit(h.ctx, &pollArgs[0], C.size_t(len(pollArgs)), &pollResult, 1)
		if bool(C.rc_result_ok(&pollResult)) && C.rc_option_get(C.rc_result_val(&pollResult)) != nil {
			break
		}
	}
	return string(output), nil
}

func resourceRep(ctx unsafe.Pointer, value *C.wasmtime_component_val_t) (uint32, uint32) {
	any := C.rc_resource_any(value)
	var host unsafe.Pointer
	if errPtr := C.wasmtime_component_resource_any_to_host(ctx, any, &host); errPtr != nil {
		return 0, 0
	}
	rep := uint32(C.wasmtime_component_resource_host_rep(host))
	ty := uint32(C.wasmtime_component_resource_host_type(host))
	C.wasmtime_component_resource_host_delete(host)
	return rep, ty
}

var _ = unsafe.Pointer(nil)
