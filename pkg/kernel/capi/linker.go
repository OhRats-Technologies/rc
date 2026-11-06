//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"fmt"
	"reflect"
	"unsafe"
)

func addInstance(root unsafe.Pointer, name string) (unsafe.Pointer, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	var out unsafe.Pointer
	if errPtr := C.wasmtime_component_linker_instance_add_instance(root, cname, C.size_t(len(name)), &out); errPtr != nil {
		return nil, takeError(errPtr)
	}
	if out == nil {
		return nil, fmt.Errorf("could not define import instance %q", name)
	}
	return out, nil
}

func addFunc(instance unsafe.Pointer, name string, callback C.rc_component_func_callback_t) error {
	return addFuncData(instance, name, callback, nil)
}

func addFuncData(instance unsafe.Pointer, name string, callback C.rc_component_func_callback_t, data unsafe.Pointer) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	if errPtr := C.wasmtime_component_linker_instance_add_func(instance, cname, C.size_t(len(name)), callback, data, nil); errPtr != nil {
		return takeError(errPtr)
	}
	return nil
}

func takeError(pointer unsafe.Pointer) error {
	var message C.wasm_name_t
	C.wasmtime_error_message(pointer, &message)
	text := goString((*C.uint8_t)(unsafe.Pointer(message.data)), message.size)
	C.wasm_byte_vec_delete(&message)
	C.wasmtime_error_delete(pointer)
	return fmt.Errorf("%s", text)
}

func valueString(value *C.wasmtime_component_val_t) string {
	if value == nil {
		return ""
	}
	return goString(C.rc_string_data(value), C.rc_string_len(value))
}

func goString(data *C.uint8_t, length C.size_t) string {
	if data == nil || length == 0 {
		return ""
	}
	return C.GoStringN((*C.char)(unsafe.Pointer(data)), C.int(length))
}

func pointerField(value any, name string) unsafe.Pointer {
	field := reflect.ValueOf(value).Elem().FieldByName(name)
	if !field.IsValid() || field.Kind() != reflect.Ptr {
		return nil
	}
	return unsafe.Pointer(field.Pointer())
}

func addResource(instance unsafe.Pointer, name string, ty uint32) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	resourceType := C.wasmtime_component_resource_type_new_host(C.uint32_t(ty))
	if resourceType == nil {
		return fmt.Errorf("could not create resource type %q", name)
	}
	defer C.wasmtime_component_resource_type_delete(resourceType)
	if errPtr := C.wasmtime_component_linker_instance_add_resource(instance, cname, C.size_t(len(name)), resourceType, C.rc_resource_destructor(), nil, nil); errPtr != nil {
		return takeError(errPtr)
	}
	return nil
}
