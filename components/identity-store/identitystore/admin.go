package identitystore

import (
	"bytes"
	"errors"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_identity_admin_consumer"
	"wit_component/ohrats_rc_identity_admin_issuer"
	"wit_component/ohrats_rc_identity_types"
	"wit_component/ohrats_rc_session_types"
	"wit_component/ohrats_rc_webauthn_types"
	"wit_component/ohrats_rc_webauthn_verifier"
)

const (
	adminCeremonyKind   = "admin-step-up"
	adminAuthBucket     = "human-admin-authorizations"
	adminCeremonyTTLMs  = 5 * 60 * 1000
	adminAuthTTLMs      = 2 * 60 * 1000
	adminTokenBytes     = 32
	adminChallengeBytes = 32
	adminCeremonyIDLen  = 18
)

type ceremonyMeta struct {
	UserId          string `json:"user_id"`
	SessionId       string `json:"session_id"`
	BrowserClientId string `json:"browser_client_id"`
	Operation       string `json:"operation"`
}

type ceremonyState struct {
	Challenge byteArray `json:"challenge"`
	RpId      string    `json:"rp_id"`
	Origin    string    `json:"origin"`
}

type storedAuthorization struct {
	UserId          string `json:"user_id"`
	BrowserClientId string `json:"browser_client_id"`
	Operation       string `json:"operation"`
	IssuedAtMs      uint64 `json:"issued_at_ms"`
	ExpiresAtMs     uint64 `json:"expires_at_ms"`
}

func BeginAdmin(
	sessionCookie string,
	browserClientId string,
	operation string,
	relyingParty ohrats_rc_webauthn_types.RelyingParty,
) (ohrats_rc_identity_admin_issuer.Challenge, error) {
	session, err := requireSession(sessionCookie)
	if err != nil {
		return ohrats_rc_identity_admin_issuer.Challenge{}, err
	}
	if err := validID(browserClientId, "browser client id"); err != nil {
		return ohrats_rc_identity_admin_issuer.Challenge{}, err
	}
	if err := validID(operation, "administrator operation"); err != nil {
		return ohrats_rc_identity_admin_issuer.Challenge{}, err
	}
	if err := boundedText(relyingParty.Id, "relying party id", 255); err != nil {
		return ohrats_rc_identity_admin_issuer.Challenge{}, err
	}
	if err := boundedText(relyingParty.Origin, "relying party origin", 512); err != nil {
		return ohrats_rc_identity_admin_issuer.Challenge{}, err
	}
	passkeys, err := AllPasskeys(witTypes.Some(session.Principal.UserId))
	if err != nil {
		return ohrats_rc_identity_admin_issuer.Challenge{}, err
	}
	if len(passkeys) == 0 {
		return ohrats_rc_identity_admin_issuer.Challenge{}, errors.New("no passkey is registered")
	}
	challenge, err := random(adminChallengeBytes)
	if err != nil {
		return ohrats_rc_identity_admin_issuer.Challenge{}, err
	}
	id, err := randomHexID(adminCeremonyIDLen)
	if err != nil {
		return ohrats_rc_identity_admin_issuer.Challenge{}, err
	}
	now := nowMs()
	metadata, err := encode(ceremonyMeta{
		UserId:          session.Principal.UserId,
		SessionId:       session.Id,
		BrowserClientId: browserClientId,
		Operation:       operation,
	})
	if err != nil {
		return ohrats_rc_identity_admin_issuer.Challenge{}, err
	}
	state, err := encode(ceremonyState{
		Challenge: emptyByteArray(challenge),
		RpId:      relyingParty.Id,
		Origin:    relyingParty.Origin,
	})
	if err != nil {
		return ohrats_rc_identity_admin_issuer.Challenge{}, err
	}
	err = PutCeremony(ohrats_rc_identity_types.Ceremony{
		Id:          id,
		Kind:        adminCeremonyKind,
		UserId:      witTypes.None[string](),
		Metadata:    metadata,
		State:       state,
		ExpiresAtMs: saturatingAdd(now, adminCeremonyTTLMs),
	})
	if err != nil {
		return ohrats_rc_identity_admin_issuer.Challenge{}, err
	}
	credentialIds := make([][]byte, 0, len(passkeys))
	for _, passkey := range passkeys {
		credentialIds = append(credentialIds, passkey.Credential.Id)
	}
	return ohrats_rc_identity_admin_issuer.Challenge{
		Id:            id,
		Challenge:     challenge,
		RelyingParty:  relyingParty,
		CredentialIds: credentialIds,
	}, nil
}

func IssueAdmin(
	sessionCookie string,
	browserClientId string,
	challengeId string,
	authentication ohrats_rc_webauthn_types.AuthenticationRequest,
) ([]byte, error) {
	session, err := requireSession(sessionCookie)
	if err != nil {
		return nil, err
	}
	if err := validID(browserClientId, "browser client id"); err != nil {
		return nil, err
	}
	if err := validID(challengeId, "administrator ceremony id"); err != nil {
		return nil, err
	}
	ceremony, err := TakeCeremony(challengeId, adminCeremonyKind)
	if err != nil {
		return nil, err
	}
	if ceremony.IsNone() {
		return nil, errors.New("administrator ceremony expired")
	}
	value := ceremony.Some()
	meta, err := decode[ceremonyMeta](value.Metadata)
	if err != nil {
		return nil, err
	}
	state, err := decode[ceremonyState](value.State)
	if err != nil {
		return nil, err
	}
	if meta.UserId != session.Principal.UserId ||
		meta.SessionId != session.Id ||
		meta.BrowserClientId != browserClientId ||
		!bytes.Equal(authentication.Challenge, state.Challenge) ||
		authentication.RelyingParty.Id != state.RpId ||
		authentication.RelyingParty.Origin != state.Origin {
		return nil, errors.New("administrator ceremony binding failed")
	}
	found, err := GetPasskeyByCredentialID(authentication.CredentialId)
	if err != nil {
		return nil, err
	}
	if found.IsNone() {
		return nil, errors.New("unknown passkey")
	}
	passkey := found.Some()
	if passkey.UserId != session.Principal.UserId ||
		!bytes.Equal(authentication.ExpectedUserHandle, []byte(session.Principal.UserId)) {
		return nil, errors.New("passkey does not belong to browser user")
	}
	algorithm := passkey.Credential.Algorithm
	authentication.Credential = passkey.Credential
	verified, err := verifyAuthentication(algorithm, authentication)
	if err != nil {
		return nil, err
	}
	if !verified.UserVerified {
		return nil, errors.New("fresh passkey verification required")
	}
	issuedAtMs := nowMs()
	if _, err := UpdatePasskey(passkey.Id, verified.Credential, issuedAtMs); err != nil {
		return nil, err
	}
	token, err := random(adminTokenBytes)
	if err != nil {
		return nil, err
	}
	stored := storedAuthorization{
		UserId:          meta.UserId,
		BrowserClientId: meta.BrowserClientId,
		Operation:       meta.Operation,
		IssuedAtMs:      issuedAtMs,
		ExpiresAtMs:     saturatingAdd(issuedAtMs, adminAuthTTLMs),
	}
	encoded, err := encode(stored)
	if err != nil {
		return nil, err
	}
	if err := insert(adminAuthBucket, digest(token), encoded); err != nil {
		return nil, err
	}
	return token, nil
}

func ConsumeAdmin(token []byte, operation string) (ohrats_rc_identity_admin_consumer.Claim, error) {
	if err := validID(operation, "administrator operation"); err != nil {
		return ohrats_rc_identity_admin_consumer.Claim{}, err
	}
	if len(token) != adminTokenBytes {
		return ohrats_rc_identity_admin_consumer.Claim{}, errors.New("invalid administrator authorization")
	}
	value, found, err := takeValue(adminAuthBucket, digest(token))
	if err != nil {
		return ohrats_rc_identity_admin_consumer.Claim{}, err
	}
	if !found {
		return ohrats_rc_identity_admin_consumer.Claim{}, errors.New("administrator authorization already consumed or withdrawn")
	}
	stored, err := decode[storedAuthorization](value)
	if err != nil {
		return ohrats_rc_identity_admin_consumer.Claim{}, err
	}
	consumedAtMs := nowMs()
	if stored.ExpiresAtMs <= consumedAtMs || stored.Operation != operation {
		return ohrats_rc_identity_admin_consumer.Claim{}, errors.New("administrator authorization rejected")
	}
	return ohrats_rc_identity_admin_consumer.Claim{
		UserId:          stored.UserId,
		BrowserClientId: stored.BrowserClientId,
		IssuedAtMs:      stored.IssuedAtMs,
		ConsumedAtMs:    consumedAtMs,
		ExpiresAtMs:     stored.ExpiresAtMs,
	}, nil
}

// WithdrawAdmin drops every outstanding human authorization when the component
// is deactivated.
func WithdrawAdmin() {
	entries, err := scanAll(adminAuthBucket)
	if err != nil {
		return
	}
	deletions := make([]Delete, 0, len(entries))
	for _, entry := range entries {
		deletions = append(deletions, Delete{Bucket: adminAuthBucket, Key: entry.Key})
	}
	_, _ = removeMany(deletions)
}

func requireSession(cookieHeader string) (ohrats_rc_session_types.Session, error) {
	session, err := FindSession(cookieHeader)
	if err != nil {
		return ohrats_rc_session_types.Session{}, err
	}
	if session.IsNone() {
		return ohrats_rc_session_types.Session{}, errors.New("active browser session required")
	}
	return session.Some(), nil
}

func verifyAuthentication(
	algorithm string,
	value ohrats_rc_webauthn_types.AuthenticationRequest,
) (ohrats_rc_webauthn_types.VerifiedAuthentication, error) {
	result := ohrats_rc_webauthn_verifier.VerifyAuthentication(algorithm, value)
	if result.IsErr() {
		return ohrats_rc_webauthn_types.VerifiedAuthentication{}, errors.New(result.Err())
	}
	return result.Ok(), nil
}

func boundedText(value string, label string, limit int) error {
	if value == "" || len(value) > limit || hasControl(value) {
		return errors.New("invalid " + label)
	}
	return nil
}

func saturatingAdd(value uint64, delta uint64) uint64 {
	sum := value + delta
	if sum < value {
		return ^uint64(0)
	}
	return sum
}
