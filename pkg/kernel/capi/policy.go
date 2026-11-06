//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import "unsafe"

func policyAuthorizeStart(_ unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid start request")
		return
	}
	request := argValue(args, 0)
	plan := newValue()
	C.rc_alloc_record(plan, 10)
	cloneField(plan, 0, "mode", request, "mode")
	cloneField(plan, 1, "cwd", request, "cwd")
	cloneField(plan, 2, "environment", request, "environment")
	cloneField(plan, 3, "terminal", request, "terminal")
	*C.rc_record_item(plan, 4, cString("scrollback-bytes"), 16) = *newU32Value(4 << 20)
	*C.rc_record_item(plan, 5, cString("stdin-chunk-bytes"), 17) = *newU32Value(1 << 20)
	*C.rc_record_item(plan, 6, cString("authorization-timeout-ms"), 24) = *newU32Value(15_000)
	*C.rc_record_item(plan, 7, cString("terminate-grace-ms"), 18) = *newU32Value(350)
	*C.rc_record_item(plan, 8, cString("reattach-grace-ms"), 17) = *newU32Value(60_000)
	cloneField(plan, 9, "max-runtime-ms", request, "max-runtime-ms")
	C.rc_set_result(results, true, plan)
}

func cloneField(target *C.wasmtime_component_val_t, index int, name string, source *C.wasmtime_component_val_t, sourceName string) {
	value := recordField(source, sourceName)
	item := C.rc_record_item(target, C.size_t(index), cString(name), C.size_t(len(name)))
	if value == nil {
		C.rc_set_none_option(item)
		return
	}
	C.rc_clone_val(item, value)
}

func policyAuthorizeAccess(_ unsafe.Pointer, _ *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	C.rc_set_result(results, true, nil)
}

func policyNormalizeResize(_ unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid resize request")
		return
	}
	request := argValue(args, 0)
	size := newValue()
	C.rc_alloc_record(size, 2)
	cols := recordField(request, "cols")
	rows := recordField(request, "rows")
	if cols != nil {
		*C.rc_record_item(size, 0, cString("cols"), 4) = *cols
	}
	if rows != nil {
		*C.rc_record_item(size, 1, cString("rows"), 4) = *rows
	}
	C.rc_set_result(results, true, size)
}

func policyAuthorizeSignal(_ unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid signal request")
		return
	}
	signal := recordField(argValue(args, 0), "signal")
	if signal == nil {
		setStringResult(results, false, "invalid signal request")
		return
	}
	C.rc_set_result(results, true, signal)
}

func diagnosticsSubmit(_ unsafe.Pointer, _ *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	payload := newValue()
	C.rc_set_u64(payload, 0)
	C.rc_set_result(results, true, payload)
}
