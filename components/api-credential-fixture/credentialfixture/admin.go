package credentialfixture

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_identity_admin_issuer"
	"wit_component/ohrats_rc_identity_credentials"
	"wit_component/ohrats_rc_identity_types"
	"wit_component/ohrats_rc_session_management"
	"wit_component/ohrats_rc_webauthn_types"
)

const (
	userID        = "fixture-user"
	browserClient = "browser-fixture"
	rpID          = "rc.example.test"
	origin        = "https://rc.example.test"
)

var fixturePrivateKey = [32]byte{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3}

var publicX = [32]byte{
	0x59, 0x1a, 0xb7, 0x71, 0xeb, 0xbc, 0xfd, 0x6d, 0x9c, 0xb9, 0x09, 0x4d, 0x10, 0x65, 0x28, 0xad,
	0xd1, 0xa6, 0x9d, 0x44, 0xc2, 0xc1, 0xf6, 0x27, 0xf0, 0x89, 0xec, 0x58, 0xb9, 0xc6, 0x1a, 0xdf,
}

var publicY = [32]byte{
	0x9f, 0x4e, 0x6a, 0xbf, 0x0d, 0x04, 0x5c, 0x0c, 0x69, 0x3a, 0x3c, 0x68, 0xad, 0x7c, 0x97, 0xca,
	0x72, 0xbe, 0x64, 0xde, 0xf4, 0xa2, 0x6f, 0xec, 0xd2, 0x63, 0xdd, 0x98, 0xa9, 0x27, 0x80, 0xf0,
}

func createUser(fixture string) error {
	result := ohrats_rc_identity_credentials.CreateUser(
		userID,
		"Fixture User",
		"Fixture Passkey",
		fixtureCredential(fixture, 0),
	)
	if result.IsErr() {
		return errors.New(result.Err())
	}
	return nil
}

// authorization runs one real session + passkey step-up ceremony for the
// requested operation and returns the single-use human authorization proof.
func authorization(browserClientId string, fixture string, operation string) (ohrats_rc_identity_types.HumanAuthorization, error) {
	empty := ohrats_rc_identity_types.HumanAuthorization{}
	issued := ohrats_rc_session_management.Issue(userID, 0)
	if issued.IsErr() {
		return empty, errors.New(issued.Err())
	}
	cookie := "rc_session=" + issued.Ok().Token
	challenge := ohrats_rc_identity_admin_issuer.Begin(cookie, browserClientId, operation, relyingParty())
	if challenge.IsErr() {
		return empty, errors.New(challenge.Err())
	}
	request, err := authentication(challenge.Ok(), fixture)
	if err != nil {
		return empty, err
	}
	proof := ohrats_rc_identity_admin_issuer.Issue(cookie, browserClientId, challenge.Ok().Id, request)
	if proof.IsErr() {
		return empty, errors.New(proof.Err())
	}
	return proof.Ok(), nil
}

// forgedAuthorization completes the ceremony with a corrupted signature and
// must never yield a proof.
func forgedAuthorization(browserClientId string, fixture string) (ohrats_rc_identity_types.HumanAuthorization, error) {
	empty := ohrats_rc_identity_types.HumanAuthorization{}
	issued := ohrats_rc_session_management.Issue(userID, 0)
	if issued.IsErr() {
		return empty, errors.New(issued.Err())
	}
	cookie := "rc_session=" + issued.Ok().Token
	challenge := ohrats_rc_identity_admin_issuer.Begin(cookie, browserClientId, "api-credential.create", relyingParty())
	if challenge.IsErr() {
		return empty, errors.New(challenge.Err())
	}
	request, err := authentication(challenge.Ok(), fixture)
	if err != nil {
		return empty, err
	}
	request.Signature[0] ^= 1
	proof := ohrats_rc_identity_admin_issuer.Issue(cookie, browserClientId, challenge.Ok().Id, request)
	if proof.IsErr() {
		return empty, errors.New(proof.Err())
	}
	return proof.Ok(), nil
}

func forgedToken() ohrats_rc_identity_types.HumanAuthorization {
	return ohrats_rc_identity_types.HumanAuthorization{Token: make([]uint8, 32)}
}

func authentication(challenge ohrats_rc_identity_admin_issuer.Challenge, fixture string) (ohrats_rc_webauthn_types.AuthenticationRequest, error) {
	empty := ohrats_rc_webauthn_types.AuthenticationRequest{}
	credentialId := []byte("credential-" + fixture)
	result := ohrats_rc_identity_credentials.GetByCredentialId(credentialId)
	if result.IsErr() {
		return empty, errors.New(result.Err())
	}
	option := result.Ok()
	if option.IsNone() {
		return empty, errors.New("fixture passkey is missing")
	}
	credential := option.Some().Credential
	if credential.SignCount == math.MaxUint32 {
		return empty, errors.New("fixture passkey sign count exhausted")
	}
	signCount := credential.SignCount + 1
	challengeBase64 := base64.RawURLEncoding.EncodeToString(challenge.Challenge)
	clientData := []byte(fmt.Sprintf(
		`{"type":"webauthn.get","challenge":"%s","origin":"%s","crossOrigin":false}`,
		challengeBase64,
		origin,
	))
	rpHash := sha256.Sum256([]byte(rpID))
	authenticatorData := make([]uint8, 0, len(rpHash)+5)
	authenticatorData = append(authenticatorData, rpHash[:]...)
	authenticatorData = append(authenticatorData, 0x0d)
	var counter [4]byte
	binary.BigEndian.PutUint32(counter[:], signCount)
	authenticatorData = append(authenticatorData, counter[:]...)
	clientDataHash := sha256.Sum256(clientData)
	signed := make([]uint8, 0, len(authenticatorData)+len(clientDataHash))
	signed = append(signed, authenticatorData...)
	signed = append(signed, clientDataHash[:]...)
	signature, err := fixtureSignature(signed)
	if err != nil {
		return empty, err
	}
	return ohrats_rc_webauthn_types.AuthenticationRequest{
		RelyingParty:       challenge.RelyingParty,
		Challenge:          challenge.Challenge,
		CredentialId:       credential.Id,
		ClientDataJson:     clientData,
		AuthenticatorData:  authenticatorData,
		Signature:          signature,
		Credential:         credential,
		ExpectedUserHandle: []uint8(userID),
		ResponseUserHandle: witTypes.Some([]uint8(userID)),
		UserHandleMode:     ohrats_rc_webauthn_types.UserHandleMode_Identified,
		UserVerification:   ohrats_rc_webauthn_types.UserVerification_Required,
	}, nil
}

// fixtureCredential builds the COSE ES256 key deterministically. Keys are
// emitted in ascending COSE label order, matching the previous CBOR encoder.
func fixtureCredential(fixture string, signCount uint32) ohrats_rc_webauthn_types.StoredCredential {
	cose := []uint8{0xa5}
	cose = append(cose, 0x22, 0x58, 0x20)
	cose = append(cose, publicY[:]...)
	cose = append(cose, 0x21, 0x58, 0x20)
	cose = append(cose, publicX[:]...)
	cose = append(cose, 0x20, 0x01)
	cose = append(cose, 0x01, 0x02)
	cose = append(cose, 0x03, 0x26)
	return ohrats_rc_webauthn_types.StoredCredential{
		Id:             []uint8("credential-" + fixture),
		Algorithm:      "es256",
		PublicKeyCose:  cose,
		SignCount:      signCount,
		BackupEligible: true,
		BackupState:    false,
	}
}

func relyingParty() ohrats_rc_webauthn_types.RelyingParty {
	return ohrats_rc_webauthn_types.RelyingParty{Id: rpID, Origin: origin}
}
