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
	"errors"
	"sync"
	"unsafe"
)

const (
	resourceSecretKey    uint32 = 5
	resourceSessionKey   uint32 = 6
	resourceSharedSecret uint32 = 7
)

type secretKeyResource struct {
	algorithm string
	ed        ed25519.PrivateKey
	x         *ecdh.PrivateKey
}

type sharedSecretResource struct {
	bytes []byte
}

type sessionKeyResource struct {
	key []byte
}

var keySlots = struct {
	sync.Mutex
	values map[string]*secretKeyResource
}{values: make(map[string]*secretKeyResource)}

func generateSecret(algorithm string) (*secretKeyResource, error) {
	switch algorithm {
	case "ed25519":
		_, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		return &secretKeyResource{algorithm: algorithm, ed: private}, nil
	case "x25519":
		private, err := ecdh.X25519().GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		return &secretKeyResource{algorithm: algorithm, x: private}, nil
	default:
		return nil, errors.New("unsupported key algorithm")
	}
}

func keysEnsure(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid key request")
		return
	}
	slot := valueString(argValue(args, 0))
	algorithm := valueEnum(argValue(args, 1))
	keySlots.Lock()
	key := keySlots.values[slot+"\x00"+algorithm]
	if key == nil {
		var err error
		key, err = generateSecret(algorithm)
		if err != nil {
			keySlots.Unlock()
			setStringResult(results, false, err.Error())
			return
		}
		keySlots.values[slot+"\x00"+algorithm] = key
	}
	keySlots.Unlock()
	setKeyResult(ctx, results, key, resourceSecretKey)
}

func keysOpen(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid key request")
		return
	}
	keySlots.Lock()
	key := keySlots.values[valueString(argValue(args, 0))+"\x00"+valueEnum(argValue(args, 1))]
	keySlots.Unlock()
	option := newValue()
	if key == nil {
		C.rc_set_none_option(option)
	} else {
		resource, err := hostResource(ctx, addResourceObject(key), resourceSecretKey)
		if err != nil {
			setStringResult(results, false, err.Error())
			return
		}
		C.rc_set_some_option(option, resourceValue(resource))
	}
	C.rc_set_result(results, true, option)
}

func keysGenerate(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid key request")
		return
	}
	key, err := generateSecret(valueEnum(argValue(args, 0)))
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setKeyResult(ctx, results, key, resourceSecretKey)
}

func keysRemove(_ unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid key request")
		return
	}
	keySlots.Lock()
	key := valueString(argValue(args, 0)) + "\x00" + valueEnum(argValue(args, 1))
	_, existed := keySlots.values[key]
	delete(keySlots.values, key)
	keySlots.Unlock()
	payload := newValue()
	C.rc_set_bool(payload, C.bool(existed))
	C.rc_set_result(results, true, payload)
}

func secretPublicKey(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	key, ok := selfSecret(ctx, args)
	if !ok {
		setStringResult(results, false, "unknown secret key")
		return
	}
	var bytes []byte
	if key.algorithm == "ed25519" {
		bytes = key.ed.Public().(ed25519.PublicKey)
	} else {
		bytes = key.x.PublicKey().Bytes()
	}
	value := newValue()
	C.rc_alloc_record(value, 2)
	*C.rc_record_item(value, 0, cString("algorithm"), 9) = *newEnumValue(key.algorithm)
	*C.rc_record_item(value, 1, cString("bytes"), 5) = *newBytesValue(bytes)
	C.rc_set_result(results, true, value)
}

func secretSign(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	key, ok := selfSecret(ctx, args)
	if !ok || nargs < 2 {
		setStringResult(results, false, "unknown secret key")
		return
	}
	if key.algorithm != "ed25519" {
		setStringResult(results, false, "key does not support signing")
		return
	}
	signature := ed25519.Sign(key.ed, valueBytes(argValue(args, 1)))
	payload := newValue()
	setBytesValue(payload, signature)
	C.rc_set_result(results, true, payload)
}

func secretAgree(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	key, ok := selfSecret(ctx, args)
	if !ok || nargs < 2 {
		setStringResult(results, false, "unknown secret key")
		return
	}
	if key.algorithm != "x25519" {
		setStringResult(results, false, "key does not support agreement")
		return
	}
	peerBytes := valueBytes(argValue(args, 1))
	peer, err := ecdh.X25519().NewPublicKey(peerBytes)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	shared, err := key.x.ECDH(peer)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	setKeyResult(ctx, results, &sharedSecretResource{bytes: shared}, resourceSharedSecret)
}

func keysDerive(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 4 {
		setStringResult(results, false, "invalid derive request")
		return
	}
	first, ok1 := selfShared(ctx, args, 0)
	second, ok2 := selfShared(ctx, args, 1)
	if !ok1 || !ok2 {
		setStringResult(results, false, "unknown shared secret")
		return
	}
	salt := valueBytes(argValue(args, 2))
	info := valueBytes(argValue(args, 3))
	ikm := append(append([]byte{}, first.bytes...), second.bytes...)
	setKeyResult(ctx, results, &sessionKeyResource{key: hkdf(ikm, salt, info, 32)}, resourceSessionKey)
}
