//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import "unsafe"

func linkExtraHosts(root unsafe.Pointer, netSlot unsafe.Pointer) error {
	var err error
	var process unsafe.Pointer
	process, err = addInstance(root, "ohrats:rc-process/process-host@0.3.0")
	if err == nil {
		for name, ty := range map[string]uint32{
			"execution-group": resourceGroup,
			"child":           resourceChild,
			"byte-stream":     resourceStream,
		} {
			if err = addResource(process, name, ty); err != nil {
				break
			}
		}
	}
	if err == nil {
		for name, callback := range map[string]C.rc_component_func_callback_t{
			"create-group":                    C.rc_process_create_group_callback(),
			"[method]execution-group.spawn":   C.rc_process_group_spawn_callback(),
			"[method]execution-group.signal":  C.rc_process_group_signal_callback(),
			"[method]execution-group.resize":  C.rc_process_group_resize_callback(),
			"[method]execution-group.close":   C.rc_process_group_close_callback(),
			"[method]child.state":             C.rc_process_child_state_callback(),
			"[method]child.poll-exit":         C.rc_process_child_poll_exit_callback(),
			"[method]byte-stream.read":        C.rc_process_stream_read_callback(),
			"[method]byte-stream.write":       C.rc_process_stream_write_callback(),
			"[method]byte-stream.close-write": C.rc_process_stream_close_write_callback(),
			"[method]byte-stream.close":       C.rc_process_stream_close_callback(),
		} {
			if err = addFunc(process, name, callback); err != nil {
				break
			}
		}
	}
	if process != nil {
		C.wasmtime_component_linker_instance_delete(process)
	}
	if err == nil {
		var environment unsafe.Pointer
		environment, err = addInstance(root, "ohrats:rc-process/environment-host@0.3.0")
		if err == nil {
			for name, callback := range map[string]C.rc_component_func_callback_t{
				"host-platform":   C.rc_environment_platform_callback(),
				"snapshot":        C.rc_environment_snapshot_callback(),
				"find-executable": C.rc_environment_find_callback(),
			} {
				if err = addFunc(environment, name, callback); err != nil {
					break
				}
			}
		}
		if environment != nil {
			C.wasmtime_component_linker_instance_delete(environment)
		}
	}
	if err == nil {
		var clock unsafe.Pointer
		clock, err = addInstance(root, "ohrats:rc-process/clock-host@0.3.0")
		if err == nil {
			err = addResource(clock, "timer", resourceTimer)
		}
		if err == nil {
			for name, callback := range map[string]C.rc_component_func_callback_t{
				"now-unix-ms":          C.rc_clock_now_callback(),
				"arm":                  C.rc_clock_arm_callback(),
				"[method]timer.wait":   C.rc_timer_wait_callback(),
				"[method]timer.cancel": C.rc_timer_cancel_callback(),
			} {
				if err = addFunc(clock, name, callback); err != nil {
					break
				}
			}
		}
		if clock != nil {
			C.wasmtime_component_linker_instance_delete(clock)
		}
	}
	if err == nil {
		var policy unsafe.Pointer
		policy, err = addInstance(root, "ohrats:rc-process/policy@0.3.0")
		if err == nil {
			for name, callback := range map[string]C.rc_component_func_callback_t{
				"authorize-start":  C.rc_policy_start_callback(),
				"authorize-access": C.rc_policy_access_callback(),
				"normalize-resize": C.rc_policy_resize_callback(),
				"authorize-signal": C.rc_policy_signal_callback(),
			} {
				if err = addFunc(policy, name, callback); err != nil {
					break
				}
			}
		}
		if policy != nil {
			C.wasmtime_component_linker_instance_delete(policy)
		}
	}
	if err == nil {
		var diagnostics unsafe.Pointer
		diagnostics, err = addInstance(root, "ohrats:rc-diagnostics/reporting@0.1.0")
		if err == nil {
			err = addFunc(diagnostics, "submit", C.rc_diagnostics_submit_callback())
		}
		if diagnostics != nil {
			C.wasmtime_component_linker_instance_delete(diagnostics)
		}
	}
	if err == nil {
		var httpClient unsafe.Pointer
		httpClient, err = addInstance(root, "ohrats:rc-plugin/http-client@0.1.0")
		if err == nil {
			err = addFunc(httpClient, "send", C.rc_http_send_callback())
		}
		if httpClient != nil {
			C.wasmtime_component_linker_instance_delete(httpClient)
		}
	}
	if err == nil {
		var keys unsafe.Pointer
		keys, err = addInstance(root, "ohrats:rc-keys/host-custody@0.1.0")
		if err == nil {
			for name, ty := range map[string]uint32{"secret-key": resourceSecretKey, "session-key": resourceSessionKey, "shared-secret": resourceSharedSecret} {
				if err = addResource(keys, name, ty); err != nil {
					break
				}
			}
		}
		if err == nil {
			for name, callback := range map[string]C.rc_component_func_callback_t{
				"ensure": C.rc_keys_ensure_callback(), "open": C.rc_keys_open_callback(),
				"generate": C.rc_keys_generate_callback(), "remove": C.rc_keys_remove_callback(),
				"derive":                        C.rc_keys_derive_callback(),
				"[method]secret-key.public-key": C.rc_secret_public_callback(),
				"[method]secret-key.sign":       C.rc_secret_sign_callback(),
				"[method]secret-key.agree":      C.rc_secret_agree_callback(),
				"[method]session-key.encrypt":   C.rc_session_encrypt_callback(),
				"[method]session-key.decrypt":   C.rc_session_decrypt_callback(),
			} {
				if err = addFunc(keys, name, callback); err != nil {
					break
				}
			}
		}
		if keys != nil {
			C.wasmtime_component_linker_instance_delete(keys)
		}
	}
	if err == nil {
		var netHost unsafe.Pointer
		netHost, err = addInstance(root, "ohrats:rc-net/byte-stream-host@0.1.0")
		if err == nil {
			err = addResource(netHost, "connection", resourceConnection)
		}
		if err == nil {
			for name, callback := range map[string]C.rc_component_func_callback_t{
				"connect":                        C.rc_net_connect_callback(),
				"[method]connection.write":       C.rc_net_write_callback(),
				"[method]connection.read":        C.rc_net_read_callback(),
				"[method]connection.close-write": C.rc_net_close_write_callback(),
				"[method]connection.close":       C.rc_net_close_callback(),
			} {
				if name == "connect" {
					if err = addFuncData(netHost, name, callback, netSlot); err != nil {
						break
					}
				} else if err = addFuncData(netHost, name, callback, netSlot); err != nil {
					break
				}
			}
		}
		if netHost != nil {
			C.wasmtime_component_linker_instance_delete(netHost)
		}
	}
	if err == nil {
		var filesystem unsafe.Pointer
		filesystem, err = addInstance(root, "ohrats:rc-process/filesystem-host@0.3.0")
		if err == nil {
			for name, callback := range map[string]C.rc_component_func_callback_t{
				"read": C.rc_filesystem_read_callback(), "write": C.rc_filesystem_write_callback(),
				"list-directory": C.rc_filesystem_list_callback(), "create-directory": C.rc_filesystem_mkdir_callback(),
				"remove": C.rc_filesystem_remove_callback(), "rename": C.rc_filesystem_rename_callback(),
				"current-directory": C.rc_filesystem_cwd_callback(),
			} {
				if err = addFunc(filesystem, name, callback); err != nil {
					break
				}
			}
		}
		if filesystem != nil {
			C.wasmtime_component_linker_instance_delete(filesystem)
		}
	}
	return err
}
