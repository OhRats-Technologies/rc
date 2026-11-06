package export_wit_world

import (
	"encoding/hex"
	"errors"
	"fmt"

	"wit_component/ohrats_rc_crypto_types"
	"wit_component/ohrats_rc_crypto_verifier"
	"wit_component/ohrats_rc_keys_custody"
	"wit_component/ohrats_rc_keys_types"
)

// payload is the deterministic message signed by the custody fixture.
var payload = []byte("rc-key-custody-fixture-v1")

func invoke(command string, args []string) (uint32, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("usage: rc %s <slot>", command)
	}
	slot := args[0]
	switch command {
	case "key-custody-public":
		ensured := ohrats_rc_keys_custody.Ensure(slot, ohrats_rc_keys_types.KeyAlgorithm_Ed25519)
		if ensured.IsErr() {
			return 0, errors.New(ensured.Err())
		}
		return printPublic(ensured.Ok())
	case "key-custody-verify":
		return verify(slot)
	case "key-custody-lookup":
		return lookup(slot)
	case "key-custody-remove":
		return remove(slot)
	}
	return 0, fmt.Errorf("unsupported command %q", command)
}

func printPublic(key ohrats_rc_keys_types.PublicKey) (uint32, error) {
	if key.Algorithm != ohrats_rc_keys_types.KeyAlgorithm_Ed25519 || len(key.Bytes) != 32 {
		return 0, errors.New("custody returned an invalid Ed25519 public key")
	}
	fmt.Println(hex.EncodeToString(key.Bytes))
	return 0, nil
}

func verify(slot string) (uint32, error) {
	ensured := ohrats_rc_keys_custody.Ensure(slot, ohrats_rc_keys_types.KeyAlgorithm_Ed25519)
	if ensured.IsErr() {
		return 0, errors.New(ensured.Err())
	}
	key := ensured.Ok()
	signed := ohrats_rc_keys_custody.Sign(slot, ohrats_rc_keys_types.KeyAlgorithm_Ed25519, payload)
	if signed.IsErr() {
		return 0, errors.New(signed.Err())
	}
	verified := ohrats_rc_crypto_verifier.Verify(
		ohrats_rc_crypto_types.SignatureAlgorithm_Ed25519,
		ohrats_rc_crypto_types.PublicKey{
			Algorithm: ohrats_rc_crypto_types.SignatureAlgorithm_Ed25519,
			Bytes:     key.Bytes,
		},
		payload,
		signed.Ok(),
	)
	if verified.IsErr() {
		return 0, errors.New(verified.Err())
	}
	if !verified.Ok() {
		return 0, errors.New("custody signature did not verify")
	}
	fmt.Println("key custody fixture: ok")
	return 0, nil
}

func lookup(slot string) (uint32, error) {
	found := ohrats_rc_keys_custody.Lookup(slot, ohrats_rc_keys_types.KeyAlgorithm_Ed25519)
	if found.IsErr() {
		return 0, errors.New(found.Err())
	}
	option := found.Ok()
	if option.IsSome() {
		return printPublic(option.Some())
	}
	fmt.Println("missing")
	return 0, nil
}

func remove(slot string) (uint32, error) {
	removed := ohrats_rc_keys_custody.Remove(slot, ohrats_rc_keys_types.KeyAlgorithm_Ed25519)
	if removed.IsErr() {
		return 0, errors.New(removed.Err())
	}
	if removed.Ok() {
		fmt.Println("removed")
	} else {
		fmt.Println("missing")
	}
	return 0, nil
}
