//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"
)

const (
	sessionDefaultLifetime = 30 * 24 * time.Hour
	challengeLifetime      = 2 * time.Minute
	authorizationLifetime  = 60 * time.Second
)

func sessionDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func sessionToken() string {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(value)
}

func cookieTokens(header string) []string {
	var tokens []string
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && name == "rc_session" && value != "" {
			tokens = append(tokens, value)
		}
	}
	return tokens
}

func sessionActive(state *identityDomain, token string, now uint64) (identitySession, bool) {
	session, ok := state.Sessions[sessionDigest(token)]
	if !ok || session.ExpiresAtMs <= now {
		return identitySession{}, false
	}
	if _, exists := state.Users[session.UserID]; !exists {
		return identitySession{}, false
	}
	return session, true
}

func domainSessionIssue(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid session request")
		return
	}
	userID := valueString(argValue(args, 0))
	expires := uint64(C.rc_u64(argValue(args, 1)))
	now := identityNow()
	if expires == 0 {
		expires = now + uint64(sessionDefaultLifetime/time.Millisecond)
	}
	if userID == "" || expires <= now {
		setStringResult(results, false, "invalid session request")
		return
	}
	token := sessionToken()
	if token == "" {
		setStringResult(results, false, "session issuance failed")
		return
	}
	var issued identitySession
	err := mutateIdentity(func(state *identityDomain) error {
		user, exists := state.Users[userID]
		if !exists {
			return domainError("identity user not found")
		}
		issued = identitySession{
			ID: identityToken("sess_"), UserID: userID, DisplayName: user.DisplayName,
			CreatedAtMs: now, ExpiresAtMs: expires,
		}
		state.Sessions[sessionDigest(token)] = issued
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	record := newOrderedRecord([]fieldValue{
		{name: "token", value: newStringValue(token)},
		{name: "session", value: identitySessionValue(issued)},
	})
	setOK(results, record)
}

func domainSessionFind(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid session cookie")
		return
	}
	state, _, err := loadIdentity()
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	option := newValue()
	now := identityNow()
	for _, token := range cookieTokens(valueString(argValue(args, 0))) {
		if session, ok := sessionActive(state, token, now); ok {
			C.rc_set_some_option(option, identitySessionValue(session))
			setOK(results, option)
			return
		}
	}
	C.rc_set_none_option(option)
	setOK(results, option)
}

func domainSessionRevoke(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid session cookie")
		return
	}
	tokens := cookieTokens(valueString(argValue(args, 0)))
	revoked := false
	err := mutateIdentity(func(state *identityDomain) error {
		for _, token := range tokens {
			key := sessionDigest(token)
			if _, exists := state.Sessions[key]; exists {
				delete(state.Sessions, key)
				revoked = true
			}
		}
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
