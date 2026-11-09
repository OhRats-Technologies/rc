//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import "encoding/base64"

type identityCredential struct {
	ID             []byte `json:"id"`
	Algorithm      string `json:"algorithm"`
	PublicKeyCOSE  []byte `json:"publicKeyCose"`
	SignCount      uint32 `json:"signCount"`
	BackupEligible bool   `json:"backupEligible"`
	BackupState    bool   `json:"backupState"`
}

type identityUser struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	CreatedAtMs uint64 `json:"createdAtMs"`
}

type identityPasskey struct {
	ID           string             `json:"id"`
	UserID       string             `json:"userId"`
	Name         string             `json:"name"`
	Credential   identityCredential `json:"credential"`
	CreatedAtMs  uint64             `json:"createdAtMs"`
	LastUsedAtMs *uint64            `json:"lastUsedAtMs,omitempty"`
}

type identityCeremony struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	UserID      *string `json:"userId,omitempty"`
	Metadata    []byte  `json:"metadata"`
	State       []byte  `json:"state"`
	ExpiresAtMs uint64  `json:"expiresAtMs"`
}

type identitySession struct {
	ID          string `json:"id"`
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	CreatedAtMs uint64 `json:"createdAtMs"`
	ExpiresAtMs uint64 `json:"expiresAtMs"`
}

type identityChallenge struct {
	ID              string   `json:"id"`
	UserID          string   `json:"userId"`
	BrowserClientID string   `json:"browserClientId"`
	Operation       string   `json:"operation"`
	Challenge       []byte   `json:"challenge"`
	RPID            string   `json:"rpId"`
	Origin          string   `json:"origin"`
	CredentialIDs   [][]byte `json:"credentialIds"`
	ExpiresAtMs     uint64   `json:"expiresAtMs"`
}

type identityAuthorization struct {
	UserID          string `json:"userId"`
	BrowserClientID string `json:"browserClientId"`
	Operation       string `json:"operation"`
	ExpiresAtMs     uint64 `json:"expiresAtMs"`
}

type identityDomain struct {
	Users          map[string]identityUser          `json:"users"`
	Passkeys       map[string]identityPasskey       `json:"passkeys"`
	CredentialIDs  map[string]string                `json:"credentialIds"`
	Ceremonies     map[string]identityCeremony      `json:"ceremonies"`
	Sessions       map[string]identitySession       `json:"sessions"`
	Challenges     map[string]identityChallenge     `json:"challenges"`
	Authorizations map[string]identityAuthorization `json:"authorizations"`
}

func parseIdentityCredential(value *C.wasmtime_component_val_t) identityCredential {
	return identityCredential{
		ID:             fieldBytes(value, "id"),
		Algorithm:      fieldString(value, "algorithm"),
		PublicKeyCOSE:  fieldBytes(value, "public-key-cose"),
		SignCount:      fieldU32(value, "sign-count"),
		BackupEligible: fieldBool(value, "backup-eligible"),
		BackupState:    fieldBool(value, "backup-state"),
	}
}

func identityCredentialValue(value identityCredential) *C.wasmtime_component_val_t {
	return newOrderedRecord([]fieldValue{
		{name: "id", value: newBytesValue(value.ID)},
		{name: "algorithm", value: newStringValue(value.Algorithm)},
		{name: "public-key-cose", value: newBytesValue(value.PublicKeyCOSE)},
		{name: "sign-count", value: newU32Value(value.SignCount)},
		{name: "backup-eligible", value: newBoolValue(value.BackupEligible)},
		{name: "backup-state", value: newBoolValue(value.BackupState)},
	})
}

func identityUserValue(value identityUser) *C.wasmtime_component_val_t {
	return newOrderedRecord([]fieldValue{
		{name: "id", value: newStringValue(value.ID)},
		{name: "display-name", value: newStringValue(value.DisplayName)},
		{name: "created-at-ms", value: newU64Value(value.CreatedAtMs)},
	})
}

func identityPasskeyValue(value identityPasskey) *C.wasmtime_component_val_t {
	return newOrderedRecord([]fieldValue{
		{name: "id", value: newStringValue(value.ID)},
		{name: "user-id", value: newStringValue(value.UserID)},
		{name: "name", value: newStringValue(value.Name)},
		{name: "credential", value: identityCredentialValue(value.Credential)},
		{name: "created-at-ms", value: newU64Value(value.CreatedAtMs)},
		{name: "last-used-at-ms", value: optionU64(value.LastUsedAtMs)},
	})
}

func identitySessionValue(value identitySession) *C.wasmtime_component_val_t {
	principal := newOrderedRecord([]fieldValue{
		{name: "user-id", value: newStringValue(value.UserID)},
		{name: "display-name", value: newStringValue(value.DisplayName)},
	})
	return newOrderedRecord([]fieldValue{
		{name: "id", value: newStringValue(value.ID)},
		{name: "principal", value: principal},
		{name: "created-at-ms", value: newU64Value(value.CreatedAtMs)},
		{name: "expires-at-ms", value: newU64Value(value.ExpiresAtMs)},
	})
}

func identityCeremonyValue(value identityCeremony) *C.wasmtime_component_val_t {
	return newOrderedRecord([]fieldValue{
		{name: "id", value: newStringValue(value.ID)},
		{name: "kind", value: newStringValue(value.Kind)},
		{name: "user-id", value: optionStringPtr(value.UserID)},
		{name: "metadata", value: newBytesValue(value.Metadata)},
		{name: "state", value: newBytesValue(value.State)},
		{name: "expires-at-ms", value: newU64Value(value.ExpiresAtMs)},
	})
}

func identityChallengeValue(value identityChallenge) *C.wasmtime_component_val_t {
	credentials := make([]*C.wasmtime_component_val_t, len(value.CredentialIDs))
	for index, id := range value.CredentialIDs {
		credentials[index] = newBytesValue(id)
	}
	return newOrderedRecord([]fieldValue{
		{name: "id", value: newStringValue(value.ID)},
		{name: "challenge", value: newBytesValue(value.Challenge)},
		{name: "relying-party", value: newOrderedRecord([]fieldValue{
			{name: "id", value: newStringValue(value.RPID)},
			{name: "origin", value: newStringValue(value.Origin)},
		})},
		{name: "credential-ids", value: newListValue(credentials)},
	})
}

func identityAuthorizationValue(value []byte) *C.wasmtime_component_val_t {
	return newOrderedRecord([]fieldValue{{name: "token", value: newBytesValue(value)}})
}

func optionU64(value *uint64) *C.wasmtime_component_val_t {
	option := newValue()
	if value == nil {
		C.rc_set_none_option(option)
		return option
	}
	C.rc_set_some_option(option, newU64Value(*value))
	return option
}

func optionStringPtr(value *string) *C.wasmtime_component_val_t {
	option := newValue()
	if value == nil {
		C.rc_set_none_option(option)
		return option
	}
	C.rc_set_some_option(option, newStringValue(*value))
	return option
}

func credentialKey(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}
