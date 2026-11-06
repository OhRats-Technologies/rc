//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"fmt"
)

func (h *Host) callNested(instanceName, functionName string, args []C.wasmtime_component_val_t) (C.wasmtime_component_val_t, error) {
	var zero C.wasmtime_component_val_t
	instanceIndex := h.instance.GetExportIndex(h.store, nil, instanceName)
	if instanceIndex == nil {
		return zero, fmt.Errorf("component interface %q was not found", instanceName)
	}
	functionIndex := h.instance.GetExportIndex(h.store, instanceIndex, functionName)
	if functionIndex == nil {
		return zero, fmt.Errorf("component export %q.%q was not found", instanceName, functionName)
	}
	context := C.wasmtime_store_context(pointerField(h.store, "_ptr"))
	function := C.malloc(functionHandleSize)
	defer C.free(function)
	if !bool(C.wasmtime_component_instance_get_func(h.instancePtr, context, pointerField(functionIndex, "_ptr"), function)) {
		return zero, fmt.Errorf("component export %q.%q was not found", instanceName, functionName)
	}
	var argsPtr *C.wasmtime_component_val_t
	if len(args) > 0 {
		argsPtr = &args[0]
	}
	if errPtr := C.wasmtime_component_func_call(function, context, argsPtr, C.size_t(len(args)), &zero, 1); errPtr != nil {
		return zero, takeError(errPtr)
	}
	return zero, nil
}
