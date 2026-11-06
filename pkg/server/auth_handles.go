package server

import (
	"encoding/hex"
	"errors"

	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
)

// verifyWithAccountHandles verifies an assertion whose user handle may use
// either encoding of the account ID: the UTF-8 bytes this server writes, or the
// 16 raw UUID bytes that the Rust-era server (webauthn-rs) wrote for the same
// accounts. Both name the same account, so either match is accepted.
func verifyWithAccountHandles(userID string, verify func(userHandle []byte) error) error {
	err := verify([]byte(userID))
	if !errors.Is(err, webauthn.ErrUserHandleMismatch) {
		return err
	}
	if raw, ok := canonicalUUIDBytes(userID); ok {
		return verify(raw)
	}
	return err
}

// canonicalUUIDBytes decodes an 8-4-4-4-12 hexadecimal UUID into 16 bytes.
func canonicalUUIDBytes(value string) ([]byte, bool) {
	if len(value) != 36 {
		return nil, false
	}
	for _, index := range []int{8, 13, 18, 23} {
		if value[index] != '-' {
			return nil, false
		}
	}
	hexDigits := value[:8] + value[9:13] + value[14:18] + value[19:23] + value[24:]
	raw, err := hex.DecodeString(hexDigits)
	if err != nil || len(raw) != 16 {
		return nil, false
	}
	return raw, true
}
