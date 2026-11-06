package crypto

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
)

var b64Std = base64.StdEncoding

func ControlGrantChallenge(grant string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("rc-control-grant-v1\n%s", grant)))
	return b64URL.EncodeToString(digest[:])
}

type clientData struct {
	Type        string `json:"type"`
	Challenge   string `json:"challenge"`
	Origin      string `json:"origin"`
	CrossOrigin bool   `json:"crossOrigin"`
}

type assertionResponse struct {
	ClientDataJSON    string `json:"clientDataJSON"`
	AuthenticatorData string `json:"authenticatorData"`
	Signature         string `json:"signature"`
}

type webauthnAssertion struct {
	Type     string            `json:"type"`
	RawID    string            `json:"rawId"`
	Response assertionResponse `json:"response"`
}

type storedEc2Key struct {
	Curve string `json:"curve"`
	X     string `json:"x"`
	Y     string `json:"y"`
}

type storedRsaKey struct {
	N string   `json:"n"`
	E [3]uint8 `json:"e"`
}

type storedCoseKey struct {
	Type string                     `json:"type_"`
	Key  map[string]json.RawMessage `json:"key"`
}

func VerifyWebauthnAssertion(assertionJSON, credentialID, storedPublicKey, challenge, origin, rpID string) error {
	var assertion webauthnAssertion
	if err := json.Unmarshal([]byte(assertionJSON), &assertion); err != nil {
		return ErrAssertion
	}

	rawIDBytes, err := b64URL.DecodeString(assertion.RawID)
	if err != nil {
		return ErrAssertion
	}
	if assertion.Type != "public-key" || b64URL.EncodeToString(rawIDBytes) != credentialID {
		return ErrCredentialMismatch
	}

	clientDataJSON, err := b64URL.DecodeString(assertion.Response.ClientDataJSON)
	if err != nil {
		return ErrAssertion
	}

	var cd clientData
	if err := json.Unmarshal(clientDataJSON, &cd); err != nil {
		return ErrAssertion
	}
	if cd.Type != "webauthn.get" || cd.Challenge != challenge || !sameOrigin(cd.Origin, origin) || cd.CrossOrigin {
		return ErrVerification
	}

	authData, err := b64URL.DecodeString(assertion.Response.AuthenticatorData)
	if err != nil || len(authData) < 37 {
		return ErrVerification
	}

	rpHash := sha256.Sum256([]byte(rpID))
	if !bytes.Equal(authData[:32], rpHash[:]) {
		return ErrVerification
	}

	flags := authData[32]
	if flags&0x01 == 0 || flags&0x04 == 0 || (flags&0x10 != 0 && flags&0x08 == 0) {
		return ErrVerification
	}

	keyBytes, err := b64Std.DecodeString(storedPublicKey)
	if err != nil {
		return ErrStoredPasskey
	}
	var stored storedCoseKey
	if err := json.Unmarshal(keyBytes, &stored); err != nil {
		return ErrStoredPasskey
	}

	clientHash := sha256.Sum256(clientDataJSON)
	var signed []byte
	signed = append(signed, authData...)
	signed = append(signed, clientHash[:]...)

	sigBytes, err := b64URL.DecodeString(assertion.Response.Signature)
	if err != nil {
		return ErrAssertion
	}

	return verifyStoredKey(&stored, signed, sigBytes)
}

func verifyStoredKey(key *storedCoseKey, signedMsg, sig []byte) error {
	hash := sha256.Sum256(signedMsg)
	switch key.Type {
	case "ES256":
		raw, ok := key.Key["EC_EC2"]
		if !ok {
			return ErrStoredPasskey
		}
		var ec storedEc2Key
		if err := json.Unmarshal(raw, &ec); err != nil || ec.Curve != "SECP256R1" {
			return ErrStoredPasskey
		}
		xBytes, err := b64URL.DecodeString(ec.X)
		if err != nil || len(xBytes) != 32 {
			return ErrStoredPasskey
		}
		yBytes, err := b64URL.DecodeString(ec.Y)
		if err != nil || len(yBytes) != 32 {
			return ErrStoredPasskey
		}
		pub := &ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(xBytes),
			Y:     new(big.Int).SetBytes(yBytes),
		}
		if !ecdsa.VerifyASN1(pub, hash[:], sig) {
			return ErrVerification
		}
		return nil

	case "RS256":
		raw, ok := key.Key["RSA"]
		if !ok {
			return ErrStoredPasskey
		}
		var rsaKey storedRsaKey
		if err := json.Unmarshal(raw, &rsaKey); err != nil {
			return ErrStoredPasskey
		}
		nBytes, err := b64URL.DecodeString(rsaKey.N)
		if err != nil {
			return ErrStoredPasskey
		}
		e := int(rsaKey.E[0])<<16 | int(rsaKey.E[1])<<8 | int(rsaKey.E[2])
		pub := &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: e,
		}
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hash[:], sig); err != nil {
			return ErrVerification
		}
		return nil

	default:
		return ErrStoredPasskey
	}
}

func sameOrigin(a, b string) bool {
	u1, err1 := url.Parse(a)
	u2, err2 := url.Parse(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return u1.Scheme == u2.Scheme && u1.Host == u2.Host
}
