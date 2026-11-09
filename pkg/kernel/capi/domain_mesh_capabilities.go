//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

func domainMeshNegotiate(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 2 {
		setRejection(results, "invalid-capability")
		return
	}
	local, remote := argValue(args, 0), argValue(args, 1)
	id := fieldString(local, "id")
	if !meshIdentifier(id) || id != fieldString(remote, "id") {
		setRejection(results, "invalid-capability")
		return
	}
	localVersions, ok := meshVersions(field(local, "versions"))
	if !ok {
		setRejection(results, "invalid-capability")
		return
	}
	remoteVersions, ok := meshVersions(field(remote, "versions"))
	if !ok {
		setRejection(results, "invalid-capability")
		return
	}
	remoteFeatures := meshFeatureSet(fieldStrings(remote, "features"))
	for _, feature := range fieldStrings(local, "required-features") {
		if !remoteFeatures[feature] {
			option := newValue()
			C.rc_set_none_option(option)
			setOK(results, option)
			return
		}
	}
	version := uint32(0)
	for _, candidate := range localVersions {
		for _, remoteVersion := range remoteVersions {
			if candidate == remoteVersion && candidate > version {
				version = candidate
			}
		}
	}
	if version == 0 {
		option := newValue()
		C.rc_set_none_option(option)
		setOK(results, option)
		return
	}
	features := make([]*C.wasmtime_component_val_t, 0)
	for _, feature := range fieldStrings(local, "supported-features") {
		if remoteFeatures[feature] {
			features = append(features, newStringValue(feature))
		}
	}
	negotiated := newOrderedRecord([]fieldValue{
		{name: "id", value: newStringValue(id)},
		{name: "version", value: newU32Value(version)},
		{name: "features", value: newListValue(features)},
	})
	option := newValue()
	C.rc_set_some_option(option, negotiated)
	setOK(results, option)
}
