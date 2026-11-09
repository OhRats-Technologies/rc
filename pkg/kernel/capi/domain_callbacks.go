//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"strconv"
	"strings"
	"unsafe"
)

//export goRCDomain
func goRCDomain(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	text := C.GoString((*C.char)(data))
	split := strings.IndexByte(text, '|')
	if split < 0 {
		setStringResult(results, false, "invalid domain binding")
		return nil
	}
	id, err := strconv.ParseUint(text[:split], 10, 64)
	if err != nil {
		setStringResult(results, false, "invalid domain binding")
		return nil
	}
	domainDispatch(id, text[split+1:], args, nargs, results, nresults)
	return nil
}
