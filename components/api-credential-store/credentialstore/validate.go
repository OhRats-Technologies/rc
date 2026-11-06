package credentialstore

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"sort"
	"unicode"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_api_credentials_types"
)

const (
	maxID        = 128
	maxName      = 80
	maxKey       = 64
	maxNonce     = 128
	maxSignature = 128
	maxCode      = 256
)

// text rejects empty, oversized, or control-containing text. The maximum is a
// byte length because the durable limits are byte-oriented.
func text(value string, label string, max int) error {
	if value == "" || len(value) > max || hasControl(value) {
		return fmt.Errorf("invalid %s", label)
	}
	return nil
}

func identifier(value string, label string) error {
	if err := text(value, label, maxID); err != nil {
		return err
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if isAlphanumeric(character) || character == '.' || character == '_' ||
			character == ':' || character == '-' {
			continue
		}
		return fmt.Errorf("invalid %s", label)
	}
	return nil
}

func publicKey(value string) error {
	if err := text(value, "public key", maxKey); err != nil {
		return err
	}
	decoded, err := decodeBase64(value)
	if err != nil || len(decoded) != 32 {
		return errors.New("invalid public key")
	}
	return nil
}

func nonce(value string) error {
	if err := text(value, "nonce", maxNonce); err != nil {
		return err
	}
	if len(value) < 16 || len(value) > maxNonce {
		return errors.New("invalid nonce")
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if isAlphanumeric(character) || character == '_' || character == '-' {
			continue
		}
		return errors.New("invalid nonce")
	}
	return nil
}

// scopes sorts and deduplicates requested scopes, defaulting to read access.
func scopes(value []ohrats_rc_api_credentials_types.Scope) []ohrats_rc_api_credentials_types.Scope {
	requested := append([]uint8{}, value...)
	sort.Slice(requested, func(first int, second int) bool {
		return requested[first] < requested[second]
	})
	result := make([]ohrats_rc_api_credentials_types.Scope, 0, len(requested))
	for _, scope := range requested {
		if len(result) > 0 && result[len(result)-1] == scope {
			continue
		}
		result = append(result, scope)
	}
	if len(result) == 0 {
		result = append(result, ohrats_rc_api_credentials_types.Scope_Read)
	}
	return result
}

// lifetime resolves a requested lifetime into an absolute expiry. Zero means
// the credential never expires.
func lifetime(value witTypes.Option[ohrats_rc_api_credentials_types.Lifetime], now uint64) uint64 {
	if value.IsNone() {
		return 0
	}
	var age uint64
	switch value.Some() {
	case ohrats_rc_api_credentials_types.Lifetime_OneHour:
		age = 60 * 60
	case ohrats_rc_api_credentials_types.Lifetime_OneDay:
		age = 24 * 60 * 60
	case ohrats_rc_api_credentials_types.Lifetime_SevenDays:
		age = 7 * 24 * 60 * 60
	case ohrats_rc_api_credentials_types.Lifetime_ThirtyDays:
		age = 30 * 24 * 60 * 60
	case ohrats_rc_api_credentials_types.Lifetime_NinetyDays:
		age = 90 * 24 * 60 * 60
	case ohrats_rc_api_credentials_types.Lifetime_OneEightyDays:
		age = 180 * 24 * 60 * 60
	case ohrats_rc_api_credentials_types.Lifetime_OneYear:
		age = 365 * 24 * 60 * 60
	default:
		return 0
	}
	delta := age * 1000
	if now > math.MaxUint64-delta {
		return math.MaxUint64
	}
	return now + delta
}

func hasControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func isAlphanumeric(character byte) bool {
	return (character >= 'a' && character <= 'z') ||
		(character >= 'A' && character <= 'Z') ||
		(character >= '0' && character <= '9')
}

func decodeBase64(value string) ([]byte, error) {
	return base64.RawURLEncoding.Strict().DecodeString(value)
}
