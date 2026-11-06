package identityfixture

import (
	"errors"
	"fmt"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_identity_ceremonies"
	"wit_component/ohrats_rc_identity_credentials"
	"wit_component/ohrats_rc_identity_types"
	"wit_component/ohrats_rc_session_management"
)

func seed(args []string) (uint32, error) {
	if len(args) != 1 {
		return 0, errors.New("usage: rc identity-seed <fixture-id>")
	}
	fixture := args[0]
	if err := validateFixture(fixture); err != nil {
		return 0, err
	}
	id := userID(fixture)
	created := ohrats_rc_identity_credentials.CreateUser(
		id,
		"Identity Fixture",
		"Fixture Passkey",
		fixtureCredential(fixture, 0),
	)
	if created.IsErr() {
		return 0, errors.New(created.Err())
	}
	user := created.Ok()
	if user.Id != id || user.DisplayName != "Identity Fixture" || user.CreatedAtMs == 0 {
		return 0, errors.New("identity user creation returned invalid state")
	}
	duplicate := ohrats_rc_identity_credentials.CreateUser(
		id,
		"Duplicate",
		"Duplicate",
		fixtureCredential(fixture, 0),
	)
	if !duplicate.IsErr() {
		return 0, errors.New("duplicate identity user or credential was accepted")
	}
	put := ohrats_rc_identity_ceremonies.Put(ohrats_rc_identity_types.Ceremony{
		Id:          ceremonyID(fixture),
		Kind:        "registration",
		UserId:      witTypes.Some(id),
		Metadata:    []byte("fixture-metadata"),
		State:       []byte("fixture-state"),
		ExpiresAtMs: saturatingAdd(nowMs(), lifetimeMs),
	})
	if put.IsErr() {
		return 0, errors.New(put.Err())
	}
	issued := ohrats_rc_session_management.Issue(id, saturatingAdd(nowMs(), lifetimeMs))
	if issued.IsErr() {
		return 0, errors.New(issued.Err())
	}
	session := issued.Ok()
	if session.Session.Principal.UserId != id || session.Token == "" {
		return 0, errors.New("browser session issuance returned invalid state")
	}
	fmt.Println(session.Token)
	return 0, nil
}
