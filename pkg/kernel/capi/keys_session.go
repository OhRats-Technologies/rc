//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"unsafe"
)

func sessionEncrypt(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	key, ok := selfSession(ctx, args)
	if !ok || nargs < 3 {
		setStringResult(results, false, "unknown session key")
		return
	}
	block, err := aes.NewCipher(key.key)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	nonce := valueBytes(argValue(args, 1))
	if len(nonce) != gcm.NonceSize() {
		setStringResult(results, false, "invalid nonce")
		return
	}
	ciphertext := gcm.Seal(nil, nonce, valueBytes(argValue(args, 2)), valueBytes(argValue(args, 3)))
	payload := newValue()
	setBytesValue(payload, ciphertext)
	C.rc_set_result(results, true, payload)
}

func sessionDecrypt(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	key, ok := selfSession(ctx, args)
	if !ok || nargs < 3 {
		setStringResult(results, false, "unknown session key")
		return
	}
	block, err := aes.NewCipher(key.key)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	nonce := valueBytes(argValue(args, 1))
	plaintext, err := gcm.Open(nil, nonce, valueBytes(argValue(args, 2)), valueBytes(argValue(args, 3)))
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	payload := newValue()
	setBytesValue(payload, plaintext)
	C.rc_set_result(results, true, payload)
}

func selfSecret(ctx unsafe.Pointer, args *C.wasmtime_component_val_t) (*secretKeyResource, bool) {
	rep, ok := selfRep(ctx, args)
	if !ok {
		return nil, false
	}
	key, ok := lookupResource(rep).(*secretKeyResource)
	return key, ok
}

func selfShared(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, index int) (*sharedSecretResource, bool) {
	if args == nil || C.rc_val_kind(argValue(args, index)) != 21 {
		return nil, false
	}
	rep, ok := selfRepAt(ctx, args, index)
	if !ok {
		return nil, false
	}
	value, ok := lookupResource(rep).(*sharedSecretResource)
	return value, ok
}

func selfSession(ctx unsafe.Pointer, args *C.wasmtime_component_val_t) (*sessionKeyResource, bool) {
	rep, ok := selfRep(ctx, args)
	if !ok {
		return nil, false
	}
	key, ok := lookupResource(rep).(*sessionKeyResource)
	return key, ok
}

func setKeyResult(ctx unsafe.Pointer, results *C.wasmtime_component_val_t, object any, ty uint32) {
	resource, err := hostResource(ctx, addResourceObject(object), ty)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	payload := newValue()
	C.rc_set_resource(payload, resource)
	C.rc_set_result(results, true, payload)
}

func hkdf(ikm, salt, info []byte, size int) []byte {
	if len(salt) == 0 {
		salt = make([]byte, sha256.Size)
	}
	extract := hmac.New(sha256.New, salt)
	extract.Write(ikm)
	prk := extract.Sum(nil)
	result := make([]byte, 0, size)
	previous := []byte{}
	for counter := byte(1); len(result) < size; counter++ {
		mac := hmac.New(sha256.New, prk)
		mac.Write(previous)
		mac.Write(info)
		mac.Write([]byte{counter})
		previous = mac.Sum(nil)
		result = append(result, previous...)
	}
	return result[:size]
}
