#include "capi.h"

extern void* goRCLog(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCCaller(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);

extern void* goRCStorageRevision(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCStorageGet(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCStorageScan(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCStorageCommit(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);

rc_component_func_callback_t rc_storage_revision_callback(void) { return (rc_component_func_callback_t)goRCStorageRevision; }
rc_component_func_callback_t rc_storage_get_callback(void) { return (rc_component_func_callback_t)goRCStorageGet; }
rc_component_func_callback_t rc_storage_scan_callback(void) { return (rc_component_func_callback_t)goRCStorageScan; }
rc_component_func_callback_t rc_storage_commit_callback(void) { return (rc_component_func_callback_t)goRCStorageCommit; }

extern void* goRCResourceDestructor(void*, void*, uint32_t);
rc_resource_destructor_t rc_resource_destructor(void) { return (rc_resource_destructor_t)goRCResourceDestructor; }

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
rc_component_func_callback_t rc_process_create_group_callback(void) { return (rc_component_func_callback_t)goRCProcessCreateGroup; }
rc_component_func_callback_t rc_process_group_spawn_callback(void) { return (rc_component_func_callback_t)goRCProcessGroupSpawn; }
rc_component_func_callback_t rc_process_group_signal_callback(void) { return (rc_component_func_callback_t)goRCProcessGroupSignal; }
rc_component_func_callback_t rc_process_group_resize_callback(void) { return (rc_component_func_callback_t)goRCProcessGroupResize; }
rc_component_func_callback_t rc_process_group_close_callback(void) { return (rc_component_func_callback_t)goRCProcessGroupClose; }
rc_component_func_callback_t rc_process_child_state_callback(void) { return (rc_component_func_callback_t)goRCProcessChildState; }
rc_component_func_callback_t rc_process_child_poll_exit_callback(void) { return (rc_component_func_callback_t)goRCProcessChildPollExit; }
rc_component_func_callback_t rc_process_stream_read_callback(void) { return (rc_component_func_callback_t)goRCProcessStreamRead; }
rc_component_func_callback_t rc_process_stream_write_callback(void) { return (rc_component_func_callback_t)goRCProcessStreamWrite; }
rc_component_func_callback_t rc_process_stream_close_write_callback(void) { return (rc_component_func_callback_t)goRCProcessStreamCloseWrite; }
rc_component_func_callback_t rc_process_stream_close_callback(void) { return (rc_component_func_callback_t)goRCProcessStreamClose; }

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
rc_component_func_callback_t rc_environment_platform_callback(void) { return (rc_component_func_callback_t)goRCEnvironmentPlatform; }
rc_component_func_callback_t rc_environment_snapshot_callback(void) { return (rc_component_func_callback_t)goRCEnvironmentSnapshot; }
rc_component_func_callback_t rc_environment_find_callback(void) { return (rc_component_func_callback_t)goRCEnvironmentFind; }
rc_component_func_callback_t rc_clock_now_callback(void) { return (rc_component_func_callback_t)goRCClockNow; }
rc_component_func_callback_t rc_clock_arm_callback(void) { return (rc_component_func_callback_t)goRCClockArm; }
rc_component_func_callback_t rc_timer_wait_callback(void) { return (rc_component_func_callback_t)goRCTimerWait; }
rc_component_func_callback_t rc_timer_cancel_callback(void) { return (rc_component_func_callback_t)goRCTimerCancel; }
rc_component_func_callback_t rc_filesystem_read_callback(void) { return (rc_component_func_callback_t)goRCFilesystemRead; }
rc_component_func_callback_t rc_filesystem_write_callback(void) { return (rc_component_func_callback_t)goRCFilesystemWrite; }
rc_component_func_callback_t rc_filesystem_list_callback(void) { return (rc_component_func_callback_t)goRCFilesystemList; }
rc_component_func_callback_t rc_filesystem_mkdir_callback(void) { return (rc_component_func_callback_t)goRCFilesystemMkdir; }
rc_component_func_callback_t rc_filesystem_remove_callback(void) { return (rc_component_func_callback_t)goRCFilesystemRemove; }
rc_component_func_callback_t rc_filesystem_rename_callback(void) { return (rc_component_func_callback_t)goRCFilesystemRename; }
rc_component_func_callback_t rc_filesystem_cwd_callback(void) { return (rc_component_func_callback_t)goRCFilesystemCwd; }

extern void* goRCPolicyStart(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCPolicyAccess(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCPolicyResize(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCPolicySignal(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCDiagnosticsSubmit(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
rc_component_func_callback_t rc_policy_start_callback(void) { return (rc_component_func_callback_t)goRCPolicyStart; }
rc_component_func_callback_t rc_policy_access_callback(void) { return (rc_component_func_callback_t)goRCPolicyAccess; }
rc_component_func_callback_t rc_policy_resize_callback(void) { return (rc_component_func_callback_t)goRCPolicyResize; }
rc_component_func_callback_t rc_policy_signal_callback(void) { return (rc_component_func_callback_t)goRCPolicySignal; }
rc_component_func_callback_t rc_diagnostics_submit_callback(void) { return (rc_component_func_callback_t)goRCDiagnosticsSubmit; }

extern void* goRCHttpSend(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
rc_component_func_callback_t rc_http_send_callback(void) { return (rc_component_func_callback_t)goRCHttpSend; }

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
rc_component_func_callback_t rc_keys_ensure_callback(void) { return (rc_component_func_callback_t)goRCKeysEnsure; }
rc_component_func_callback_t rc_keys_open_callback(void) { return (rc_component_func_callback_t)goRCKeysOpen; }
rc_component_func_callback_t rc_keys_generate_callback(void) { return (rc_component_func_callback_t)goRCKeysGenerate; }
rc_component_func_callback_t rc_keys_remove_callback(void) { return (rc_component_func_callback_t)goRCKeysRemove; }
rc_component_func_callback_t rc_keys_derive_callback(void) { return (rc_component_func_callback_t)goRCKeysDerive; }
rc_component_func_callback_t rc_secret_public_callback(void) { return (rc_component_func_callback_t)goRCSecretPublic; }
rc_component_func_callback_t rc_secret_sign_callback(void) { return (rc_component_func_callback_t)goRCSecretSign; }
rc_component_func_callback_t rc_secret_agree_callback(void) { return (rc_component_func_callback_t)goRCSecretAgree; }
rc_component_func_callback_t rc_session_encrypt_callback(void) { return (rc_component_func_callback_t)goRCSessionEncrypt; }
rc_component_func_callback_t rc_session_decrypt_callback(void) { return (rc_component_func_callback_t)goRCSessionDecrypt; }

extern void* goRCNetConnect(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCNetWrite(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCNetRead(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCNetCloseWrite(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
extern void* goRCNetClose(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);
rc_component_func_callback_t rc_net_connect_callback(void) { return (rc_component_func_callback_t)goRCNetConnect; }
rc_component_func_callback_t rc_net_write_callback(void) { return (rc_component_func_callback_t)goRCNetWrite; }
rc_component_func_callback_t rc_net_read_callback(void) { return (rc_component_func_callback_t)goRCNetRead; }
rc_component_func_callback_t rc_net_close_write_callback(void) { return (rc_component_func_callback_t)goRCNetCloseWrite; }
rc_component_func_callback_t rc_net_close_callback(void) { return (rc_component_func_callback_t)goRCNetClose; }

rc_component_func_callback_t rc_log_callback(void) {
  return (rc_component_func_callback_t)goRCLog;
}

rc_component_func_callback_t rc_caller_callback(void) {
  return (rc_component_func_callback_t)goRCCaller;
}

void rc_go_set_none_option(wasmtime_component_val_t* results, size_t len) {
  rc_set_none_option_result(results, len);
}
