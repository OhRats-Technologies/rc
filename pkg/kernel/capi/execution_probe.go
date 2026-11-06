//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"time"
)

// RunExecutionRuntimeProbe starts an argv execution through execution-runtime.
func (h *Host) RunExecutionRuntimeProbe() (string, error) {
	if h == nil || h.ctx == nil {
		return "", errors.New("host is unavailable")
	}
	mode := newVariantValue("argv", newTupleValue([]*C.wasmtime_component_val_t{
		newStringValue("/bin/sh"),
		newListValue([]*C.wasmtime_component_val_t{newStringValue("-c"), newStringValue("printf exec")}),
	}))
	environment := newOrderedRecord([]fieldValue{
		{"base", newEnumValue("clean")},
		{"changes", newListValue(nil)},
	})
	principal := newOrderedRecord([]fieldValue{
		{"user-id", newStringValue("probe")},
		{"role", newStringValue("owner")},
		{"can-execute", newBoolValue(true)},
		{"can-manage-devices", newBoolValue(true)},
	})
	request := newOrderedRecord([]fieldValue{
		{"execution-id", newStringValue(fmt.Sprintf("probe-%d", time.Now().UnixNano()))},
		{"mode", mode},
		{"cwd", newNoneOption()},
		{"environment", environment},
		{"terminal", newNoneOption()},
		{"channel", newEnumValue("control")},
		{"lifetime", newEnumValue("managed")},
		{"principal", principal},
		{"max-runtime-ms", newNoneOption()},
	})
	start, err := h.callNested("ohrats:rc-process/runtime@0.3.0", "start", []C.wasmtime_component_val_t{*request})
	if err != nil {
		return "", err
	}
	if !bool(C.rc_result_ok(&start)) {
		return "", errors.New(valueString(C.rc_result_val(&start)))
	}
	execution := C.rc_result_val(&start)
	var output []byte
	cursor := uint64(0)
	for attempt := 0; attempt < 200; attempt++ {
		read, err := h.callNested("ohrats:rc-process/runtime@0.3.0", "[method]execution.read", []C.wasmtime_component_val_t{
			*execution, *newU64Value(cursor), *newU32Value(4096),
		})
		if err != nil {
			return "", err
		}
		if !bool(C.rc_result_ok(&read)) {
			return "", errors.New(valueString(C.rc_result_val(&read)))
		}
		result := C.rc_result_val(&read)
		chunks := recordField(result, "chunks")
		for index := 0; index < int(C.rc_list_len(chunks)); index++ {
			chunk := C.rc_list_get(chunks, C.size_t(index))
			kind := valueEnum(recordField(chunk, "kind"))
			if kind == "stdout" {
				output = append(output, valueBytes(recordField(chunk, "bytes"))...)
			}
		}
		if next := recordField(result, "next-cursor"); next != nil {
			cursor = uint64(C.rc_u64(next))
		}
		exit := recordField(result, "exit")
		if exit != nil && C.rc_option_get(exit) != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, _ = h.callNested("ohrats:rc-process/runtime@0.3.0", "[method]execution.close", []C.wasmtime_component_val_t{*execution})
	return string(output), nil
}
