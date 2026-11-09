//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"encoding/base64"
	"time"
)

type apiCredentialRecord struct {
	ID           string   `json:"id"`
	UserID       string   `json:"userId"`
	Kind         string   `json:"kind"`
	Name         string   `json:"name"`
	PublicKey    string   `json:"publicKey"`
	Scopes       []string `json:"scopes"`
	CreatedAtMs  uint64   `json:"createdAtMs"`
	ExpiresAtMs  uint64   `json:"expiresAtMs"`
	LastUsedAtMs *uint64  `json:"lastUsedAtMs,omitempty"`
	RevokedAtMs  *uint64  `json:"revokedAtMs,omitempty"`
}

type apiCLIRecord struct {
	RequestID   string              `json:"requestId"`
	ClientID    string              `json:"clientId"`
	PublicKey   string              `json:"publicKey"`
	DeviceCode  string              `json:"deviceCode"`
	UserCode    string              `json:"userCode"`
	ExpiresAtMs uint64              `json:"expiresAtMs"`
	Interval    uint32              `json:"interval"`
	Approved    bool                `json:"approved"`
	Issued      bool                `json:"issued"`
	Credential  apiCredentialRecord `json:"credential"`
}

type apiDomainState struct {
	Credentials map[string]apiCredentialRecord `json:"credentials"`
	Nonces      map[string]uint64              `json:"nonces"`
	CLI         map[string]apiCLIRecord        `json:"cli"`
}

func (s *apiDomainState) ensure() {
	if s.Credentials == nil {
		s.Credentials = make(map[string]apiCredentialRecord)
	}
	if s.Nonces == nil {
		s.Nonces = make(map[string]uint64)
	}
	if s.CLI == nil {
		s.CLI = make(map[string]apiCLIRecord)
	}
}

func mutateAPI(fn func(*apiDomainState) error) error {
	_, err := domainMutate(domainStore(), "api", func(state *apiDomainState) error {
		state.ensure()
		return fn(state)
	})
	return err
}

func apiCredentialValue(value apiCredentialRecord) *C.wasmtime_component_val_t {
	scopes := make([]*C.wasmtime_component_val_t, len(value.Scopes))
	for index, scope := range value.Scopes {
		scopes[index] = newEnumValue(scope)
	}
	return newOrderedRecord([]fieldValue{
		{name: "id", value: newStringValue(value.ID)},
		{name: "user-id", value: newStringValue(value.UserID)},
		{name: "kind", value: newEnumValue(value.Kind)},
		{name: "name", value: newStringValue(value.Name)},
		{name: "public-key", value: newStringValue(value.PublicKey)},
		{name: "scopes", value: newListValue(scopes)},
		{name: "created-at-ms", value: newU64Value(value.CreatedAtMs)},
		{name: "expires-at-ms", value: newU64Value(value.ExpiresAtMs)},
		{name: "last-used-at-ms", value: optionU64(value.LastUsedAtMs)},
		{name: "revoked-at-ms", value: optionU64(value.RevokedAtMs)},
	})
}

func apiLifetime(value string) (time.Duration, bool) {
	switch value {
	case "never":
		return 0, true
	case "one-hour":
		return time.Hour, true
	case "one-day":
		return 24 * time.Hour, true
	case "seven-days":
		return 7 * 24 * time.Hour, true
	case "thirty-days":
		return 30 * 24 * time.Hour, true
	case "ninety-days":
		return 90 * 24 * time.Hour, true
	case "one-eighty-days":
		return 180 * 24 * time.Hour, true
	case "one-year":
		return 365 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

func apiLifetimeFromOption(value *C.wasmtime_component_val_t) (uint64, bool) {
	item := C.rc_option_get(value)
	if item == nil {
		return 0, true
	}
	duration, ok := apiLifetime(valueEnum(item))
	if !ok {
		return 0, false
	}
	if duration == 0 {
		return 0, true
	}
	return identityNow() + uint64(duration/time.Millisecond), true
}

func apiScopes(value *C.wasmtime_component_val_t) ([]string, bool) {
	if value == nil {
		return nil, false
	}
	scopes := make([]string, 0, int(C.rc_list_len(value)))
	seen := make(map[string]bool)
	for index := 0; index < int(C.rc_list_len(value)); index++ {
		scope := valueEnum(C.rc_list_get(value, C.size_t(index)))
		switch scope {
		case "read", "execute", "manage-devices", "manage-workspaces":
		default:
			return nil, false
		}
		if seen[scope] {
			return nil, false
		}
		seen[scope] = true
		scopes = append(scopes, scope)
	}
	return scopes, len(scopes) > 0
}

func validAPIKey(value string) bool {
	if len(value) < 40 || len(value) > 128 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
}
