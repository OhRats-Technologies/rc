package crypto

import "errors"

var (
	ErrBase64         = errors.New("invalid base64url")
	ErrKeyLength      = errors.New("invalid key length")
	ErrKdf            = errors.New("key derivation failed")
	ErrAuthentication = errors.New("control frame authentication failed")
	ErrSignature      = errors.New("invalid signature")
)

var (
	ErrAssertion          = errors.New("invalid passkey assertion")
	ErrCredentialMismatch = errors.New("passkey credential mismatch")
	ErrStoredPasskey      = errors.New("invalid stored passkey")
	ErrVerification       = errors.New("passkey grant verification failed")
)
