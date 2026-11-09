//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

type meshRoute struct {
	destination string
	nextHop     string
	cost        uint64
	path        []string
}

type meshAdvertisement struct {
	origin    string
	sequence  uint64
	neighbors map[string]uint64
	services  map[string]uint64
}

func meshIdentifier(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < 33 || value[i] > 126 {
			return false
		}
	}
	return true
}

func meshVersions(value *C.wasmtime_component_val_t) ([]uint32, bool) {
	if value == nil {
		return nil, false
	}
	versions := make([]uint32, 0, int(C.rc_list_len(value)))
	previous := uint32(0)
	for index := 0; index < int(C.rc_list_len(value)); index++ {
		version := uint32(C.rc_u32(C.rc_list_get(value, C.size_t(index))))
		if version == 0 || (index > 0 && version <= previous) {
			return nil, false
		}
		previous = version
		versions = append(versions, version)
	}
	return versions, len(versions) > 0
}

func meshFeatureSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}
