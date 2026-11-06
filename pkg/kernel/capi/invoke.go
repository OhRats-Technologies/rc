//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// Invoke calls the component invoke export.
func (h *Host) Invoke(command string, args []string) (uint32, string, error) {
	commandC := C.CString(command)
	defer C.free(unsafe.Pointer(commandC))
	argValues := make([]C.wasmtime_component_val_t, len(args))
	strings := make([]*C.char, len(args))
	for i, value := range args {
		strings[i] = C.CString(value)
		C.rc_set_string(&argValues[i], (*C.uint8_t)(unsafe.Pointer(strings[i])), C.size_t(len(value)))
	}
	var list *C.wasmtime_component_val_t
	if len(argValues) > 0 {
		list = (*C.wasmtime_component_val_t)(C.malloc(C.size_t(len(argValues)) * C.size_t(unsafe.Sizeof(argValues[0]))))
		copy(unsafe.Slice(list, len(argValues)), argValues)
	}
	defer C.free(unsafe.Pointer(list))
	defer func() {
		for _, value := range strings {
			C.free(unsafe.Pointer(value))
		}
	}()
	callArgs := []C.wasmtime_component_val_t{{}}
	C.rc_set_string(&callArgs[0], (*C.uint8_t)(unsafe.Pointer(commandC)), C.size_t(len(command)))
	argsVal := C.wasmtime_component_val_t{}
	if len(argValues) > 0 {
		C.rc_set_list(&argsVal, list, C.size_t(len(argValues)))
	} else {
		C.rc_set_list(&argsVal, nil, 0)
	}
	callArgs = append(callArgs, argsVal)
	var value C.wasmtime_component_val_t
	if err := h.call("invoke", callArgs, &value); err != nil {
		return 0, "", err
	}
	defer C.wasmtime_component_val_delete(&value)
	if C.rc_val_kind(&value) != 19 {
		return 0, "", fmt.Errorf("invoke did not return a result")
	}
	payload := C.rc_result_val(&value)
	if C.rc_result_ok(&value) {
		if payload == nil {
			return 0, "", nil
		}
		return uint32(C.rc_u32(payload)), "", nil
	}
	return 0, valueString(payload), nil
}
