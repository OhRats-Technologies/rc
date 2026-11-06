// Package identityfixture seeds and verifies restart-persistent identity state.
package identityfixture

import (
	"errors"
	"fmt"
	"time"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_webauthn_types"
)

const (
	lifetimeMs   uint64 = 60_000
	expiryTestMs uint64 = 200
)

// Invoke dispatches the identity fixture commands.
func Invoke(command string, args []string) (uint32, error) {
	switch command {
	case "identity-seed":
		return seed(args)
	case "identity-verify":
		return verify(args)
	default:
		return 0, fmt.Errorf("unsupported command %q", command)
	}
}

func fixtureCredential(fixture string, signCount uint32) ohrats_rc_webauthn_types.StoredCredential {
	return ohrats_rc_webauthn_types.StoredCredential{
		Id:             []byte("credential-" + fixture),
		Algorithm:      "es256",
		PublicKeyCose:  []byte{0xa5, 0x01, 0x02},
		SignCount:      signCount,
		BackupEligible: true,
		BackupState:    false,
	}
}

func validateFixture(value string) error {
	if len(value) == 0 || len(value) > 48 {
		return errors.New("invalid fixture id")
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		lowercase := character >= 'a' && character <= 'z'
		digit := character >= '0' && character <= '9'
		if !lowercase && !digit && character != '-' {
			return errors.New("invalid fixture id")
		}
	}
	return nil
}

func userID(fixture string) string {
	return "fixture-user-" + fixture
}

func ceremonyID(fixture string) string {
	return "fixture-ceremony-" + fixture
}

func saturatingAdd(value uint64, amount uint64) uint64 {
	if value > ^uint64(0)-amount {
		return ^uint64(0)
	}
	return value + amount
}

func nowMs() uint64 {
	value := time.Now().UnixMilli()
	if value < 0 {
		return 0
	}
	return uint64(value)
}

func takeOption[T any](value witTypes.Option[T]) (T, bool) {
	if value.IsSome() {
		return value.Some(), true
	}
	var zero T
	return zero, false
}
