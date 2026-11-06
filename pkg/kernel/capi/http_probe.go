//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import "strings"

// RunHTTPSourceProbe resolves an HTTP package artifact through the HTTP host.
func (h *Host) RunHTTPSourceProbe(url string) (string, error) {
	result, err := h.callNested("ohrats:rc-plugin/package-source@0.1.0", "resolve", []C.wasmtime_component_val_t{
		*newStringValue("http"), *newStringValue(url),
	})
	if err != nil {
		return "", err
	}
	if !bool(C.rc_result_ok(&result)) {
		return string(valueString(C.rc_result_val(&result))), nil
	}
	artifact := C.rc_result_val(&result)
	name := valueString(recordField(artifact, "name"))
	digest := valueString(recordField(artifact, "digest"))
	if !strings.HasPrefix(digest, "sha256:") {
		return "", errString("missing artifact digest")
	}
	return name, nil
}
