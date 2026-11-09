//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

func domainComponentInstalled(_ *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t) {
	setOK(results, newListValue(nil))
}
