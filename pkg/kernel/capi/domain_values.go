//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

func field(value *C.wasmtime_component_val_t, name string) *C.wasmtime_component_val_t {
	return recordField(value, name)
}

func fieldString(value *C.wasmtime_component_val_t, name string) string {
	return valueString(field(value, name))
}

func fieldBytes(value *C.wasmtime_component_val_t, name string) []byte {
	return valueBytes(field(value, name))
}

func fieldU32(value *C.wasmtime_component_val_t, name string) uint32 {
	item := field(value, name)
	if item == nil {
		return 0
	}
	return uint32(C.rc_u32(item))
}

func fieldU8(value *C.wasmtime_component_val_t, name string) uint8 {
	item := field(value, name)
	if item == nil {
		return 0
	}
	return uint8(C.rc_u8(item))
}

func newU8Value(value uint8) *C.wasmtime_component_val_t {
	item := newValue()
	C.rc_set_u8(item, C.uint8_t(value))
	return item
}

func fieldU64(value *C.wasmtime_component_val_t, name string) uint64 {
	item := field(value, name)
	if item == nil {
		return 0
	}
	return uint64(C.rc_u64(item))
}

func fieldBool(value *C.wasmtime_component_val_t, name string) bool {
	item := field(value, name)
	return item != nil && bool(C.rc_bool(item))
}

func fieldStrings(value *C.wasmtime_component_val_t, name string) []string {
	return parseStrings(field(value, name))
}

func fieldList(value *C.wasmtime_component_val_t, name string) *C.wasmtime_component_val_t {
	return field(value, name)
}

func optionString(value *C.wasmtime_component_val_t) (string, bool) {
	if value == nil {
		return "", false
	}
	item := C.rc_option_get(value)
	if item == nil {
		return "", false
	}
	return valueString(item), true
}

func variantName(value *C.wasmtime_component_val_t) string {
	if value == nil {
		return ""
	}
	return goString(C.rc_variant_name(value), C.rc_variant_name_len(value))
}

func variantValue(value *C.wasmtime_component_val_t) *C.wasmtime_component_val_t {
	if value == nil {
		return nil
	}
	return C.rc_variant_val(value)
}

func setOK(results *C.wasmtime_component_val_t, payload *C.wasmtime_component_val_t) {
	C.rc_set_result(results, true, payload)
}

func setRejection(results *C.wasmtime_component_val_t, tag string) {
	C.rc_set_result(results, false, newVariantValue(tag, nil))
}

func setOKStringList(results *C.wasmtime_component_val_t, values []string) {
	items := make([]*C.wasmtime_component_val_t, len(values))
	for index, value := range values {
		items[index] = newStringValue(value)
	}
	setOK(results, newListValue(items))
}

func recordString(value *C.wasmtime_component_val_t, name, text string) *C.wasmtime_component_val_t {
	return newOrderedRecord([]fieldValue{{name: name, value: newStringValue(text)}})
}

func optionStringValue(value string) *C.wasmtime_component_val_t {
	if value == "" {
		return newNoneOption()
	}
	option := newValue()
	C.rc_set_some_option(option, newStringValue(value))
	return option
}

func optionBytesValue(value []byte) *C.wasmtime_component_val_t {
	option := newValue()
	if value == nil {
		C.rc_set_none_option(option)
		return option
	}
	C.rc_set_some_option(option, newBytesValue(value))
	return option
}
