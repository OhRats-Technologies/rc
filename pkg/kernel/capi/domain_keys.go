//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

type keyRecord struct {
	Algorithm string `json:"algorithm"`
	Private   string `json:"private"`
}

type keyDomain struct {
	Keys map[string]keyRecord `json:"keys"`
}

func domainKeyEnsure(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid key request")
		return
	}
	slot, algorithm := valueString(argValue(args, 0)), valueEnum(argValue(args, 1))
	if slot == "" || (algorithm != "ed25519" && algorithm != "x25519") {
		setStringResult(results, false, "invalid key request")
		return
	}
	state, err := domainMutate(domainStore(), "keys", func(state *keyDomain) error {
		if state.Keys == nil {
			state.Keys = make(map[string]keyRecord)
		}
		key, exists := state.Keys[slot+"\x00"+algorithm]
		if exists && key.Private != "" {
			return nil
		}
		seed := make([]byte, ed25519.SeedSize)
		if _, err := rand.Read(seed); err != nil {
			return err
		}
		state.Keys[slot+"\x00"+algorithm] = keyRecord{Algorithm: algorithm, Private: base64.RawURLEncoding.EncodeToString(seed)}
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	key := state.Keys[slot+"\x00"+algorithm]
	public, err := publicKeyForRecord(key)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setOK(results, publicKeyRecord(algorithm, public))
}

func domainKeyLookup(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid key request")
		return
	}
	slot, algorithm := valueString(argValue(args, 0)), valueEnum(argValue(args, 1))
	state, _, err := domainLoad[keyDomain](domainStore(), "keys")
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	option := newValue()
	key, ok := state.Keys[slot+"\x00"+algorithm]
	if !ok {
		C.rc_set_none_option(option)
	} else {
		public, err := publicKeyForRecord(key)
		if err != nil {
			setStringResult(results, false, err.Error())
			return
		}
		C.rc_set_some_option(option, publicKeyRecord(algorithm, public))
	}
	setOK(results, option)
}

func domainKeySign(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 3 {
		setStringResult(results, false, "invalid key request")
		return
	}
	slot, algorithm := valueString(argValue(args, 0)), valueEnum(argValue(args, 1))
	state, _, err := domainLoad[keyDomain](domainStore(), "keys")
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	key, ok := state.Keys[slot+"\x00"+algorithm]
	if !ok || algorithm != "ed25519" {
		setStringResult(results, false, "key does not support signing")
		return
	}
	seed, err := base64.RawURLEncoding.DecodeString(key.Private)
	if err != nil || len(seed) != ed25519.SeedSize {
		setStringResult(results, false, "stored key is invalid")
		return
	}
	signature := ed25519.Sign(ed25519.NewKeyFromSeed(seed), valueBytes(argValue(args, 2)))
	payload := newValue()
	setBytesValue(payload, signature)
	setOK(results, payload)
}

func domainKeyRemove(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid key request")
		return
	}
	slot, algorithm := valueString(argValue(args, 0)), valueEnum(argValue(args, 1))
	existed := false
	_, err := domainMutate(domainStore(), "keys", func(state *keyDomain) error {
		key := slot + "\x00" + algorithm
		_, existed = state.Keys[key]
		delete(state.Keys, key)
		return nil
	})
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	payload := newValue()
	C.rc_set_bool(payload, C.bool(existed))
	setOK(results, payload)
}

func domainCryptoVerify(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 4 {
		setStringResult(results, false, "invalid verification request")
		return
	}
	algorithm := valueEnum(argValue(args, 0))
	public := argValue(args, 1)
	if algorithm != "ed25519" || fieldString(public, "algorithm") != "ed25519" {
		setStringResult(results, false, "unsupported signature algorithm")
		return
	}
	err := verifyEd25519(fieldBytes(public, "bytes"), valueBytes(argValue(args, 2)), valueBytes(argValue(args, 3)))
	payload := newValue()
	C.rc_set_bool(payload, C.bool(err == nil))
	setOK(results, payload)
}

func domainCryptoSHA256(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid hash request")
		return
	}
	digest := sha256.Sum256(valueBytes(argValue(args, 0)))
	payload := newValue()
	setBytesValue(payload, digest[:])
	setOK(results, payload)
}

func publicKeyForRecord(key keyRecord) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(key.Private)
	if err != nil || len(raw) != ed25519.SeedSize {
		return nil, errors.New("stored key is invalid")
	}
	switch key.Algorithm {
	case "ed25519":
		return ed25519.NewKeyFromSeed(raw).Public().(ed25519.PublicKey), nil
	case "x25519":
		private, err := ecdh.X25519().NewPrivateKey(raw)
		if err != nil {
			return nil, err
		}
		return private.PublicKey().Bytes(), nil
	default:
		return nil, errors.New("unsupported key algorithm")
	}
}

func publicKeyRecord(algorithm string, bytes []byte) *C.wasmtime_component_val_t {
	return newOrderedRecord([]fieldValue{
		{name: "algorithm", value: newEnumValue(algorithm)},
		{name: "bytes", value: newBytesValue(bytes)},
	})
}

func verifyEd25519(public, payload, signature []byte) error {
	if len(public) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize {
		return errors.New("invalid signature")
	}
	if !ed25519.Verify(ed25519.PublicKey(public), payload, signature) {
		return errors.New("invalid signature")
	}
	return nil
}
