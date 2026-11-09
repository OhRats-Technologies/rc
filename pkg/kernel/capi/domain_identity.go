//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"time"
	"unicode/utf8"
)

func identityNow() uint64 {
	value := time.Now().UnixMilli()
	if value < 0 {
		return 0
	}
	return uint64(value)
}

func identityToken(prefix string) string {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return ""
	}
	return prefix + base64.RawURLEncoding.EncodeToString(value)
}

func identityName(value string) (string, bool) {
	value = strings.TrimSpace(value)
	return value, value != "" && len(value) <= 120 && utf8.ValidString(value)
}

func (s *identityDomain) ensure() {
	if s.Users == nil {
		s.Users = make(map[string]identityUser)
	}
	if s.Passkeys == nil {
		s.Passkeys = make(map[string]identityPasskey)
	}
	if s.CredentialIDs == nil {
		s.CredentialIDs = make(map[string]string)
	}
	if s.Ceremonies == nil {
		s.Ceremonies = make(map[string]identityCeremony)
	}
	if s.Sessions == nil {
		s.Sessions = make(map[string]identitySession)
	}
	if s.Challenges == nil {
		s.Challenges = make(map[string]identityChallenge)
	}
	if s.Authorizations == nil {
		s.Authorizations = make(map[string]identityAuthorization)
	}
}

func loadIdentity() (*identityDomain, uint64, error) {
	state, revision, err := domainLoad[identityDomain](domainStore(), "identity")
	if err != nil {
		return nil, 0, err
	}
	state.ensure()
	return &state, revision, nil
}

func saveIdentity(state *identityDomain, revision uint64) error {
	return domainStore().save("identity", state, revision)
}

func mutateIdentity(fn func(*identityDomain) error) error {
	_, err := domainMutate(domainStore(), "identity", func(state *identityDomain) error {
		state.ensure()
		return fn(state)
	})
	return err
}

func validIdentityCredential(value identityCredential) bool {
	if len(value.ID) == 0 || len(value.ID) > 1024 || len(value.PublicKeyCOSE) == 0 ||
		len(value.PublicKeyCOSE) > 16*1024 {
		return false
	}
	return value.Algorithm == "es256" || value.Algorithm == "rs256"
}

type domainError string

func (e domainError) Error() string { return string(e) }
