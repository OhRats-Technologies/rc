//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import "unsafe"

func setEntry(target *C.wasmtime_component_val_t, entry entryValue) {
	C.rc_alloc_record(target, 3)
	C.rc_set_string(C.rc_record_item(target, 0, cString("bucket"), 6), (*C.uint8_t)(unsafe.Pointer(C.CString(entry.bucket))), C.size_t(len(entry.bucket)))
	keyVal := C.rc_record_item(target, 1, cString("key"), 3)
	setBytesValue(keyVal, entry.key)
	valueVal := C.rc_record_item(target, 2, cString("value"), 5)
	setBytesValue(valueVal, entry.value)
}

func setBytesValue(target *C.wasmtime_component_val_t, value []byte) {
	C.rc_alloc_list(target, C.size_t(len(value)))
	for index, item := range value {
		C.rc_set_u8(C.rc_list_item(target, C.size_t(index)), C.uint8_t(item))
	}
}

func valueBytes(value *C.wasmtime_component_val_t) []byte {
	if value == nil {
		return nil
	}
	length := int(C.rc_list_len(value))
	result := make([]byte, length)
	for index := 0; index < length; index++ {
		result[index] = byte(C.rc_u8(C.rc_list_get(value, C.size_t(index))))
	}
	return result
}

func recordField(value *C.wasmtime_component_val_t, name string) *C.wasmtime_component_val_t {
	if value == nil {
		return nil
	}
	for index := 0; index < int(C.rc_record_len(value)); index++ {
		if goString(C.rc_record_key(value, C.size_t(index)), C.rc_record_key_len(value, C.size_t(index))) == name {
			return C.rc_record_val(value, C.size_t(index))
		}
	}
	return nil
}

func setStringResult(results *C.wasmtime_component_val_t, ok bool, message string) {
	payload := newValue()
	C.rc_set_string(payload, (*C.uint8_t)(unsafe.Pointer(C.CString(message))), C.size_t(len(message)))
	C.rc_set_result(results, C.bool(ok), payload)
}

func setCommitConflict(results *C.wasmtime_component_val_t, revision uint64) {
	payload := newValue()
	C.rc_set_u64(payload, C.uint64_t(revision))
	variant := newValue()
	C.rc_set_variant(variant, cString("conflict"), 8, payload)
	C.rc_set_result(results, false, variant)
}

func setCommitFailure(results *C.wasmtime_component_val_t, message string) {
	payload := newValue()
	C.rc_set_string(payload, (*C.uint8_t)(unsafe.Pointer(C.CString(message))), C.size_t(len(message)))
	variant := newValue()
	C.rc_set_variant(variant, cString("failure"), 7, payload)
	C.rc_set_result(results, false, variant)
}

func argValue(args *C.wasmtime_component_val_t, index int) *C.wasmtime_component_val_t {
	return (*C.wasmtime_component_val_t)(unsafe.Add(unsafe.Pointer(args), uintptr(index)*unsafe.Sizeof(C.wasmtime_component_val_t{})))
}

func newValue() *C.wasmtime_component_val_t {
	return (*C.wasmtime_component_val_t)(C.calloc(1, C.size_t(unsafe.Sizeof(C.wasmtime_component_val_t{}))))
}

func cString(value string) *C.char { return C.CString(value) }

func hasPrefix(value, prefix []byte) bool {
	if len(prefix) > len(value) {
		return false
	}
	for i := range prefix {
		if value[i] != prefix[i] {
			return false
		}
	}
	return true
}

func greater(value, after []byte) bool {
	for i := 0; i < len(value) && i < len(after); i++ {
		if value[i] != after[i] {
			return value[i] > after[i]
		}
	}
	return len(value) > len(after)
}
