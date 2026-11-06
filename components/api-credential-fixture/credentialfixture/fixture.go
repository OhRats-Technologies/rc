package credentialfixture

import (
	"errors"
	"fmt"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_api_credentials_credentials"
	"wit_component/ohrats_rc_api_credentials_types"
)

// Invoke dispatches the fixture commands. Every command takes exactly one
// fixture name.
func Invoke(command string, args []string) witTypes.Result[uint32, string] {
	if len(args) != 1 {
		return witTypes.Err[uint32, string]("usage: rc api-credentials-<seed|verify|race> <fixture>")
	}
	fixture := args[0]
	if err := valid(fixture); err != nil {
		return witTypes.Err[uint32, string](err.Error())
	}
	switch command {
	case "api-credentials-seed":
		return commandResult(seed(fixture))
	case "api-credentials-verify":
		return commandResult(verify(fixture))
	case "api-credentials-race":
		return commandResult(race(fixture))
	default:
		return witTypes.Err[uint32, string](fmt.Sprintf("unsupported command %q", command))
	}
}

func commandResult(err error) witTypes.Result[uint32, string] {
	if err != nil {
		return witTypes.Err[uint32, string](err.Error())
	}
	return witTypes.Ok[uint32, string](0)
}

func seed(fixture string) error {
	if err := createUser(fixture); err != nil {
		return err
	}
	apiId := "api-" + fixture
	apiKey := key(7)
	readOnly := []ohrats_rc_api_credentials_types.Scope{ohrats_rc_api_credentials_types.Scope_Read}
	noLifetime := witTypes.None[ohrats_rc_api_credentials_types.Lifetime]()
	if result := ohrats_rc_api_credentials_credentials.CreateApi(
		forgedToken(),
		"forged-"+fixture,
		"Forged token",
		apiKey,
		readOnly,
		noLifetime,
	); !result.IsErr() {
		return errors.New("forged admin token was accepted")
	}
	wrongOperation, err := authorization(browserClient, fixture, "api-credential.revoke")
	if err != nil {
		return err
	}
	if result := ohrats_rc_api_credentials_credentials.CreateApi(
		wrongOperation,
		"wrong-"+fixture,
		"Wrong operation",
		apiKey,
		readOnly,
		noLifetime,
	); !result.IsErr() {
		return errors.New("operation-mismatched admin proof was accepted")
	}
	create, err := authorization(browserClient, fixture, "api-credential.create")
	if err != nil {
		return err
	}
	created := ohrats_rc_api_credentials_credentials.CreateApi(
		create,
		apiId,
		"Fixture API",
		apiKey,
		[]ohrats_rc_api_credentials_types.Scope{
			ohrats_rc_api_credentials_types.Scope_Read,
			ohrats_rc_api_credentials_types.Scope_Execute,
		},
		noLifetime,
	)
	if created.IsErr() {
		return errors.New(created.Err())
	}
	if result := ohrats_rc_api_credentials_credentials.CreateApi(
		create,
		"replay-"+fixture,
		"Replayed proof",
		apiKey,
		readOnly,
		noLifetime,
	); !result.IsErr() {
		return errors.New("administrator proof was reusable")
	}
	expiryId := "exp-" + fixture
	createExpiry, err := authorization(browserClient, fixture, "api-credential.create")
	if err != nil {
		return err
	}
	expiring := ohrats_rc_api_credentials_credentials.CreateApi(
		createExpiry,
		expiryId,
		"Expires",
		apiKey,
		readOnly,
		witTypes.Some(ohrats_rc_api_credentials_types.Lifetime_OneHour),
	)
	if expiring.IsErr() {
		return errors.New(expiring.Err())
	}
	cliId := "cli-" + fixture
	cliKey := key(9)
	requestId := "req-" + fixture
	deviceCode := "device-" + fixture
	userCode := "user-" + fixture
	startedAt := nowMs()
	started := ohrats_rc_api_credentials_credentials.StartCli(
		cliId,
		cliKey,
		witTypes.Some(ohrats_rc_api_credentials_types.Lifetime_OneDay),
		requestId,
		deviceCode,
		userCode,
		startedAt,
	)
	if started.IsErr() {
		return errors.New(started.Err())
	}
	start := started.Ok()
	polled := ohrats_rc_api_credentials_credentials.PollCli(start.RequestId, deviceCode, startedAt)
	if polled.IsErr() {
		return errors.New(polled.Err())
	}
	if polled.Ok().IsSome() {
		return errors.New("unapproved CLI authorization was issued")
	}
	if _, err := forgedAuthorization(browserClient, fixture); err == nil {
		return errors.New("forged admin proof was issued")
	}
	wrongClient, err := authorization(browserClient, fixture, "api-credential.cli-approve")
	if err != nil {
		return err
	}
	if result := ohrats_rc_api_credentials_credentials.ApproveCli(wrongClient, start.RequestId, userCode, cliKey); !result.IsErr() {
		return errors.New("client-mismatched admin proof was accepted")
	}
	approve, err := authorization(cliId, fixture, "api-credential.cli-approve")
	if err != nil {
		return err
	}
	approved := ohrats_rc_api_credentials_credentials.ApproveCli(approve, start.RequestId, userCode, cliKey)
	if approved.IsErr() {
		return errors.New(approved.Err())
	}
	fmt.Println("api credential seed: ok")
	return nil
}

func verify(fixture string) error {
	api := "api-" + fixture
	expiring := "exp-" + fixture
	cli := "cli-" + fixture
	listed := ohrats_rc_api_credentials_credentials.All(userID)
	if listed.IsErr() {
		return errors.New(listed.Err())
	}
	values := listed.Ok()
	if len(values) != 3 || countKind(values, ohrats_rc_api_credentials_types.Kind_Api) != 2 {
		return errors.New("credential persistence failed")
	}
	validAt := nowMs()
	validRequest := signed(7, api, validAt, "nonce-fixture-0001", "GET", "/api/v1/me", []byte{})
	verified := ohrats_rc_api_credentials_credentials.Verify(validRequest, validAt)
	if verified.IsErr() {
		return errors.New(verified.Err())
	}
	claim := verified.Ok()
	if claim.CredentialId != api || !containsScope(claim.Scopes, ohrats_rc_api_credentials_types.Scope_Execute) {
		return errors.New("API proof or scopes failed")
	}
	if replayed := ohrats_rc_api_credentials_credentials.Verify(validRequest, validAt); !replayed.IsErr() {
		return errors.New("successful nonce was replayable")
	}
	changedAt := nowMs()
	changedRequest := signed(7, api, changedAt, "nonce-fixture-0002", "POST", "/api/v1/me", []byte("original"))
	changedRequest.Body = []byte("changed")
	if accepted := ohrats_rc_api_credentials_credentials.Verify(changedRequest, changedAt); !accepted.IsErr() {
		return errors.New("body-bound signature was accepted")
	}
	polledAt := nowMs()
	deviceCode := "device-" + fixture
	requestId := "req-" + fixture
	polled := ohrats_rc_api_credentials_credentials.PollCli(requestId, deviceCode, polledAt)
	if polled.IsErr() {
		return errors.New(polled.Err())
	}
	if polled.Ok().IsNone() {
		return errors.New("approved CLI was not returned")
	}
	cliValue := polled.Ok().Some()
	if cliValue.Id != cli {
		return errors.New("CLI authorization was reusable")
	}
	if reused := ohrats_rc_api_credentials_credentials.PollCli(requestId, deviceCode, polledAt); !reused.IsErr() {
		return errors.New("CLI authorization was reusable")
	}
	cliAt := nowMs()
	cliProof := signed(9, cli, cliAt, "nonce-cli-000001", "GET", "/api/v1/devices", []byte{})
	if verifiedCLI := ohrats_rc_api_credentials_credentials.Verify(cliProof, cliAt); verifiedCLI.IsErr() {
		return errors.New(verifiedCLI.Err())
	}
	restored := ohrats_rc_api_credentials_credentials.Get(expiring)
	if restored.IsErr() {
		return errors.New(restored.Err())
	}
	if restored.Ok().IsNone() {
		return errors.New("expiring credential was not restored")
	}
	expiredAt := restored.Ok().Some().ExpiresAtMs + 1000
	expiredRequest := signed(7, expiring, expiredAt, "nonce-expiry-0001", "GET", "/", []byte{})
	if accepted := ohrats_rc_api_credentials_credentials.Verify(expiredRequest, expiredAt); !accepted.IsErr() {
		return errors.New("expired credential was accepted")
	}
	revokedAt := nowMs()
	revokedRequest := signed(7, api, revokedAt, "nonce-revoked-01", "GET", "/", []byte{})
	revoke, err := authorization(browserClient, fixture, "api-credential.revoke")
	if err != nil {
		return err
	}
	revoked := ohrats_rc_api_credentials_credentials.Revoke(revoke, api)
	if revoked.IsErr() {
		return errors.New(revoked.Err())
	}
	if !revoked.Ok() {
		return errors.New("revoked credential remained valid")
	}
	if accepted := ohrats_rc_api_credentials_credentials.Verify(revokedRequest, revokedAt); !accepted.IsErr() {
		return errors.New("revoked credential remained valid")
	}
	fmt.Println("api credential state: ok")
	return nil
}

func race(fixture string) error {
	id := "api-" + fixture
	now := nowMs()
	request := signed(7, id, now, "nonce-race-00001", "GET", "/", []byte{})
	verified := ohrats_rc_api_credentials_credentials.Verify(request, now)
	if verified.IsErr() {
		return errors.New(verified.Err())
	}
	return nil
}
