//go:build cgo && !windows

package capi

/*
#include <stddef.h>
#include <stdint.h>
typedef struct wasmtime_component_val wasmtime_component_val_t;
extern void* goRCLog(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCCaller(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void rc_go_set_none_option(wasmtime_component_val_t*, size_t);
extern void* goRCStorageRevision(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCStorageGet(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCStorageScan(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCStorageCommit(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCResourceDestructor(void*, void*, uint32_t);
extern void* goRCProcessCreateGroup(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCProcessGroupSpawn(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCProcessGroupSignal(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCProcessGroupResize(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCProcessGroupClose(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCProcessChildState(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCProcessChildPollExit(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCProcessStreamRead(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCProcessStreamWrite(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCProcessStreamCloseWrite(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCProcessStreamClose(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCEnvironmentPlatform(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCEnvironmentSnapshot(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCEnvironmentFind(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCClockNow(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCClockArm(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCTimerWait(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCTimerCancel(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCFilesystemRead(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCFilesystemWrite(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCFilesystemList(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCFilesystemMkdir(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCFilesystemRemove(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCFilesystemRename(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCFilesystemCwd(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCPolicyStart(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCPolicyAccess(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCPolicyResize(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCPolicySignal(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCDiagnosticsSubmit(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
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
*/
import "C"

import "unsafe"

//export goRCLog
func goRCLog(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	return nil
}

//export goRCCaller
func goRCCaller(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	C.rc_go_set_none_option(results, nresults)
	return nil
}

//export goRCStorageRevision
func goRCStorageRevision(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	storageRevision(data, args, nargs, results, nresults)
	return nil
}

//export goRCStorageGet
func goRCStorageGet(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	storageGet(data, args, nargs, results, nresults)
	return nil
}

//export goRCStorageScan
func goRCStorageScan(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	storageScan(data, args, nargs, results, nresults)
	return nil
}

//export goRCStorageCommit
func goRCStorageCommit(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	storageCommit(data, args, nargs, results, nresults)
	return nil
}

//export goRCResourceDestructor
func goRCResourceDestructor(data unsafe.Pointer, context unsafe.Pointer, rep C.uint32_t) unsafe.Pointer {
	releaseResource(uint32(rep))
	return nil
}

//export goRCProcessCreateGroup
func goRCProcessCreateGroup(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	processCreateGroup(context, args, nargs, results, nresults)
	return nil
}

//export goRCProcessGroupSpawn
func goRCProcessGroupSpawn(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	processGroupSpawn(context, args, nargs, results, nresults)
	return nil
}

//export goRCProcessGroupSignal
func goRCProcessGroupSignal(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	processGroupSignal(context, args, nargs, results, nresults)
	return nil
}

//export goRCProcessGroupResize
func goRCProcessGroupResize(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	processGroupResize(context, args, nargs, results, nresults)
	return nil
}

//export goRCProcessGroupClose
func goRCProcessGroupClose(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	processGroupClose(context, args, nargs, results, nresults)
	return nil
}

//export goRCProcessChildState
func goRCProcessChildState(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	processChildState(context, args, nargs, results, nresults)
	return nil
}

//export goRCProcessChildPollExit
func goRCProcessChildPollExit(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	processChildPollExit(context, args, nargs, results, nresults)
	return nil
}

//export goRCProcessStreamRead
func goRCProcessStreamRead(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	processStreamRead(context, args, nargs, results, nresults)
	return nil
}

//export goRCProcessStreamWrite
func goRCProcessStreamWrite(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	processStreamWrite(context, args, nargs, results, nresults)
	return nil
}

//export goRCProcessStreamCloseWrite
func goRCProcessStreamCloseWrite(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	processStreamCloseWrite(context, args, nargs, results, nresults)
	return nil
}

//export goRCProcessStreamClose
func goRCProcessStreamClose(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	processStreamClose(context, args, nargs, results, nresults)
	return nil
}

//export goRCEnvironmentPlatform
func goRCEnvironmentPlatform(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	environmentPlatform(context, args, nargs, results, nresults)
	return nil
}

//export goRCEnvironmentSnapshot
func goRCEnvironmentSnapshot(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	environmentSnapshot(context, args, nargs, results, nresults)
	return nil
}

//export goRCEnvironmentFind
func goRCEnvironmentFind(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	environmentFind(context, args, nargs, results, nresults)
	return nil
}

//export goRCClockNow
func goRCClockNow(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	clockNow(context, args, nargs, results, nresults)
	return nil
}

//export goRCClockArm
func goRCClockArm(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	clockArm(context, args, nargs, results, nresults)
	return nil
}

//export goRCTimerWait
func goRCTimerWait(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	timerWait(context, args, nargs, results, nresults)
	return nil
}

//export goRCTimerCancel
func goRCTimerCancel(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	timerCancel(context, args, nargs, results, nresults)
	return nil
}

//export goRCFilesystemRead
func goRCFilesystemRead(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	filesystemRead(context, args, nargs, results, nresults)
	return nil
}

//export goRCFilesystemWrite
func goRCFilesystemWrite(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	filesystemWrite(context, args, nargs, results, nresults)
	return nil
}

//export goRCFilesystemList
func goRCFilesystemList(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	filesystemList(context, args, nargs, results, nresults)
	return nil
}

//export goRCFilesystemMkdir
func goRCFilesystemMkdir(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	filesystemMkdir(context, args, nargs, results, nresults)
	return nil
}

//export goRCFilesystemRemove
func goRCFilesystemRemove(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	filesystemRemove(context, args, nargs, results, nresults)
	return nil
}

//export goRCFilesystemRename
func goRCFilesystemRename(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	filesystemRename(context, args, nargs, results, nresults)
	return nil
}

//export goRCFilesystemCwd
func goRCFilesystemCwd(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	filesystemCwd(context, args, nargs, results, nresults)
	return nil
}

//export goRCPolicyStart
func goRCPolicyStart(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	policyAuthorizeStart(context, args, nargs, results, nresults)
	return nil
}

//export goRCPolicyAccess
func goRCPolicyAccess(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	policyAuthorizeAccess(context, args, nargs, results, nresults)
	return nil
}

//export goRCPolicyResize
func goRCPolicyResize(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	policyNormalizeResize(context, args, nargs, results, nresults)
	return nil
}

//export goRCPolicySignal
func goRCPolicySignal(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	policyAuthorizeSignal(context, args, nargs, results, nresults)
	return nil
}

//export goRCDiagnosticsSubmit
func goRCDiagnosticsSubmit(data unsafe.Pointer, context unsafe.Pointer, functionType unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) unsafe.Pointer {
	diagnosticsSubmit(context, args, nargs, results, nresults)
	return nil
}
