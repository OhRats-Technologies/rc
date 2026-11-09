//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"
)

type domainState struct {
	backend storageBackend
	mu      sync.Mutex
}

var (
	domainRegistry  sync.Map
	domainSequence  atomic.Uint64
	domainBindingMu sync.Mutex
	domainBindings  = make(map[uint64][]unsafe.Pointer)
)

func newDomainSlot(backend storageBackend) unsafe.Pointer {
	id := domainSequence.Add(1)
	domainRegistry.Store(id, &domainState{backend: backend})
	slot := C.malloc(8)
	*(*C.uint64_t)(slot) = C.uint64_t(id)
	return slot
}

func releaseDomainSlot(slot unsafe.Pointer) {
	if slot == nil {
		return
	}
	id := uint64(*(*C.uint64_t)(slot))
	domainRegistry.Delete(id)
	domainBindingMu.Lock()
	for _, binding := range domainBindings[id] {
		C.free(binding)
	}
	delete(domainBindings, id)
	domainBindingMu.Unlock()
	C.free(slot)
}

// newDomainBinding stores the C callback binding until the host closes so the
// linker never holds a dangling name pointer.
func newDomainBinding(id uint64, handler string) unsafe.Pointer {
	binding := unsafe.Pointer(C.CString(fmt.Sprintf("%d|%s", id, handler)))
	domainBindingMu.Lock()
	domainBindings[id] = append(domainBindings[id], binding)
	domainBindingMu.Unlock()
	return binding
}

func lookupDomainID(id uint64) *domainState {
	value, ok := domainRegistry.Load(id)
	if !ok {
		return nil
	}
	return value.(*domainState)
}

// domainRead reads one domain bucket with the storage revision taken before
// the value so a concurrent commit can never be silently overwritten.
func domainRead(d *domainState, bucket string) ([]byte, uint64, error) {
	revision, err := d.backend.revision()
	if err != nil {
		return nil, 0, err
	}
	raw, ok, err := d.backend.get("rc-domain", []byte(bucket))
	if err != nil {
		return nil, 0, err
	}
	if !ok {
		raw = nil
	}
	return raw, revision, nil
}

func (d *domainState) save(bucket string, value any, revision uint64) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	next, failure, err := d.backend.commit(revision, []storageChange{{
		put: true, bucket: "rc-domain", key: []byte(bucket), value: raw,
	}})
	_ = next
	if failure != nil {
		return errDomainConflict
	}
	return err
}

func domainLoad[T any](d *domainState, bucket string) (T, uint64, error) {
	var value T
	raw, revision, err := domainRead(d, bucket)
	if err != nil {
		return value, 0, err
	}
	if raw != nil {
		if err := json.Unmarshal(raw, &value); err != nil {
			return value, 0, err
		}
	}
	return value, revision, nil
}

// domainMutate applies one bounded read-modify-write transaction. Competing
// component hosts retry against the latest storage revision instead of losing
// a nonce, session, or event cursor update.
func domainMutate[T any](d *domainState, bucket string, fn func(*T) error) (T, error) {
	var zero T
	for attempt := 0; attempt < 8; attempt++ {
		value, revision, err := domainLoad[T](d, bucket)
		if err != nil {
			return zero, err
		}
		if err := fn(&value); err != nil {
			return zero, err
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return zero, err
		}
		_, failure, err := d.backend.commit(revision, []storageChange{{
			put: true, bucket: "rc-domain", key: []byte(bucket), value: raw,
		}})
		if err != nil {
			return zero, err
		}
		if failure == nil {
			return value, nil
		}
	}
	return zero, errDomainConflict
}
