package passkey

// parseNoneAttestation decodes a WebAuthn attestation object and accepts only
// the "none" format with an empty attestation statement.
func parseNoneAttestation(raw []byte) (authenticatorData, error) {
	if len(raw) > maxAttestationObjectBytes {
		return authenticatorData{}, ErrPayloadTooLarge
	}
	value, err := decodeCBORValue(raw)
	if err != nil {
		return authenticatorData{}, ErrInvalidCBOR
	}
	mapping, ok := value.(cborMap)
	if !ok {
		return authenticatorData{}, ErrInvalidCBOR
	}
	formatValue, found := mapText(mapping, "fmt")
	format, ok := formatValue.(string)
	if !found || !ok {
		return authenticatorData{}, ErrInvalidCBOR
	}
	if format != "none" {
		return authenticatorData{}, ErrUnsupportedAttestation
	}
	authValue, found := mapText(mapping, "authData")
	authData, ok := authValue.([]byte)
	if !found || !ok {
		return authenticatorData{}, ErrInvalidCBOR
	}
	data, err := parseAuthenticatorData(authData)
	if err != nil {
		return authenticatorData{}, err
	}
	statementValue, found := mapText(mapping, "attStmt")
	statement, ok := statementValue.(cborMap)
	if !found || !ok || len(statement.keys) != 0 {
		return authenticatorData{}, ErrInvalidAttestation
	}
	return data, nil
}
