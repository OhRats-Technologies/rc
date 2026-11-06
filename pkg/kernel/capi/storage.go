//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"sort"
	"sync"
	"sync/atomic"
	"unsafe"
)

type storageChange struct {
	put    bool
	bucket string
	key    []byte
	value  []byte
}

type storageFailure struct {
	revision uint64
	message  string
}

type storageBackend interface {
	revision() (uint64, error)
	get(bucket string, key []byte) ([]byte, bool, error)
	scan(bucket string, prefix, after []byte, limit int) (uint64, []entryValue, []byte, error)
	commit(expected uint64, changes []storageChange) (uint64, *storageFailure, error)
	close() error
}

type memoryStorage struct {
	mu      sync.Mutex
	rev     uint64
	buckets map[string]map[string][]byte
}

func (s *memoryStorage) revision() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rev, nil
}

func (s *memoryStorage) get(bucket string, key []byte) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.buckets[bucket][string(key)]
	return append([]byte(nil), value...), ok, nil
}

func (s *memoryStorage) scan(bucket string, prefix, after []byte, limit int) (uint64, []entryValue, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []string
	for key := range s.buckets[bucket] {
		raw := []byte(key)
		if len(prefix) > 0 && !hasPrefix(raw, prefix) {
			continue
		}
		if after != nil && !greater(raw, after) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	var next []byte
	if len(keys) > limit {
		next = []byte(keys[limit])
		keys = keys[:limit]
	}
	entries := make([]entryValue, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, entryValue{bucket: bucket, key: []byte(key), value: s.buckets[bucket][key]})
	}
	return s.rev, entries, next, nil
}

func (s *memoryStorage) commit(expected uint64, changes []storageChange) (uint64, *storageFailure, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if expected != s.rev {
		return s.rev, &storageFailure{revision: s.rev}, nil
	}
	for _, change := range changes {
		if s.buckets[change.bucket] == nil {
			s.buckets[change.bucket] = make(map[string][]byte)
		}
		if change.put {
			s.buckets[change.bucket][string(change.key)] = append([]byte(nil), change.value...)
		} else {
			delete(s.buckets[change.bucket], string(change.key))
		}
	}
	s.rev++
	return s.rev, nil, nil
}

func (s *memoryStorage) close() error { return nil }

var (
	storageRegistry sync.Map
	storageSequence atomic.Uint64
)

func newStorage(path string) (storageBackend, unsafe.Pointer, error) {
	var backend storageBackend
	var err error
	if path == "" {
		backend = &memoryStorage{buckets: make(map[string]map[string][]byte)}
	} else {
		backend, err = openSQLiteStorage(path)
		if err != nil {
			return nil, nil, err
		}
	}
	id := storageSequence.Add(1)
	storageRegistry.Store(id, backend)
	slot := C.malloc(8)
	*(*C.uint64_t)(slot) = C.uint64_t(id)
	return backend, slot, nil
}

func releaseStorage(slot unsafe.Pointer) {
	if slot == nil {
		return
	}
	id := uint64(*(*C.uint64_t)(slot))
	storageRegistry.Delete(id)
	C.free(slot)
}

func lookupStorage(data unsafe.Pointer) storageBackend {
	if data == nil {
		return nil
	}
	id := uint64(*(*C.uint64_t)(data))
	value, ok := storageRegistry.Load(id)
	if !ok {
		return nil
	}
	return value.(storageBackend)
}

func storageRevision(data unsafe.Pointer, _ *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) {
	backend := lookupStorage(data)
	if backend == nil || nresults < 1 {
		setStringResult(results, false, "storage host unavailable")
		return
	}
	revision, err := backend.revision()
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	payload := newValue()
	C.rc_set_u64(payload, C.uint64_t(revision))
	C.rc_set_result(results, true, payload)
}

func storageGet(data unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) {
	backend := lookupStorage(data)
	if backend == nil || nargs < 2 || nresults < 1 {
		setStringResult(results, false, "invalid storage request")
		return
	}
	value, ok, err := backend.get(valueString(argValue(args, 0)), valueBytes(argValue(args, 1)))
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	option := newValue()
	if ok {
		payload := newValue()
		setBytesValue(payload, value)
		C.rc_set_some_option(option, payload)
	} else {
		C.rc_set_none_option(option)
	}
	C.rc_set_result(results, true, option)
}

func storageScan(data unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) {
	backend := lookupStorage(data)
	if backend == nil || nargs < 4 || nresults < 1 {
		setStringResult(results, false, "invalid storage scan")
		return
	}
	bucket := valueString(argValue(args, 0))
	prefix := valueBytes(argValue(args, 1))
	var after []byte
	if option := C.rc_option_get(argValue(args, 2)); option != nil {
		after = valueBytes(option)
	}
	limit := int(C.rc_u32(argValue(args, 3)))
	if limit < 1 {
		limit = 1
	}
	if limit > 10_000 {
		limit = 10_000
	}
	revision, entries, next, err := backend.scan(bucket, prefix, after, limit)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	page := newValue()
	C.rc_alloc_record(page, 3)
	C.rc_set_u64(C.rc_record_item(page, 0, cString("revision"), 8), C.uint64_t(revision))
	entriesVal := C.rc_record_item(page, 1, cString("entries"), 7)
	C.rc_alloc_list(entriesVal, C.size_t(len(entries)))
	for index, entry := range entries {
		setEntry(C.rc_list_item(entriesVal, C.size_t(index)), entry)
	}
	nextVal := C.rc_record_item(page, 2, cString("next-key"), 8)
	option := newValue()
	if next != nil {
		payload := newValue()
		setBytesValue(payload, next)
		C.rc_set_some_option(option, payload)
	} else {
		C.rc_set_none_option(option)
	}
	*nextVal = *option
	C.rc_set_result(results, true, page)
}

func storageCommit(data unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) {
	backend := lookupStorage(data)
	if backend == nil || nargs < 2 || nresults < 1 {
		setCommitFailure(results, "invalid storage commit")
		return
	}
	expected := uint64(C.rc_u64(argValue(args, 0)))
	changesVal := argValue(args, 1)
	var changes []storageChange
	for index := 0; index < int(C.rc_list_len(changesVal)); index++ {
		change := C.rc_list_get(changesVal, C.size_t(index))
		name := goString(C.rc_variant_name(change), C.rc_variant_name_len(change))
		payload := C.rc_variant_val(change)
		if payload == nil {
			continue
		}
		entry := storageChange{
			put:    name == "put",
			bucket: valueString(recordField(payload, "bucket")),
			key:    valueBytes(recordField(payload, "key")),
			value:  valueBytes(recordField(payload, "value")),
		}
		changes = append(changes, entry)
	}
	revision, failure, err := backend.commit(expected, changes)
	if err != nil {
		setCommitFailure(results, err.Error())
		return
	}
	if failure != nil {
		if failure.message != "" {
			setCommitFailure(results, failure.message)
		} else {
			setCommitConflict(results, revision)
		}
		return
	}
	payload := newValue()
	C.rc_set_u64(payload, C.uint64_t(revision))
	C.rc_set_result(results, true, payload)
}

type entryValue struct {
	bucket string
	key    []byte
	value  []byte
}
