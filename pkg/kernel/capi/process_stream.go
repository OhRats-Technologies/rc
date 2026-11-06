//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"os"
	"unsafe"
)

func processChildState(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	child, ok := selfChild(ctx, args)
	if !ok {
		C.rc_set_enum(results, cString("lost"), 4)
		return
	}
	child.group.mu.Lock()
	done := child.group.done
	child.group.mu.Unlock()
	if done {
		C.rc_set_enum(results, cString("exited"), 6)
	} else {
		C.rc_set_enum(results, cString("running"), 7)
	}
}

func processChildPollExit(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	child, ok := selfChild(ctx, args)
	if !ok {
		setStringResult(results, false, "unknown child")
		return
	}
	child.group.mu.Lock()
	done, cmd := child.group.done, child.group.cmd
	child.group.mu.Unlock()
	option := newValue()
	if !done || cmd == nil || cmd.ProcessState == nil {
		C.rc_set_none_option(option)
		C.rc_set_result(results, true, option)
		return
	}
	exit := newValue()
	C.rc_alloc_record(exit, 2)
	code := cmd.ProcessState.ExitCode()
	codeOption := C.rc_record_item(exit, 0, cString("code"), 4)
	if code >= 0 {
		value := newValue()
		C.rc_set_u32(value, C.uint32_t(code))
		C.rc_set_some_option(codeOption, value)
	} else {
		C.rc_set_none_option(codeOption)
	}
	C.rc_set_none_option(C.rc_record_item(exit, 1, cString("signal"), 6))
	C.rc_set_some_option(option, exit)
	C.rc_set_result(results, true, option)
}

func processStreamRead(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	stream, ok := selfStream(ctx, args)
	if !ok || nargs < 2 {
		setStringResult(results, false, "unknown byte stream")
		return
	}
	max := int(C.rc_u32(argValue(args, 1)))
	data, eof, _ := stream.read(max)
	variant := newValue()
	if eof {
		C.rc_set_variant(variant, cString("eof"), 3, nil)
	} else if len(data) == 0 {
		C.rc_set_variant(variant, cString("would-block"), 11, nil)
	} else {
		payload := newValue()
		setBytesValue(payload, data)
		C.rc_set_variant(variant, cString("data"), 4, payload)
	}
	C.rc_set_result(results, true, variant)
}

func processStreamWrite(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	stream, ok := selfStream(ctx, args)
	if !ok || nargs < 2 {
		setStringResult(results, false, "unknown byte stream")
		return
	}
	stream.mu.Lock()
	writer := stream.writer
	stream.mu.Unlock()
	if writer == nil {
		setStringResult(results, false, "stream is not writable")
		return
	}
	n, err := writer.Write(valueBytes(argValue(args, 1)))
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	accepted := newValue()
	C.rc_set_u32(accepted, C.uint32_t(n))
	variant := newValue()
	C.rc_set_variant(variant, cString("accepted"), 8, accepted)
	C.rc_set_result(results, true, variant)
}

func processStreamCloseWrite(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	stream, ok := selfStream(ctx, args)
	if !ok {
		setStringResult(results, false, "unknown byte stream")
		return
	}
	stream.mu.Lock()
	writer := stream.writer
	stream.mu.Unlock()
	if writer != nil {
		_ = writer.Close()
	}
	C.rc_set_result(results, true, nil)
}

func processStreamClose(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, _ C.size_t, _ *C.wasmtime_component_val_t, _ C.size_t) {
	if stream, ok := selfStream(ctx, args); ok {
		stream.close()
	}
}

func selfGroup(ctx unsafe.Pointer, args *C.wasmtime_component_val_t) (*processGroup, bool) {
	rep, ok := selfRep(ctx, args)
	if !ok {
		return nil, false
	}
	group, ok := lookupResource(rep).(*processGroup)
	return group, ok
}

func selfChild(ctx unsafe.Pointer, args *C.wasmtime_component_val_t) (*childResource, bool) {
	rep, ok := selfRep(ctx, args)
	if !ok {
		return nil, false
	}
	child, ok := lookupResource(rep).(*childResource)
	return child, ok
}

func selfStream(ctx unsafe.Pointer, args *C.wasmtime_component_val_t) (*streamResource, bool) {
	rep, ok := selfRep(ctx, args)
	if !ok {
		return nil, false
	}
	stream, ok := lookupResource(rep).(*streamResource)
	return stream, ok
}

func selfRep(ctx unsafe.Pointer, args *C.wasmtime_component_val_t) (uint32, bool) {
	return selfRepAt(ctx, args, 0)
}

func selfRepAt(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, index int) (uint32, bool) {
	if args == nil || C.rc_val_kind(argValue(args, index)) != 21 {
		return 0, false
	}
	any := C.rc_resource_any(argValue(args, index))
	if any == nil {
		return 0, false
	}
	var host unsafe.Pointer
	if errPtr := C.wasmtime_component_resource_any_to_host(ctx, any, &host); errPtr != nil {
		return 0, false
	}
	rep := uint32(C.wasmtime_component_resource_host_rep(host))
	C.wasmtime_component_resource_host_delete(host)
	return rep, true
}

func hostResource(ctx unsafe.Pointer, rep uint32, ty uint32) (unsafe.Pointer, error) {
	host := C.wasmtime_component_resource_host_new(true, C.uint32_t(rep), C.uint32_t(ty))
	if host == nil {
		return nil, errString("could not create host resource")
	}
	var any unsafe.Pointer
	if errPtr := C.wasmtime_component_resource_host_to_any(ctx, host, &any); errPtr != nil {
		C.wasmtime_component_resource_host_delete(host)
		return nil, takeError(errPtr)
	}
	C.wasmtime_component_resource_host_delete(host)
	return any, nil
}

func resourceValue(any unsafe.Pointer) *C.wasmtime_component_val_t {
	value := newValue()
	C.rc_set_resource(value, any)
	return value
}

func valueEnum(value *C.wasmtime_component_val_t) string {
	if value == nil || C.rc_val_kind(value) != 17 {
		return ""
	}
	return goString(C.rc_variant_name(value), C.rc_variant_name_len(value))
}

func processEnvironment(value *C.wasmtime_component_val_t) []string {
	environment := os.Environ()
	if value == nil {
		return environment
	}
	base := valueEnum(recordField(value, "base"))
	if base == "clean" {
		environment = nil
	}
	changes := recordField(value, "changes")
	if changes == nil {
		return environment
	}
	values := make(map[string]string)
	for _, entry := range environment {
		for index := 0; index < len(entry); index++ {
			if entry[index] == '=' {
				values[entry[:index]] = entry[index+1:]
				break
			}
		}
	}
	for index := 0; index < int(C.rc_list_len(changes)); index++ {
		change := C.rc_list_get(changes, C.size_t(index))
		name := valueString(recordField(change, "name"))
		option := recordField(change, "value")
		if option != nil && C.rc_option_get(option) != nil {
			values[name] = valueString(C.rc_option_get(option))
		} else {
			delete(values, name)
		}
	}
	result := make([]string, 0, len(values))
	for name, value := range values {
		result = append(result, name+"="+value)
	}
	return result
}

type errString string

func (e errString) Error() string { return string(e) }
