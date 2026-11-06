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

func beginRegistration(
	kind string,
	userID string,
	displayName string,
	relyingParty RelyingParty,
) (string, map[string]any, error) {
	challenge, err := randomBytes(32)
	if err != nil {
		return "", nil, err
	}
	ceremonyID, err := randomID()
	if err != nil {
		return "", nil, err
	}
	passkeys := ohrats_rc_identity_credentials.All(witTypes.Some(userID))
	if passkeys.IsErr() {
		return "", nil, errors.New(passkeys.Err())
	}
	exclude := make([]any, 0, len(passkeys.Ok()))
	for _, passkey := range passkeys.Ok() {
		exclude = append(exclude, map[string]any{
			"id":   encodeBase64URL(passkey.Credential.Id),
			"type": "public-key",
		})
	}
	options := map[string]any{
		"challenge": encodeBase64URL(challenge),
		"rp": map[string]any{
			"id":   relyingParty.ID,
			"name": "RC",
		},
		"user": map[string]any{
			"id":          encodeBase64URL([]byte(userID)),
			"name":        displayName,
			"displayName": displayName,
		},
		"pubKeyCredParams": []any{
			map[string]any{"type": "public-key", "alg": -7},
		},
		"timeout":     ceremonyTTLMs,
		"attestation": "none",
		"authenticatorSelection": map[string]any{
			"residentKey":        "required",
			"requireResidentKey": true,
			"userVerification":   "required",
		},
		"excludeCredentials": exclude,
	}
	metadata, err := json.Marshal(registrationMeta{DisplayName: displayName})
	if err != nil {
		return "", nil, err
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
		Kind:        kind,
		UserId:      witTypes.Some(userID),
		Metadata:    metadata,
		State:       state,
		ExpiresAtMs: nowMs() + ceremonyTTLMs,
	})
	if put.IsErr() {
		return "", nil, errors.New(put.Err())
	}
	return ceremonyID, options, nil
}

func finishRegistration(
	kind string,
	ceremonyID string,
	response json.RawMessage,
) (RegistrationResult, error) {
	taken := ohrats_rc_identity_ceremonies.Take(ceremonyID, kind)
	if taken.IsErr() {
		return RegistrationResult{}, errors.New(taken.Err())
	}
	ceremony, ok := optionValue(taken.Ok())
	if !ok {
		return RegistrationResult{}, errors.New("registration expired")
	}
	userID, ok := optionValue(ceremony.UserId)
	if !ok {
		return RegistrationResult{}, errors.New("registration expired")
	}
	var meta registrationMeta
	if err := json.Unmarshal(ceremony.Metadata, &meta); err != nil {
		return RegistrationResult{}, err
	}
	var state ceremonyState
	if err := json.Unmarshal(ceremony.State, &state); err != nil {
		return RegistrationResult{}, err
	}
	parsed, inner, err := parseCredentialResponse[registrationResponse](response)
	if err != nil {
		return RegistrationResult{}, err
	}
	if parsed.RawID == nil || parsed.ID == nil || parsed.Type == nil ||
		inner.ClientDataJSON == nil || inner.AttestationObject == nil {
		return RegistrationResult{}, errors.New("invalid passkey response")
	}
	credentialID, err := decodeBase64URL(*parsed.RawID, "credential id")
	if err != nil {
		return RegistrationResult{}, err
	}
	challenge, err := decodeBase64URL(state.Challenge, "challenge")
	if err != nil {
		return RegistrationResult{}, err
	}
	clientData, err := decodeBase64URL(*inner.ClientDataJSON, "client data")
	if err != nil {
		return RegistrationResult{}, err
	}
	attestation, err := decodeBase64URL(*inner.AttestationObject, "attestation")
	if err != nil {
		return RegistrationResult{}, err
	}
	verified := ohrats_rc_webauthn_verifier.VerifyRegistration(
		passkeyAlgorithm,
		ohrats_rc_webauthn_types.RegistrationRequest{
			RelyingParty:      wireRelyingParty(state),
			Challenge:         challenge,
			CredentialId:      credentialID,
			ClientDataJson:    clientData,
			AttestationObject: attestation,
			UserVerification:  ohrats_rc_webauthn_types.UserVerification_Required,
		},
	)
	if verified.IsErr() {
		return RegistrationResult{}, errors.New(verified.Err())
	}
	return RegistrationResult{
		UserID:      userID,
		DisplayName: meta.DisplayName,
		Credential:  verified.Ok().Credential,
	}, nil
}
