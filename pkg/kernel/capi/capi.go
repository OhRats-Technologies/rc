//go:build cgo && !windows

package capi

/*
#cgo LDFLAGS: -lwasmtime
#include <stdlib.h>
#include "capi.h"
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/bytecodealliance/wasmtime-go/v50"
)

// functionHandleSize is deliberately larger than the header-reported
// wasmtime_component_func_t so the installed Wasmtime ABI cannot write past the
// allocation when returning a component function handle.
const functionHandleSize = 256

// Host owns one instantiated component and its host linker.
type Host struct {
	store       *wasmtime.Store
	component   *wasmtime.Component
	linker      *wasmtime.ComponentLinker
	ctx         unsafe.Pointer
	instance    *wasmtime.ComponentInstance
	instancePtr unsafe.Pointer
	storageSlot unsafe.Pointer
	storage     storageBackend
	netSlot     unsafe.Pointer
}

// New compiles and instantiates one component with the RC plugin host imports.
func New(engine *wasmtime.Engine, wasm []byte) (*Host, error) {
	return NewWithStorage(engine, wasm, "")
}

// NewWithStorage instantiates a component using a persistent storage path.
// An empty path uses process-local in-memory storage.
func NewWithStorage(engine *wasmtime.Engine, wasm []byte, storagePath string) (*Host, error) {
	return NewWithNet(engine, wasm, storagePath, nil)
}

// NewWithNet instantiates a component with an exact endpoint allowlist.
func NewWithNet(engine *wasmtime.Engine, wasm []byte, storagePath string, endpoints []string) (*Host, error) {
	component, err := wasmtime.NewComponent(engine, wasm)
	if err != nil {
		return nil, err
	}
	store := wasmtime.NewStore(engine)
	host := &Host{store: store, component: component}
	host.ctx = C.wasmtime_store_context(pointerField(store, "_ptr"))
	if host.ctx == nil {
		host.Close()
		return nil, fmt.Errorf("could not resolve store context")
	}
	host.linker = wasmtime.NewComponentLinker(engine)
	linkerPtr := pointerField(host.linker, "_ptr")
	root := C.wasmtime_component_linker_root(linkerPtr)
	if root == nil {
		host.Close()
		return nil, fmt.Errorf("could not open component linker root")
	}
	callContext, err := addInstance(root, "ohrats:rc-plugin/call-context@0.1.0")
	if err == nil {
		err = addFunc(callContext, "caller-component-id", C.rc_caller_callback())
	}
	if err == nil {
		var pluginHost unsafe.Pointer
		pluginHost, err = addInstance(root, "ohrats:rc-plugin/host@0.1.0")
		if err == nil {
			err = addFunc(pluginHost, "log", C.rc_log_callback())
		}
		if pluginHost != nil {
			C.wasmtime_component_linker_instance_delete(pluginHost)
		}
	}
	if err == nil {
		var storageErr error
		host.netSlot = newNetHost(endpoints)
		host.storage, host.storageSlot, storageErr = newStorage(storagePath)
		if storageErr != nil {
			host.Close()
			return nil, storageErr
		}
		var storage unsafe.Pointer
		storage, err = addInstance(root, "ohrats:rc-storage/durable-store@0.1.0")
		if err == nil {
			for name, callback := range map[string]C.rc_component_func_callback_t{
				"revision": C.rc_storage_revision_callback(),
				"get":      C.rc_storage_get_callback(),
				"scan":     C.rc_storage_scan_callback(),
				"commit":   C.rc_storage_commit_callback(),
			} {
				if err = addFuncData(storage, name, callback, host.storageSlot); err != nil {
					break
				}
			}
		}
		if storage != nil {
			C.wasmtime_component_linker_instance_delete(storage)
		}
	}
	if callContext != nil {
		C.wasmtime_component_linker_instance_delete(callContext)
	}
	if err != nil {
		host.Close()
		return nil, err
	}
	wasi := C.wasi_config_new()
	if wasi == nil {
		host.Close()
		return nil, fmt.Errorf("could not create WASI configuration")
	}
	C.wasi_config_inherit_stdout(wasi)
	C.wasi_config_inherit_stderr(wasi)
	if errPtr := C.wasmtime_context_set_wasi(host.ctx, wasi); errPtr != nil {
		host.Close()
		return nil, takeError(errPtr)
	}
	if errPtr := C.wasmtime_component_linker_add_wasip2(linkerPtr); errPtr != nil {
		host.Close()
		return nil, takeError(errPtr)
	}
	if err == nil {
		err = linkExtraHosts(root, host.netSlot)
	}
	C.wasmtime_component_linker_instance_delete(root)
	if err != nil {
		host.Close()
		return nil, err
	}
	if err := host.linker.DefineUnknownImportsAsTraps(component); err != nil {
		host.Close()
		return nil, err
	}
	instance, err := host.linker.Instantiate(store, component)
	if err != nil {
		host.Close()
		return nil, err
	}
	host.instance = instance
	host.instancePtr = unsafe.Pointer(instance)
	runtime.KeepAlive(host.instance)
	runtime.KeepAlive(component)
	runtime.KeepAlive(store)
	return host, nil
}

// Close releases the store and linker.
func (h *Host) Close() {
	if h == nil {
		return
	}
	if h.linker != nil {
		h.linker.Close()
		h.linker = nil
	}
	if h.store != nil {
		h.store.Close()
		h.store = nil
	}
	if h.component != nil {
		h.component.Close()
		h.component = nil
	}
	releaseStorage(h.storageSlot)
	h.storageSlot = nil
	if h.storage != nil {
		_ = h.storage.close()
		h.storage = nil
	}
	releaseNetHost(h.netSlot)
	h.netSlot = nil
}

// Descriptor calls the component descriptor export.
func (h *Host) Descriptor() (Descriptor, error) {
	var value C.wasmtime_component_val_t
	if err := h.call("descriptor", nil, &value); err != nil {
		return Descriptor{}, err
	}
	defer C.wasmtime_component_val_delete(&value)
	if C.rc_val_kind(&value) != 14 {
		return Descriptor{}, fmt.Errorf("descriptor is not a record")
	}
	descriptor := Descriptor{}
	for i := 0; i < int(C.rc_record_len(&value)); i++ {
		key := goString(C.rc_record_key(&value, C.size_t(i)), C.rc_record_key_len(&value, C.size_t(i)))
		field := C.rc_record_val(&value, C.size_t(i))
		switch key {
		case "id":
			descriptor.ID = valueString(field)
		case "version":
			descriptor.Version = valueString(field)
		case "provides":
			descriptor.Provides = parseServices(field)
		case "requires":
			descriptor.Requires = parseRequirements(field)
		case "commands":
			descriptor.Commands = parseCommands(field)
		}
	}
	return descriptor, nil
}

// Activate calls the component activate export.
func (h *Host) Activate() error {
	var value C.wasmtime_component_val_t
	if err := h.call("activate", nil, &value); err != nil {
		return err
	}
	defer C.wasmtime_component_val_delete(&value)
	if C.rc_val_kind(&value) != 19 {
		return fmt.Errorf("activate did not return a result")
	}
	if !bool(C.rc_result_ok(&value)) {
		return fmt.Errorf("activation failed: %s", valueString(C.rc_result_val(&value)))
	}
	return nil
}

func (h *Host) call(name string, args []C.wasmtime_component_val_t, result *C.wasmtime_component_val_t) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	context := C.wasmtime_store_context(pointerField(h.store, "_ptr"))
	if context == nil {
		return fmt.Errorf("store context is unavailable")
	}
	exportIndex := h.instance.GetExportIndex(h.store, nil, name)
	if exportIndex == nil {
		return fmt.Errorf("component export %q was not found", name)
	}
	index := pointerField(exportIndex, "_ptr")
	function := C.malloc(functionHandleSize)
	defer C.free(function)
	if !bool(C.wasmtime_component_instance_get_func(h.instancePtr, context, index, function)) {
		return fmt.Errorf("component export %q was not found", name)
	}
	var argsPtr *C.wasmtime_component_val_t
	if len(args) > 0 {
		argsPtr = &args[0]
	}
	if errPtr := C.wasmtime_component_func_call(function, context, argsPtr, C.size_t(len(args)), result, 1); errPtr != nil {
		return takeError(errPtr)
	}
	return nil
}
