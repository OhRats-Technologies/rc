//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

func domainIdentityCeremonyPut(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid identity ceremony")
		return
	}
	value := argValue(args, 0)
	ceremony := identityCeremony{
		ID: fieldString(value, "id"), Kind: fieldString(value, "kind"),
		Metadata: fieldBytes(value, "metadata"), State: fieldBytes(value, "state"),
		ExpiresAtMs: fieldU64(value, "expires-at-ms"),
	}
	if userID, ok := optionString(field(value, "user-id")); ok {
		ceremony.UserID = &userID
	}
	if ceremony.ID == "" || ceremony.Kind == "" || ceremony.ExpiresAtMs == 0 {
		setStringResult(results, false, "invalid identity ceremony")
		return
	}
	err := mutateIdentity(func(state *identityDomain) error {
		state.Ceremonies[ceremony.ID] = ceremony
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, nil)
}

func domainIdentityCeremonyTake(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid identity ceremony")
		return
	}
	id, kind := valueString(argValue(args, 0)), valueString(argValue(args, 1))
	var found identityCeremony
	option := newValue()
	err := mutateIdentity(func(state *identityDomain) error {
		ceremony, exists := state.Ceremonies[id]
		if !exists || ceremony.Kind != kind {
			C.rc_set_none_option(option)
			return nil
		}
		delete(state.Ceremonies, id)
		if ceremony.ExpiresAtMs <= identityNow() {
			C.rc_set_none_option(option)
			return nil
		}
		found = ceremony
		C.rc_set_some_option(option, identityCeremonyValue(found))
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, option)
}
