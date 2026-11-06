//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"
)

const resourceTimer uint32 = 4

type timerResource struct {
	timer *time.Timer
}

func environmentPlatform(_ unsafe.Pointer, _ *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	name := "unix"
	if runtime.GOOS == "windows" {
		name = "windows"
	}
	C.rc_set_enum(results, cString(name), C.size_t(len(name)))
}

func environmentSnapshot(_ unsafe.Pointer, _ *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	environment := os.Environ()
	value := newValue()
	C.rc_alloc_list(value, C.size_t(len(environment)))
	for index, entry := range environment {
		name, content, _ := strings.Cut(entry, "=")
		tuple := C.rc_list_item(value, C.size_t(index))
		C.rc_alloc_tuple(tuple, 2)
		*C.rc_tuple_item(tuple, 0) = *newStringValue(name)
		*C.rc_tuple_item(tuple, 1) = *newStringValue(content)
	}
	C.rc_set_result(results, true, value)
}

func environmentFind(_ unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid find-executable request")
		return
	}
	path, err := exec.LookPath(valueString(argValue(args, 0)))
	option := newValue()
	if err != nil {
		C.rc_set_none_option(option)
	} else {
		C.rc_set_some_option(option, newStringValue(path))
	}
	C.rc_set_result(results, true, option)
}

func clockNow(_ unsafe.Pointer, _ *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	C.rc_set_u64(results, C.uint64_t(time.Now().UnixMilli()))
}

func clockArm(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid timer request")
		return
	}
	deadline := int64(C.rc_u64(argValue(args, 0)))
	delay := time.Until(time.UnixMilli(deadline))
	if delay < 0 {
		delay = 0
	}
	timer := &timerResource{timer: time.NewTimer(delay)}
	rep := addResourceObject(timer)
	resource, err := hostResource(ctx, rep, resourceTimer)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	payload := newValue()
	C.rc_set_resource(payload, resource)
	C.rc_set_result(results, true, payload)
}

func timerWait(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	timer, ok := selfTimer(ctx, args)
	if !ok {
		setStringResult(results, false, "unknown timer")
		return
	}
	<-timer.timer.C
	C.rc_set_result(results, true, nil)
}

func timerCancel(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if timer, ok := selfTimer(ctx, args); ok {
		timer.timer.Stop()
	}
}

func filesystemRead(_ unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid filesystem read")
		return
	}
	limit := int(C.rc_u64(argValue(args, 1)))
	if limit <= 0 || limit > 48<<20 {
		limit = 48 << 20
	}
	data, err := os.ReadFile(valueString(argValue(args, 0)))
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	if len(data) > limit {
		data = data[:limit]
	}
	value := newValue()
	setBytesValue(value, data)
	C.rc_set_result(results, true, value)
}

func filesystemWrite(_ unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 3 {
		setStringResult(results, false, "invalid filesystem write")
		return
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if bool(C.rc_bool(argValue(args, 2))) {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	file, err := os.OpenFile(valueString(argValue(args, 0)), flags, 0o600)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	_, err = file.Write(valueBytes(argValue(args, 1)))
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	C.rc_set_result(results, true, nil)
}

func filesystemList(_ unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid filesystem list")
		return
	}
	limit := int(C.rc_u32(argValue(args, 1)))
	if limit < 1 || limit > 4096 {
		limit = 4096
	}
	entries, err := os.ReadDir(valueString(argValue(args, 0)))
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	if len(entries) > limit {
		entries = entries[:limit]
	}
	value := newValue()
	C.rc_alloc_list(value, C.size_t(len(entries)))
	for index, entry := range entries {
		record := C.rc_list_item(value, C.size_t(index))
		C.rc_alloc_record(record, 2)
		*C.rc_record_item(record, 0, cString("name"), 4) = *newStringValue(entry.Name())
		kind := "other"
		switch {
		case entry.IsDir():
			kind = "directory"
		case entry.Type()&os.ModeSymlink != 0:
			kind = "symlink"
		case entry.Type().IsRegular():
			kind = "file"
		}
		enum := newEnumValue(kind)
		*C.rc_record_item(record, 1, cString("kind"), 4) = *enum
	}
	C.rc_set_result(results, true, value)
}

func filesystemMkdir(_ unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid create-directory")
		return
	}
	path := valueString(argValue(args, 0))
	recursive := bool(C.rc_bool(argValue(args, 1)))
	var err error
	if recursive {
		err = os.MkdirAll(path, 0o700)
	} else {
		err = os.Mkdir(path, 0o700)
	}
	setUnitResult(results, err)
}

func filesystemRemove(_ unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid remove")
		return
	}
	path := valueString(argValue(args, 0))
	var err error
	if bool(C.rc_bool(argValue(args, 1))) {
		err = os.RemoveAll(path)
	} else {
		err = os.Remove(path)
	}
	setUnitResult(results, err)
}

func filesystemRename(_ unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid rename")
		return
	}
	setUnitResult(results, os.Rename(valueString(argValue(args, 0)), valueString(argValue(args, 1))))
}

func filesystemCwd(_ unsafe.Pointer, _ *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	directory, err := os.Getwd()
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setStringOkResult(results, directory)
}

func selfTimer(ctx unsafe.Pointer, args *C.wasmtime_component_val_t) (*timerResource, bool) {
	rep, ok := selfRep(ctx, args)
	if !ok {
		return nil, false
	}
	timer, ok := lookupResource(rep).(*timerResource)
	return timer, ok
}

func setUnitResult(results *C.wasmtime_component_val_t, err error) {
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	C.rc_set_result(results, true, nil)
}

func setStringOkResult(results *C.wasmtime_component_val_t, value string) {
	C.rc_set_result(results, true, newStringValue(value))
}

func releaseTimer(timer *timerResource) {
	if timer != nil && timer.timer != nil {
		timer.timer.Stop()
	}
}

var _ = filepath.Separator
