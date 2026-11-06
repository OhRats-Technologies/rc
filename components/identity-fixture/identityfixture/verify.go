package identityfixture

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_identity_ceremonies"
	"wit_component/ohrats_rc_identity_credentials"
	"wit_component/ohrats_rc_identity_types"
	"wit_component/ohrats_rc_identity_users"
	"wit_component/ohrats_rc_session_lookup"
	"wit_component/ohrats_rc_session_management"
)

func verify(args []string) (uint32, error) {
	if len(args) != 2 {
		return 0, errors.New("usage: rc identity-verify <fixture-id> <session-token>")
	}
	fixture := args[0]
	token := args[1]
	if err := validateFixture(fixture); err != nil {
		return 0, err
	}
	id := userID(fixture)
	cookie := "theme=dark; rc_session=" + token + "; locale=en"
	found := ohrats_rc_session_lookup.Find(cookie)
	if found.IsErr() {
		return 0, errors.New(found.Err())
	}
	session, ok := takeOption(found.Ok())
	if !ok {
		return 0, errors.New("browser session was not restored")
	}
	if session.Principal.UserId != id ||
		session.Principal.DisplayName != "Identity Fixture" ||
		session.CreatedAtMs == 0 ||
		session.ExpiresAtMs <= nowMs() {
		return 0, errors.New("restored browser session is invalid")
	}
	fetched := ohrats_rc_identity_users.Get(id)
	if fetched.IsErr() {
		return 0, errors.New(fetched.Err())
	}
	user, ok := takeOption(fetched.Ok())
	if !ok {
		return 0, errors.New("identity user was not restored")
	}
	if user.DisplayName != "Identity Fixture" {
		return 0, errors.New("restored identity user is invalid")
	}
	counted := ohrats_rc_identity_users.Count()
	if counted.IsErr() {
		return 0, errors.New(counted.Err())
	}
	if counted.Ok() == 0 {
		return 0, errors.New("restored identity user is invalid")
	}
	all := ohrats_rc_identity_users.All()
	if all.IsErr() {
		return 0, errors.New(all.Err())
	}
	listed := false
	for _, value := range all.Ok() {
		if value.Id == id {
			listed = true
			break
		}
	}
	if !listed {
		return 0, errors.New("identity user listing omitted fixture user")
	}

	credentialID := fixtureCredential(fixture, 0).Id
	passkeyResult := ohrats_rc_identity_credentials.GetByCredentialId(credentialID)
	if passkeyResult.IsErr() {
		return 0, errors.New(passkeyResult.Err())
	}
	passkey, ok := takeOption(passkeyResult.Ok())
	if !ok {
		return 0, errors.New("identity passkey was not restored")
	}
	if passkey.UserId != id || passkey.Name != "Fixture Passkey" {
		return 0, errors.New("restored identity passkey is invalid")
	}
	next := passkey.Credential
	next.SignCount = 1
	updatedResult := ohrats_rc_identity_credentials.Update(passkey.Id, next, nowMs())
	if updatedResult.IsErr() {
		return 0, errors.New(updatedResult.Err())
	}
	updated := updatedResult.Ok()
	if updated.Credential.SignCount != 1 || updated.LastUsedAtMs.IsNone() {
		return 0, errors.New("identity passkey update was not persisted")
	}

	expiresAtMs := saturatingAdd(nowMs(), expiryTestMs)
	expiringResult := ohrats_rc_session_management.Issue(id, expiresAtMs)
	if expiringResult.IsErr() {
		return 0, errors.New(expiringResult.Err())
	}
	expiring := expiringResult.Ok()
	expiringCookie := "rc_session=" + expiring.Token
	expiringID := "fixture-expiring-" + fixture
	put := ohrats_rc_identity_ceremonies.Put(ohrats_rc_identity_types.Ceremony{
		Id:          expiringID,
		Kind:        "login",
		UserId:      witTypes.Some(id),
		Metadata:    []byte{},
		State:       []byte("expires"),
		ExpiresAtMs: expiresAtMs,
	})
	if put.IsErr() {
		return 0, errors.New(put.Err())
	}
	time.Sleep(time.Duration(expiryTestMs+50) * time.Millisecond)
	expired := ohrats_rc_session_lookup.Find(expiringCookie)
	if expired.IsErr() {
		return 0, errors.New(expired.Err())
	}
	if expired.Ok().IsSome() {
		return 0, errors.New("expired identity state remained active")
	}
	expiredCeremony := ohrats_rc_identity_ceremonies.Take(expiringID, "login")
	if expiredCeremony.IsErr() {
		return 0, errors.New(expiredCeremony.Err())
	}
	if expiredCeremony.Ok().IsSome() {
		return 0, errors.New("expired identity state remained active")
	}

	registrationID := ceremonyID(fixture)
	wrongKind := ohrats_rc_identity_ceremonies.Take(registrationID, "login")
	if wrongKind.IsErr() {
		return 0, errors.New(wrongKind.Err())
	}
	if wrongKind.Ok().IsSome() {
		return 0, errors.New("wrong ceremony kind matched")
	}
	taken := ohrats_rc_identity_ceremonies.Take(registrationID, "registration")
	if taken.IsErr() {
		return 0, errors.New(taken.Err())
	}
	ceremony, ok := takeOption(taken.Ok())
	if !ok {
		return 0, errors.New("registration ceremony was not restored")
	}
	if ceremony.UserId.IsNone() ||
		ceremony.UserId.Some() != id ||
		!bytes.Equal(ceremony.Metadata, []byte("fixture-metadata")) ||
		!bytes.Equal(ceremony.State, []byte("fixture-state")) {
		return 0, errors.New("restored ceremony is invalid")
	}
	again := ohrats_rc_identity_ceremonies.Take(registrationID, "registration")
	if again.IsErr() {
		return 0, errors.New(again.Err())
	}
	if again.Ok().IsSome() {
		return 0, errors.New("single-use ceremony was returned twice")
	}

	revokeCookie := "rc_session=" + token + "; rc_session=invalid; rc_session=" + token
	revoked := ohrats_rc_session_management.Revoke(revokeCookie)
	if revoked.IsErr() {
		return 0, errors.New(revoked.Err())
	}
	stillFound := ohrats_rc_session_lookup.Find(cookie)
	if stillFound.IsErr() {
		return 0, errors.New(stillFound.Err())
	}
	if !revoked.Ok() || stillFound.Ok().IsSome() {
		return 0, errors.New("browser session revocation failed")
	}
	missing := ohrats_rc_session_management.Revoke(cookie)
	if missing.IsErr() {
		return 0, errors.New(missing.Err())
	}
	invalidFound := ohrats_rc_session_lookup.Find("rc_session=invalid")
	if invalidFound.IsErr() {
		return 0, errors.New(invalidFound.Err())
	}
	if missing.Ok() || invalidFound.Ok().IsSome() {
		return 0, errors.New("missing browser session was treated as active")
	}
	fmt.Println("identity state: ok")
	return 0, nil
}
