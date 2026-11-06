package identityhttp

import (
	"encoding/json"
	"errors"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_identity_ceremonies"
	"wit_component/ohrats_rc_identity_credentials"
	"wit_component/ohrats_rc_identity_types"
	"wit_component/ohrats_rc_webauthn_types"
	"wit_component/ohrats_rc_webauthn_verifier"
)

func beginLogin(relyingParty RelyingParty) (string, map[string]any, error) {
	passkeys := ohrats_rc_identity_credentials.All(witTypes.None[string]())
	if passkeys.IsErr() {
		return "", nil, errors.New(passkeys.Err())
	}
	values := passkeys.Ok()
	if len(values) == 0 {
		return "", nil, errors.New("No passkey is registered yet. Complete RC setup first.")
	}
	if len(values) > maxCredentials {
		return "", nil, errors.New("too many passkeys are registered")
	}
	challenge, err := randomBytes(32)
	if err != nil {
		return "", nil, err
	}
	ceremonyID, err := randomID()
	if err != nil {
		return "", nil, err
	}
	allow := make([]any, 0, len(values))
	for _, passkey := range values {
		allow = append(allow, map[string]any{
			"id":   encodeBase64URL(passkey.Credential.Id),
			"type": "public-key",
		})
	}
	options := map[string]any{
		"challenge":        encodeBase64URL(challenge),
		"rpId":             relyingParty.ID,
		"timeout":          ceremonyTTLMs,
		"userVerification": "required",
		"allowCredentials": allow,
	}
	state, err := json.Marshal(ceremonyState{
		Challenge: encodeBase64URL(challenge),
		RPID:      relyingParty.ID,
		Origin:    relyingParty.Origin,
	})
	if err != nil {
		return "", nil, err
	}
	put := ohrats_rc_identity_ceremonies.Put(ohrats_rc_identity_types.Ceremony{
		Id:          ceremonyID,
		Kind:        "login",
		UserId:      witTypes.None[string](),
		Metadata:    []byte{},
		State:       state,
		ExpiresAtMs: nowMs() + ceremonyTTLMs,
	})
	if put.IsErr() {
		return "", nil, errors.New(put.Err())
	}
	return ceremonyID, options, nil
}

func finishLogin(ceremonyID string, response json.RawMessage) (string, error) {
	taken := ohrats_rc_identity_ceremonies.Take(ceremonyID, "login")
	if taken.IsErr() {
		return "", errors.New(taken.Err())
	}
	ceremony, ok := optionValue(taken.Ok())
	if !ok {
		return "", errors.New("authentication expired")
	}
	var state ceremonyState
	if err := json.Unmarshal(ceremony.State, &state); err != nil {
		return "", err
	}
	parsed, inner, err := parseCredentialResponse[authenticationResponse](response)
	if err != nil {
		return "", err
	}
	if parsed.RawID == nil || parsed.ID == nil || parsed.Type == nil ||
		inner.ClientDataJSON == nil || inner.AuthenticatorData == nil || inner.Signature == nil {
		return "", errors.New("invalid passkey response")
	}
	credentialID, err := decodeBase64URL(*parsed.RawID, "credential id")
	if err != nil {
		return "", err
	}
	passkeyResult := ohrats_rc_identity_credentials.GetByCredentialId(credentialID)
	if passkeyResult.IsErr() {
		return "", errors.New(passkeyResult.Err())
	}
	passkey, ok := optionValue(passkeyResult.Ok())
	if !ok {
		return "", errors.New("unknown passkey")
	}
	challenge, err := decodeBase64URL(state.Challenge, "challenge")
	if err != nil {
		return "", err
	}
	clientData, err := decodeBase64URL(*inner.ClientDataJSON, "client data")
	if err != nil {
		return "", err
	}
	authenticatorData, err := decodeBase64URL(*inner.AuthenticatorData, "authenticator data")
	if err != nil {
		return "", err
	}
	signature, err := decodeBase64URL(*inner.Signature, "signature")
	if err != nil {
		return "", err
	}
	responseUserHandle := witTypes.None[[]byte]()
	if inner.UserHandle != nil {
		userHandle, err := decodeBase64URL(*inner.UserHandle, "user handle")
		if err != nil {
			return "", err
		}
		responseUserHandle = witTypes.Some(userHandle)
	}
	verified := ohrats_rc_webauthn_verifier.VerifyAuthentication(
		passkeyAlgorithm,
		ohrats_rc_webauthn_types.AuthenticationRequest{
			RelyingParty:       wireRelyingParty(state),
			Challenge:          challenge,
			CredentialId:       credentialID,
			ClientDataJson:     clientData,
			AuthenticatorData:  authenticatorData,
			Signature:          signature,
			Credential:         passkey.Credential,
			ExpectedUserHandle: []byte(passkey.UserId),
			ResponseUserHandle: responseUserHandle,
			UserHandleMode:     ohrats_rc_webauthn_types.UserHandleMode_Identified,
			UserVerification:   ohrats_rc_webauthn_types.UserVerification_Required,
		},
	)
	if verified.IsErr() {
		return "", errors.New(verified.Err())
	}
	updated := ohrats_rc_identity_credentials.Update(passkey.Id, verified.Ok().Credential, nowMs())
	if updated.IsErr() {
		return "", errors.New(updated.Err())
	}
	return passkey.UserId, nil
}
