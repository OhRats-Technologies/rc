//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import "unsafe"

func newStringValue(value string) *C.wasmtime_component_val_t {
	item := newValue()
	C.rc_set_string(item, (*C.uint8_t)(unsafe.Pointer(C.CString(value))), C.size_t(len(value)))
	return item
}

func newU32Value(value uint32) *C.wasmtime_component_val_t {
	item := newValue()
	C.rc_set_u32(item, C.uint32_t(value))
	return item
}

func newListValue(values []*C.wasmtime_component_val_t) *C.wasmtime_component_val_t {
	item := newValue()
	C.rc_alloc_list(item, C.size_t(len(values)))
	for index, value := range values {
		*C.rc_list_item(item, C.size_t(index)) = *value
	}
	return item
}

type fieldValue struct {
	name  string
	value *C.wasmtime_component_val_t
}

func newOrderedRecord(fields []fieldValue) *C.wasmtime_component_val_t {
	item := newValue()
	C.rc_alloc_record(item, C.size_t(len(fields)))
	for index, field := range fields {
		*C.rc_record_item(item, C.size_t(index), cString(field.name), C.size_t(len(field.name))) = *field.value
	}
	return item
}

func newRecordValue(fields map[string]*C.wasmtime_component_val_t) *C.wasmtime_component_val_t {
	item := newValue()
	C.rc_alloc_record(item, C.size_t(len(fields)))
	index := 0
	for name, value := range fields {
		*C.rc_record_item(item, C.size_t(index), cString(name), C.size_t(len(name))) = *value
		index++
	}
	return item
}

func newNoneOption() *C.wasmtime_component_val_t {
	item := newValue()
	C.rc_set_none_option(item)
	return item
}

func newEnumValue(name string) *C.wasmtime_component_val_t {
	item := newValue()
	C.rc_set_enum(item, cString(name), C.size_t(len(name)))
	return item
}

func newU64Value(value uint64) *C.wasmtime_component_val_t {
	item := newValue()
	C.rc_set_u64(item, C.uint64_t(value))
	return item
}

func newBoolValue(value bool) *C.wasmtime_component_val_t {
	item := newValue()
	C.rc_set_bool(item, C.bool(value))
	return item
}

func newBytesValue(value []byte) *C.wasmtime_component_val_t {
	item := newValue()
	C.rc_alloc_list(item, C.size_t(len(value)))
	for index, b := range value {
		C.rc_set_u8(C.rc_list_item(item, C.size_t(index)), C.uint8_t(b))
	}
	return item
}

func newVariantValue(name string, payload *C.wasmtime_component_val_t) *C.wasmtime_component_val_t {
	item := newValue()
	C.rc_set_variant(item, cString(name), C.size_t(len(name)), payload)
	return item
}

func newTupleValue(values []*C.wasmtime_component_val_t) *C.wasmtime_component_val_t {
	item := newValue()
	C.rc_alloc_tuple(item, C.size_t(len(values)))
	for index, value := range values {
		*C.rc_tuple_item(item, C.size_t(index)) = *value
	}
	return item
}
