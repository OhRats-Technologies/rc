//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

func domainIdentityCredentialCreateUser(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 4 {
		setStringResult(results, false, "invalid identity credential")
		return
	}
	id := valueString(argValue(args, 0))
	display, ok := identityName(valueString(argValue(args, 1)))
	passkeyName, nameOK := identityName(valueString(argValue(args, 2)))
	credential := parseIdentityCredential(argValue(args, 3))
	if id == "" || !ok || !nameOK || !validIdentityCredential(credential) {
		setStringResult(results, false, "invalid identity credential")
		return
	}
	var created identityUser
	err := mutateIdentity(func(state *identityDomain) error {
		if _, exists := state.Users[id]; exists {
			return domainError("identity user already exists")
		}
		key := credentialKey(credential.ID)
		if _, exists := state.CredentialIDs[key]; exists {
			return domainError("identity credential already exists")
		}
		created = identityUser{ID: id, DisplayName: display, CreatedAtMs: identityNow()}
		state.Users[id] = created
		passkey := identityPasskey{
			ID: identityToken("pk_"), UserID: id, Name: passkeyName,
			Credential: credential, CreatedAtMs: created.CreatedAtMs,
		}
		state.Passkeys[passkey.ID] = passkey
		state.CredentialIDs[key] = passkey.ID
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, identityUserValue(created))
}

func domainIdentityCredentialAdd(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 3 {
		setStringResult(results, false, "invalid identity credential")
		return
	}
	userID := valueString(argValue(args, 0))
	name, ok := identityName(valueString(argValue(args, 1)))
	credential := parseIdentityCredential(argValue(args, 2))
	if userID == "" || !ok || !validIdentityCredential(credential) {
		setStringResult(results, false, "invalid identity credential")
		return
	}
	var created identityPasskey
	err := mutateIdentity(func(state *identityDomain) error {
		if _, exists := state.Users[userID]; !exists {
			return domainError("identity user not found")
		}
		key := credentialKey(credential.ID)
		if _, exists := state.CredentialIDs[key]; exists {
			return domainError("identity credential already exists")
		}
		created = identityPasskey{
			ID: identityToken("pk_"), UserID: userID, Name: name,
			Credential: credential, CreatedAtMs: identityNow(),
		}
		state.Passkeys[created.ID] = created
		state.CredentialIDs[key] = created.ID
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, identityPasskeyValue(created))
}

func domainIdentityCredentialGet(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid identity credential")
		return
	}
	state, _, err := loadIdentity()
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	passkey, ok := state.Passkeys[state.CredentialIDs[credentialKey(valueBytes(argValue(args, 0)))]]
	option := newValue()
	if !ok {
		C.rc_set_none_option(option)
	} else {
		C.rc_set_some_option(option, identityPasskeyValue(passkey))
	}
	setOK(results, option)
}

func domainIdentityCredentialAll(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	state, _, err := loadIdentity()
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	filter, filtered := "", false
	if nargs > 0 {
		filter, filtered = optionString(argValue(args, 0))
	}
	items := make([]*C.wasmtime_component_val_t, 0, len(state.Passkeys))
	for _, passkey := range state.Passkeys {
		if filtered && passkey.UserID != filter {
			continue
		}
		items = append(items, identityPasskeyValue(passkey))
	}
	setOK(results, newListValue(items))
}

func domainIdentityCredentialUpdate(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 3 {
		setStringResult(results, false, "invalid identity credential")
		return
	}
	id := valueString(argValue(args, 0))
	credential := parseIdentityCredential(argValue(args, 1))
	usedAt := uint64(C.rc_u64(argValue(args, 2)))
	if id == "" || !validIdentityCredential(credential) || usedAt == 0 {
		setStringResult(results, false, "invalid identity credential")
		return
	}
	var updated identityPasskey
	err := mutateIdentity(func(state *identityDomain) error {
		passkey, exists := state.Passkeys[id]
		if !exists {
			return domainError("identity credential not found")
		}
		delete(state.CredentialIDs, credentialKey(passkey.Credential.ID))
		passkey.Credential = credential
		passkey.LastUsedAtMs = &usedAt
		state.Passkeys[id] = passkey
		state.CredentialIDs[credentialKey(credential.ID)] = id
		updated = passkey
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, identityPasskeyValue(updated))
}

func domainIdentityCredentialRemove(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid identity credential")
		return
	}
	id, userID := valueString(argValue(args, 0)), valueString(argValue(args, 1))
	removed := false
	err := mutateIdentity(func(state *identityDomain) error {
		passkey, exists := state.Passkeys[id]
		if !exists || passkey.UserID != userID {
			return nil
		}
		delete(state.CredentialIDs, credentialKey(passkey.Credential.ID))
		delete(state.Passkeys, id)
		removed = true
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	payload := newValue()
	C.rc_set_bool(payload, C.bool(removed))
	setOK(results, payload)
}
