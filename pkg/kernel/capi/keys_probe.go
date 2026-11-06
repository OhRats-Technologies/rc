//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"crypto/ed25519"
	"errors"
)

func (h *Host) RunKeysProbe() error {
	if h == nil || h.ctx == nil {
		return errors.New("host is unavailable")
	}
	ed, err := h.ensureKey("probe-slot", "ed25519")
	if err != nil {
		return err
	}
	public, err := h.secretPublic(ed)
	if err != nil {
		return err
	}
	signature, err := h.secretSign(ed, []byte("hello"))
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(public), []byte("hello"), signature) {
		return errors.New("ed25519 signature did not verify")
	}
	first, err := h.ensureKey("probe-x1", "x25519")
	if err != nil {
		return err
	}
	second, err := h.ensureKey("probe-x2", "x25519")
	if err != nil {
		return err
	}
	firstPublic, err := h.secretPublic(first)
	if err != nil {
		return err
	}
	secondPublic, err := h.secretPublic(second)
	if err != nil {
		return err
	}
	firstShared, err := h.secretAgree(first, secondPublic)
	if err != nil {
		return err
	}
	secondShared, err := h.secretAgree(second, firstPublic)
	if err != nil {
		return err
	}
	session, err := h.derive(firstShared, secondShared, []byte("salt"), []byte("info"))
	if err != nil {
		return err
	}
	nonce := make([]byte, 12)
	aad := []byte("aad")
	ciphertext, err := h.sessionKeyUse(session, "encrypt", nonce, aad, []byte("secret"))
	if err != nil {
		return err
	}
	plaintext, err := h.sessionKeyUse(session, "decrypt", nonce, aad, ciphertext)
	if err != nil {
		return err
	}
	if string(plaintext) != "secret" {
		return errors.New("session key round-trip failed")
	}
	return nil
}

type keyHandle struct {
	rep  uint32
	ty   uint32
	more []byte
}

func (h *Host) ensureKey(slot, algorithm string) (keyHandle, error) {
	result, err := h.callKey("ensure", []C.wasmtime_component_val_t{*newStringValue(slot), *newEnumValue(algorithm)})
	if err != nil {
		return keyHandle{}, err
	}
	return h.resourceHandle(result)
}

func (h *Host) callKey(name string, args []C.wasmtime_component_val_t) (C.wasmtime_component_val_t, error) {
	// Keys host functions are imports, so invoke the callback directly with a
	// synthetic function call is not possible; use the exported Go functions.
	switch name {
	case "ensure":
		var results C.wasmtime_component_val_t
		keysEnsure(h.ctx, &args[0], C.size_t(len(args)), &results, 1)
		return results, nil
	default:
		return C.wasmtime_component_val_t{}, errors.New("unknown key operation")
	}
}

func (h *Host) resourceHandle(value C.wasmtime_component_val_t) (keyHandle, error) {
	if !bool(C.rc_result_ok(&value)) {
		return keyHandle{}, errors.New(valueString(C.rc_result_val(&value)))
	}
	resource := C.rc_result_val(&value)
	rep, ty := resourceRep(h.ctx, resource)
	return keyHandle{rep: rep, ty: ty}, nil
}

func (h *Host) secretPublic(rep keyHandle) ([]byte, error) {
	fresh, err := hostResource(h.ctx, rep.rep, rep.ty)
	if err != nil {
		return nil, err
	}
	args := []C.wasmtime_component_val_t{*resourceValue(fresh)}
	var results C.wasmtime_component_val_t
	secretPublicKey(h.ctx, &args[0], 1, &results, 1)
	if !bool(C.rc_result_ok(&results)) {
		return nil, errors.New(valueString(C.rc_result_val(&results)))
	}
	return valueBytes(recordField(C.rc_result_val(&results), "bytes")), nil
}

func (h *Host) secretSign(rep keyHandle, payload []byte) ([]byte, error) {
	fresh, err := hostResource(h.ctx, rep.rep, rep.ty)
	if err != nil {
		return nil, err
	}
	args := []C.wasmtime_component_val_t{*resourceValue(fresh), *newBytesValue(payload)}
	var results C.wasmtime_component_val_t
	secretSign(h.ctx, &args[0], 2, &results, 1)
	if !bool(C.rc_result_ok(&results)) {
		return nil, errors.New(valueString(C.rc_result_val(&results)))
	}
	return valueBytes(C.rc_result_val(&results)), nil
}

func (h *Host) secretAgree(rep keyHandle, peer []byte) (keyHandle, error) {
	fresh, err := hostResource(h.ctx, rep.rep, rep.ty)
	if err != nil {
		return keyHandle{}, err
	}
	args := []C.wasmtime_component_val_t{*resourceValue(fresh), *newBytesValue(peer)}
	var results C.wasmtime_component_val_t
	secretAgree(h.ctx, &args[0], 2, &results, 1)
	return h.resourceHandle(results)
}

func (h *Host) derive(first, second keyHandle, salt, info []byte) (keyHandle, error) {
	firstFresh, err := hostResource(h.ctx, first.rep, first.ty)
	if err != nil {
		return keyHandle{}, err
	}
	secondFresh, err := hostResource(h.ctx, second.rep, second.ty)
	if err != nil {
		return keyHandle{}, err
	}
	args := []C.wasmtime_component_val_t{*resourceValue(firstFresh), *resourceValue(secondFresh), *newBytesValue(salt), *newBytesValue(info)}
	var results C.wasmtime_component_val_t
	keysDerive(h.ctx, &args[0], 4, &results, 1)
	return h.resourceHandle(results)
}

func (h *Host) sessionKeyUse(rep keyHandle, operation string, nonce, aad, input []byte) ([]byte, error) {
	fresh, err := hostResource(h.ctx, rep.rep, rep.ty)
	if err != nil {
		return nil, err
	}
	args := []C.wasmtime_component_val_t{*resourceValue(fresh), *newBytesValue(nonce), *newBytesValue(input), *newBytesValue(aad)}
	var results C.wasmtime_component_val_t
	if operation == "encrypt" {
		sessionEncrypt(h.ctx, &args[0], 4, &results, 1)
	} else {
		sessionDecrypt(h.ctx, &args[0], 4, &results, 1)
	}
	if !bool(C.rc_result_ok(&results)) {
		return nil, errors.New(valueString(C.rc_result_val(&results)))
	}
	return valueBytes(C.rc_result_val(&results)), nil
}
