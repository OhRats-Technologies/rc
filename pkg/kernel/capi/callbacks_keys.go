//go:build cgo && !windows

package capi

/*
#include <stddef.h>
#include <stdint.h>
typedef struct wasmtime_component_val wasmtime_component_val_t;
extern void* goRCHttpSend(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCKeysEnsure(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCKeysOpen(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCKeysGenerate(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCKeysRemove(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCKeysDerive(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCSecretPublic(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCSecretSign(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCSecretAgree(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCSessionEncrypt(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCSessionDecrypt(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCNetConnect(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCNetWrite(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCNetRead(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCNetCloseWrite(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCNetClose(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
*/
import "C"

import "unsafe"

//export goRCHttpSend
func goRCHttpSend(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	httpSend(context, args, nargs, results, nresults)
	return nil
}

//export goRCKeysEnsure
func goRCKeysEnsure(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	keysEnsure(context, args, nargs, results, nresults)
	return nil
}

//export goRCKeysOpen
func goRCKeysOpen(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	keysOpen(context, args, nargs, results, nresults)
	return nil
}

//export goRCKeysGenerate
func goRCKeysGenerate(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	keysGenerate(context, args, nargs, results, nresults)
	return nil
}

//export goRCKeysRemove
func goRCKeysRemove(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	keysRemove(context, args, nargs, results, nresults)
	return nil
}

//export goRCKeysDerive
func goRCKeysDerive(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	keysDerive(context, args, nargs, results, nresults)
	return nil
}

//export goRCSecretPublic
func goRCSecretPublic(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	secretPublicKey(context, args, nargs, results, nresults)
	return nil
}

//export goRCSecretSign
func goRCSecretSign(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	secretSign(context, args, nargs, results, nresults)
	return nil
}

//export goRCSecretAgree
func goRCSecretAgree(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	secretAgree(context, args, nargs, results, nresults)
	return nil
}

//export goRCSessionEncrypt
func goRCSessionEncrypt(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	sessionEncrypt(context, args, nargs, results, nresults)
	return nil
}

//export goRCSessionDecrypt
func goRCSessionDecrypt(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	sessionDecrypt(context, args, nargs, results, nresults)
	return nil
}

//export goRCNetConnect
func goRCNetConnect(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	netConnect(context, data, args, nargs, results, nresults)
	return nil
}

//export goRCNetWrite
func goRCNetWrite(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	netWrite(context, args, nargs, results, nresults)
	return nil
}

//export goRCNetRead
func goRCNetRead(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	netRead(context, args, nargs, results, nresults)
	return nil
}

//export goRCNetCloseWrite
func goRCNetCloseWrite(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	netCloseWrite(context, args, nargs, results, nresults)
	return nil
}

//export goRCNetClose
func goRCNetClose(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	netClose(context, args, nargs, results, nresults)
	return nil
}
