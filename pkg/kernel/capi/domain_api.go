//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"strconv"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
)

const apiProofWindowSeconds = 300

func domainAPICreate(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 6 {
		setStringResult(results, false, "invalid API credential")
		return
	}
	claim, err := consumeAuthorization(argValue(args, 0), "api-credential.create")
	if err != nil {
		setStringResult(results, false, "administrative proof is invalid")
		return
	}
	id, name := valueString(argValue(args, 1)), strings.TrimSpace(valueString(argValue(args, 2)))
	publicKey := valueString(argValue(args, 3))
	scopes, scopesOK := apiScopes(argValue(args, 4))
	expires, lifetimeOK := apiLifetimeFromOption(argValue(args, 5))
	if id == "" || len(id) > 128 || name == "" || len(name) > 120 ||
		!validAPIKey(publicKey) || !scopesOK || !lifetimeOK {
		setStringResult(results, false, "invalid API credential")
		return
	}
	var created apiCredentialRecord
	err = mutateAPI(func(state *apiDomainState) error {
		if _, exists := state.Credentials[id]; exists {
			return domainError("API credential already exists")
		}
		created = apiCredentialRecord{
			ID: id, UserID: claim.UserID, Kind: "api", Name: name, PublicKey: publicKey,
			Scopes: scopes, CreatedAtMs: identityNow(), ExpiresAtMs: expires,
		}
		state.Credentials[id] = created
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, apiCredentialValue(created))
}

func domainAPIAll(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	userID := ""
	if nargs > 0 {
		userID = valueString(argValue(args, 0))
	}
	state, _, err := domainLoad[apiDomainState](domainStore(), "api")
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	items := make([]*C.wasmtime_component_val_t, 0, len(state.Credentials))
	for _, credential := range state.Credentials {
		if userID != "" && credential.UserID != userID {
			continue
		}
		items = append(items, apiCredentialValue(credential))
	}
	setOK(results, newListValue(items))
}

func domainAPIGet(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid API credential")
		return
	}
	state, _, err := domainLoad[apiDomainState](domainStore(), "api")
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	option := newValue()
	credential, ok := state.Credentials[valueString(argValue(args, 0))]
	if !ok {
		C.rc_set_none_option(option)
	} else {
		C.rc_set_some_option(option, apiCredentialValue(credential))
	}
	setOK(results, option)
}

func domainAPIRevoke(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid API credential")
		return
	}
	claim, err := consumeAuthorization(argValue(args, 0), "api-credential.revoke")
	if err != nil {
		setStringResult(results, false, "administrative proof is invalid")
		return
	}
	id := valueString(argValue(args, 1))
	revoked := false
	err = mutateAPI(func(state *apiDomainState) error {
		credential, exists := state.Credentials[id]
		if !exists || credential.UserID != claim.UserID {
			return domainError("API credential not found")
		}
		if credential.RevokedAtMs != nil {
			return nil
		}
		now := identityNow()
		credential.RevokedAtMs = &now
		state.Credentials[id] = credential
		revoked = true
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	payload := newValue()
	C.rc_set_bool(payload, C.bool(revoked))
	setOK(results, payload)
}

func domainAPIVerify(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid API proof")
		return
	}
	request := argValue(args, 0)
	now := uint64(C.rc_u64(argValue(args, 1)))
	keyID := fieldString(request, "key-id")
	timestamp, err := strconv.ParseUint(fieldString(request, "timestamp-seconds"), 10, 64)
	if err != nil || now/1000 > timestamp+apiProofWindowSeconds || timestamp > now/1000+apiProofWindowSeconds {
		setStringResult(results, false, "API proof timestamp is invalid")
		return
	}
	nonce := fieldString(request, "nonce")
	if len(nonce) < 8 || len(nonce) > 128 {
		setStringResult(results, false, "API proof nonce is invalid")
		return
	}
	state, _, err := domainLoad[apiDomainState](domainStore(), "api")
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	credential, ok := state.Credentials[keyID]
	if !ok || credential.RevokedAtMs != nil || (credential.ExpiresAtMs != 0 && credential.ExpiresAtMs <= now) {
		setStringResult(results, false, "API credential is invalid")
		return
	}
	body := fieldBytes(request, "body")
	if err := crypto.VerifyApi(credential.PublicKey, fieldString(request, "signature"), crypto.ApiRequest{
		KeyID: keyID, Timestamp: fieldString(request, "timestamp-seconds"), Nonce: nonce,
		Method: fieldString(request, "method"), RequestURI: fieldString(request, "path-and-raw-query"),
		Body: body,
	}); err != nil {
		setStringResult(results, false, "API proof signature is invalid")
		return
	}
	nonceKey := keyID + "\x00" + nonce
	err = mutateAPI(func(state *apiDomainState) error {
		if expiry, seen := state.Nonces[nonceKey]; seen && expiry > now {
			return domainError("API proof nonce was replayed")
		}
		stored, exists := state.Credentials[keyID]
		if !exists || stored.RevokedAtMs != nil || (stored.ExpiresAtMs != 0 && stored.ExpiresAtMs <= now) {
			return domainError("API credential is invalid")
		}
		state.Nonces[nonceKey] = now + 600_000
		stored.LastUsedAtMs = &now
		state.Credentials[keyID] = stored
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	scopes := make([]*C.wasmtime_component_val_t, len(credential.Scopes))
	for index, scope := range credential.Scopes {
		scopes[index] = newEnumValue(scope)
	}
	setOK(results, newOrderedRecord([]fieldValue{
		{name: "credential-id", value: newStringValue(credential.ID)},
		{name: "user-id", value: newStringValue(credential.UserID)},
		{name: "kind", value: newEnumValue(credential.Kind)},
		{name: "scopes", value: newListValue(scopes)},
	}))
}
