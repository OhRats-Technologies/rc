//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

func domainIdentityUserCreate(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid identity user")
		return
	}
	id := valueString(argValue(args, 0))
	name, ok := identityName(valueString(argValue(args, 1)))
	if id == "" || !ok {
		setStringResult(results, false, "invalid identity user")
		return
	}
	var created identityUser
	err := mutateIdentity(func(state *identityDomain) error {
		if _, exists := state.Users[id]; exists {
			return domainError("identity user already exists")
		}
		created = identityUser{ID: id, DisplayName: name, CreatedAtMs: identityNow()}
		state.Users[id] = created
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, identityUserValue(created))
}

func domainIdentityUserGet(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid identity user")
		return
	}
	state, _, err := loadIdentity()
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	user, ok := state.Users[valueString(argValue(args, 0))]
	option := newValue()
	if !ok {
		C.rc_set_none_option(option)
	} else {
		C.rc_set_some_option(option, identityUserValue(user))
	}
	setOK(results, option)
}

func domainIdentityUserAll(_ *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t) {
	state, _, err := loadIdentity()
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	items := make([]*C.wasmtime_component_val_t, 0, len(state.Users))
	for _, user := range state.Users {
		items = append(items, identityUserValue(user))
	}
	setOK(results, newListValue(items))
}

func domainIdentityUserCount(_ *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t) {
	state, _, err := loadIdentity()
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	payload := newValue()
	C.rc_set_u64(payload, C.uint64_t(len(state.Users)))
	setOK(results, payload)
}

func domainIdentityUserRename(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid identity user")
		return
	}
	id := valueString(argValue(args, 0))
	name, ok := identityName(valueString(argValue(args, 1)))
	if id == "" || !ok {
		setStringResult(results, false, "invalid identity user")
		return
	}
	var renamed identityUser
	err := mutateIdentity(func(state *identityDomain) error {
		user, exists := state.Users[id]
		if !exists {
			return domainError("identity user not found")
		}
		user.DisplayName = name
		state.Users[id] = user
		renamed = user
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, identityUserValue(renamed))
}
