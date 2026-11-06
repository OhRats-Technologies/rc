package server

import (
	"errors"
	"testing"

	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
)

func TestVerifyWithAccountHandlesAcceptsRustEraUUIDBytes(t *testing.T) {
	const userID = "e78f4d7f-4492-435a-ba02-036cbba954e6"
	var seen [][]byte
	err := verifyWithAccountHandles(userID, func(handle []byte) error {
		seen = append(seen, append([]byte(nil), handle...))
		if len(handle) == 16 {
			return nil
		}
		return webauthn.ErrUserHandleMismatch
	})
	if err != nil {
		t.Fatalf("UUID handle should verify: %v", err)
	}
	if len(seen) != 2 || string(seen[0]) != userID || len(seen[1]) != 16 {
		t.Fatalf("handles tried = %q", seen)
	}
}

func TestVerifyWithAccountHandlesKeepsOtherFailures(t *testing.T) {
	calls := 0
	boom := errors.New("signature invalid")
	err := verifyWithAccountHandles("e78f4d7f-4492-435a-ba02-036cbba954e6", func([]byte) error {
		calls++
		return boom
	})
	if !errors.Is(err, boom) || calls != 1 {
		t.Fatalf("err = %v, calls = %d; non-handle failures must not retry", err, calls)
	}
}

func TestVerifyWithAccountHandlesNonUUIDIDTriesOnce(t *testing.T) {
	calls := 0
	err := verifyWithAccountHandles("87c90688-50a8-467f-87d3-7a92cfec0e4", func([]byte) error {
		calls++
		return webauthn.ErrUserHandleMismatch
	})
	if !errors.Is(err, webauthn.ErrUserHandleMismatch) || calls != 1 {
		t.Fatalf("err = %v, calls = %d", err, calls)
	}
}
