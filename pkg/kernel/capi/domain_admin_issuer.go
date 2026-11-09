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
	"encoding/json"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
)

func domainIdentityAdminBegin(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 4 {
		setStringResult(results, false, "invalid administrative challenge")
		return
	}
	state, _, err := loadIdentity()
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	now := identityNow()
	session, ok := sessionForCookie(state, valueString(argValue(args, 0)), now)
	if !ok {
		setStringResult(results, false, "browser session required")
		return
	}
	clientID := valueString(argValue(args, 1))
	operation := valueString(argValue(args, 2))
	rp := argValue(args, 3)
	rpID, origin := fieldString(rp, "id"), fieldString(rp, "origin")
	if clientID == "" || operation == "" || rpID == "" || origin == "" {
		setStringResult(results, false, "invalid administrative challenge")
		return
	}
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		setStringResult(results, false, "administrative challenge failed")
		return
	}
	value := identityChallenge{
		ID: identityToken("chal_"), UserID: session.UserID, BrowserClientID: clientID,
		Operation: operation, Challenge: challenge, RPID: rpID, Origin: origin,
		ExpiresAtMs: now + uint64(challengeLifetime/time.Millisecond),
	}
	err = mutateIdentity(func(state *identityDomain) error {
		for _, passkey := range state.Passkeys {
			if passkey.UserID == session.UserID {
				value.CredentialIDs = append(value.CredentialIDs, passkey.Credential.ID)
			}
		}
		state.Challenges[value.ID] = value
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, identityChallengeValue(value))
}

func sessionForCookie(state *identityDomain, header string, now uint64) (identitySession, bool) {
	for _, token := range cookieTokens(header) {
		if session, ok := sessionActive(state, token, now); ok {
			return session, true
		}
	}
	return identitySession{}, false
}

type authenticationResponse struct {
	ID    string                     `json:"id"`
	RawID string                     `json:"rawId"`
	Type  string                     `json:"type"`
	Inner authenticationResponseData `json:"response"`
}

type authenticationResponseData struct {
	ClientDataJSON    string  `json:"clientDataJSON"`
	AuthenticatorData string  `json:"authenticatorData"`
	Signature         string  `json:"signature"`
	UserHandle        *string `json:"userHandle,omitempty"`
}

func domainIdentityAdminIssue(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 4 {
		setStringResult(results, false, "invalid administrative proof")
		return
	}
	state, _, err := loadIdentity()
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	now := identityNow()
	session, ok := sessionForCookie(state, valueString(argValue(args, 0)), now)
	if !ok {
		setStringResult(results, false, "browser session required")
		return
	}
	clientID := valueString(argValue(args, 1))
	challenge, exists := state.Challenges[valueString(argValue(args, 2))]
	if !exists || challenge.UserID != session.UserID || challenge.BrowserClientID != clientID ||
		challenge.ExpiresAtMs <= now {
		setStringResult(results, false, "administrative challenge is invalid or expired")
		return
	}
	request := argValue(args, 3)
	credentialID := fieldBytes(request, "credential-id")
	var passkey identityPasskey
	found := false
	for _, value := range state.Passkeys {
		if value.UserID == session.UserID && credentialKey(value.Credential.ID) == credentialKey(credentialID) {
			passkey, found = value, true
			break
		}
	}
	if !found {
		setStringResult(results, false, "administrative credential is unavailable")
		return
	}
	responseJSON, err := verificationJSON(request)
	if err != nil {
		setStringResult(results, false, "administrative proof is invalid")
		return
	}
	userHandle := fieldBytes(request, "expected-user-handle")
	if len(userHandle) == 0 {
		userHandle = []byte(session.UserID)
	}
	rp := webauthn.RelyingParty{ID: challenge.RPID, Origin: challenge.Origin}
	if err := webauthn.VerifyAuthentication(responseJSON, challenge.Challenge, rp,
		passkey.Credential.ID, passkey.Credential.PublicKeyCOSE, userHandle); err != nil {
		_ = mutateIdentity(func(state *identityDomain) error {
			delete(state.Challenges, challenge.ID)
			return nil
		})
		setStringResult(results, false, "administrative proof is invalid")
		return
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		setStringResult(results, false, "administrative proof failed")
		return
	}
	err = mutateIdentity(func(state *identityDomain) error {
		if _, exists := state.Challenges[challenge.ID]; !exists {
			return domainError("administrative challenge is invalid or expired")
		}
		delete(state.Challenges, challenge.ID)
		state.Authorizations[hex.EncodeToString(tokenDigest(token))] = identityAuthorization{
			UserID: challenge.UserID, BrowserClientID: challenge.BrowserClientID,
			Operation:   challenge.Operation,
			ExpiresAtMs: now + uint64(authorizationLifetime/time.Millisecond),
		}
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, identityAuthorizationValue(token))
}

func verificationJSON(request *C.wasmtime_component_val_t) ([]byte, error) {
	encode := base64.RawURLEncoding.EncodeToString
	body := authenticationResponse{ID: encode(fieldBytes(request, "credential-id")),
		RawID: encode(fieldBytes(request, "credential-id")), Type: "public-key"}
	body.Inner.ClientDataJSON = encode(fieldBytes(request, "client-data-json"))
	body.Inner.AuthenticatorData = encode(fieldBytes(request, "authenticator-data"))
	body.Inner.Signature = encode(fieldBytes(request, "signature"))
	if handle, ok := optionBytes(field(request, "response-user-handle")); ok {
		encoded := encode(handle)
		body.Inner.UserHandle = &encoded
	}
	return json.Marshal(body)
}

func optionBytes(value *C.wasmtime_component_val_t) ([]byte, bool) {
	if value == nil || C.rc_option_get(value) == nil {
		return nil, false
	}
	return valueBytes(C.rc_option_get(value)), true
}

func tokenDigest(token []byte) []byte {
	digest := sha256.Sum256(token)
	return digest[:]
}

// consumeAuthorization validates and consumes a one-use administrative proof
// bound to the requested operation.
func consumeAuthorization(value *C.wasmtime_component_val_t, operation string) (identityAuthorization, error) {
	token := fieldBytes(value, "token")
	if len(token) == 0 {
		return identityAuthorization{}, domainError("administrative proof is invalid")
	}
	var claim identityAuthorization
	key := hex.EncodeToString(tokenDigest(token))
	err := mutateIdentity(func(state *identityDomain) error {
		record, exists := state.Authorizations[key]
		if !exists || record.ExpiresAtMs <= identityNow() || record.Operation != operation {
			return domainError("administrative proof is invalid")
		}
		delete(state.Authorizations, key)
		claim = record
		return nil
	})
	return claim, err
}
