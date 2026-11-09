//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

func domainAPIStartCLI(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 7 {
		setStringResult(results, false, "invalid CLI authorization")
		return
	}
	clientID := valueString(argValue(args, 0))
	publicKey := valueString(argValue(args, 1))
	expires, lifetimeOK := apiLifetimeFromOption(argValue(args, 2))
	requestID := valueString(argValue(args, 3))
	deviceCode, userCode := valueString(argValue(args, 4)), valueString(argValue(args, 5))
	now := uint64(C.rc_u64(argValue(args, 6)))
	if clientID == "" || requestID == "" || deviceCode == "" || userCode == "" ||
		!validAPIKey(publicKey) || !lifetimeOK {
		setStringResult(results, false, "invalid CLI authorization")
		return
	}
	if expires == 0 {
		expires = now + uint64(24*60*60*1000)
	}
	record := apiCLIRecord{
		RequestID: requestID, ClientID: clientID, PublicKey: publicKey,
		DeviceCode: deviceCode, UserCode: userCode, ExpiresAtMs: expires, Interval: 5,
	}
	err := mutateAPI(func(state *apiDomainState) error {
		if _, exists := state.CLI[requestID]; exists {
			return domainError("CLI authorization already exists")
		}
		state.CLI[requestID] = record
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, newOrderedRecord([]fieldValue{
		{name: "request-id", value: newStringValue(requestID)},
		{name: "client-id", value: newStringValue(clientID)},
		{name: "expires-at-ms", value: newU64Value(expires)},
		{name: "interval-seconds", value: newU32Value(record.Interval)},
		{name: "verification-url", value: newStringValue("https://rc.ohrats.party/cli/authorize?request=" + requestID)},
	}))
}

func domainAPIApproveCLI(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 4 {
		setStringResult(results, false, "invalid CLI authorization")
		return
	}
	claim, err := consumeAuthorization(argValue(args, 0), "api-credential.cli-approve")
	if err != nil {
		setStringResult(results, false, "administrative proof is invalid")
		return
	}
	requestID, userCode := valueString(argValue(args, 1)), valueString(argValue(args, 2))
	publicKey := valueString(argValue(args, 3))
	if !validAPIKey(publicKey) {
		setStringResult(results, false, "invalid CLI authorization")
		return
	}
	var approved apiCredentialRecord
	err = mutateAPI(func(state *apiDomainState) error {
		record, exists := state.CLI[requestID]
		if !exists || record.ExpiresAtMs <= identityNow() {
			return domainError("CLI authorization is invalid or expired")
		}
		if record.UserCode != userCode || record.ClientID != claim.BrowserClientID {
			return domainError("CLI authorization does not match")
		}
		if record.Approved {
			return domainError("CLI authorization was already approved")
		}
		record.Approved = true
		record.PublicKey = publicKey
		record.Credential = apiCredentialRecord{
			ID: record.ClientID, UserID: claim.UserID, Kind: "cli", Name: "CLI authorization",
			PublicKey: publicKey, Scopes: []string{"read", "execute"},
			CreatedAtMs: identityNow(), ExpiresAtMs: record.ExpiresAtMs,
		}
		state.CLI[requestID] = record
		state.Credentials[record.ClientID] = record.Credential
		approved = record.Credential
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, apiCredentialValue(approved))
}

func domainAPIPollCLI(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 3 {
		setStringResult(results, false, "invalid CLI authorization")
		return
	}
	requestID, deviceCode := valueString(argValue(args, 0)), valueString(argValue(args, 1))
	now := uint64(C.rc_u64(argValue(args, 2)))
	option := newValue()
	err := mutateAPI(func(state *apiDomainState) error {
		record, exists := state.CLI[requestID]
		if !exists || record.DeviceCode != deviceCode || record.ExpiresAtMs <= now {
			return domainError("CLI authorization is invalid or expired")
		}
		if !record.Approved {
			C.rc_set_none_option(option)
			return nil
		}
		if record.Issued {
			return domainError("CLI authorization was already issued")
		}
		record.Issued = true
		state.CLI[requestID] = record
		C.rc_set_some_option(option, apiCredentialValue(record.Credential))
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, option)
}
