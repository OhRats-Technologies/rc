//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unsafe"
)

// RunExtrasProbe exercises the environment, clock, and filesystem hosts.
func (h *Host) RunExtrasProbe() (string, error) {
	if h == nil || h.ctx == nil {
		return "", errors.New("host is unavailable")
	}
	var platform C.wasmtime_component_val_t
	environmentPlatform(h.ctx, nil, 0, &platform, 1)
	if goString(C.rc_variant_name(&platform), C.rc_variant_name_len(&platform)) == "" {
		return "", errors.New("host-platform returned no enum")
	}
	var now C.wasmtime_component_val_t
	clockNow(h.ctx, nil, 0, &now, 1)
	if uint64(C.rc_u64(&now)) == 0 {
		return "", errors.New("clock returned zero")
	}
	armArgs := []C.wasmtime_component_val_t{*newU64Value(uint64(time.Now().Add(20 * time.Millisecond).UnixMilli()))}
	var armResult C.wasmtime_component_val_t
	clockArm(h.ctx, &armArgs[0], 1, &armResult, 1)
	if !bool(C.rc_result_ok(&armResult)) {
		return "", errors.New(valueString(C.rc_result_val(&armResult)))
	}
	timer := C.rc_result_val(&armResult)
	rep, ty := resourceRep(h.ctx, timer)
	fresh, err := hostResource(h.ctx, rep, ty)
	if err != nil {
		return "", err
	}
	waitArgs := []C.wasmtime_component_val_t{*resourceValue(fresh)}
	var waitResult C.wasmtime_component_val_t
	timerWait(h.ctx, &waitArgs[0], 1, &waitResult, 1)
	if !bool(C.rc_result_ok(&waitResult)) {
		return "", errors.New("timer wait failed")
	}

	directory := filepath.Join(os.TempDir(), fmt.Sprintf("rc-capi-%d", time.Now().UnixNano()))
	path := filepath.Join(directory, "probe.txt")
	mkdirArgs := []C.wasmtime_component_val_t{*newStringValue(directory), *newBoolValue(true)}
	var mkdirResult C.wasmtime_component_val_t
	filesystemMkdir(h.ctx, &mkdirArgs[0], 2, &mkdirResult, 1)
	if !bool(C.rc_result_ok(&mkdirResult)) {
		return "", errors.New(valueString(C.rc_result_val(&mkdirResult)))
	}
	writeArgs := []C.wasmtime_component_val_t{*newStringValue(path), *newBytesValue([]byte("probe")), *newBoolValue(false)}
	var writeResult C.wasmtime_component_val_t
	filesystemWrite(h.ctx, &writeArgs[0], 3, &writeResult, 1)
	if !bool(C.rc_result_ok(&writeResult)) {
		return "", errors.New(valueString(C.rc_result_val(&writeResult)))
	}
	readArgs := []C.wasmtime_component_val_t{*newStringValue(path), *newU64Value(64)}
	var readResult C.wasmtime_component_val_t
	filesystemRead(h.ctx, &readArgs[0], 2, &readResult, 1)
	if !bool(C.rc_result_ok(&readResult)) {
		return "", errors.New(valueString(C.rc_result_val(&readResult)))
	}
	content := string(valueBytes(C.rc_result_val(&readResult)))
	removeArgs := []C.wasmtime_component_val_t{*newStringValue(directory), *newBoolValue(true)}
	var removeResult C.wasmtime_component_val_t
	filesystemRemove(h.ctx, &removeArgs[0], 2, &removeResult, 1)
	if content != "probe" {
		return "", fmt.Errorf("filesystem content = %q", content)
	}
	return "ok", nil
}

var _ = unsafe.Pointer(nil)
