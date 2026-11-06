#ifndef RC_KERNEL_CAPI_H
#define RC_KERNEL_CAPI_H

#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>
#include <stdlib.h>

typedef struct { size_t size; uint8_t *data; } wasm_name_t;
typedef struct wasmtime_component_val wasmtime_component_val_t;
typedef struct wasmtime_component_valrecord_entry wasmtime_component_valrecord_entry_t;
typedef struct { size_t size; wasmtime_component_valrecord_entry_t *data; } wasmtime_component_valrecord_t;
typedef struct { size_t size; wasmtime_component_val_t *data; } wasmtime_component_vallist_t;
typedef struct { wasm_name_t discriminant; wasmtime_component_val_t *val; } wasmtime_component_valvariant_t;
typedef struct { bool is_ok; wasmtime_component_val_t *val; } wasmtime_component_valresult_t;
typedef struct { size_t size; wasm_name_t *data; } wasmtime_component_valflags_t;
typedef struct wasmtime_component_valmap_entry wasmtime_component_valmap_entry_t;
typedef struct { size_t size; wasmtime_component_valmap_entry_t *data; } wasmtime_component_valmap_t;
typedef struct wasmtime_component_resource_any wasmtime_component_resource_any_t;

typedef union {
  bool boolean;
  int8_t s8;
  uint8_t u8;
  int16_t s16;
  uint16_t u16;
  int32_t s32;
  uint32_t u32;
  int64_t s64;
  uint64_t u64;
  float f32;
  double f64;
  uint32_t character;
  wasm_name_t string;
  wasmtime_component_vallist_t list;
  wasmtime_component_valrecord_t record;
  wasmtime_component_vallist_t tuple;
  wasmtime_component_valvariant_t variant;
  wasm_name_t enumeration;
  wasmtime_component_val_t *option;
  wasmtime_component_valresult_t result;
  wasmtime_component_valflags_t flags;
  wasmtime_component_valmap_t map;
  wasmtime_component_resource_any_t *resource;
} wasmtime_component_valunion_t;

struct wasmtime_component_val {
  uint8_t kind;
  wasmtime_component_valunion_t of;
};

struct wasmtime_component_valrecord_entry {
  wasm_name_t name;
  wasmtime_component_val_t val;
};

struct wasmtime_component_valmap_entry {
  wasmtime_component_val_t key;
  wasmtime_component_val_t value;
};

typedef struct {
  uint64_t store_id;
  uint32_t private1;
  uint32_t private2;
  void *private3;
} wasmtime_component_func_t;

typedef struct {
  uint64_t store_id;
  uint32_t private1;
} wasmtime_component_instance_t;

typedef void* (*rc_resource_destructor_t)(void*, void*, uint32_t);
typedef void* (*rc_component_func_callback_t)(void*, void*, void*, wasmtime_component_val_t*, size_t, wasmtime_component_val_t*, size_t);

extern void* wasmtime_store_context(void* store);
extern void* wasmtime_component_linker_new(void* engine);
extern void* wasmtime_component_resource_type_new_host(uint32_t ty);
extern void wasmtime_component_resource_type_delete(void* ty);
extern void* wasmtime_component_resource_host_new(bool owned, uint32_t rep, uint32_t ty);
extern void wasmtime_component_resource_host_delete(void* resource);
extern uint32_t wasmtime_component_resource_host_rep(void* resource);
extern uint32_t wasmtime_component_resource_host_type(void* resource);
extern void* wasmtime_component_resource_host_to_any(void* context, void* resource, void** out);
extern void* wasmtime_component_resource_any_to_host(void* context, void* resource, void** out);
extern void wasmtime_component_resource_any_delete(void* resource);
extern void* wasmtime_component_resource_any_clone(void* resource);
extern void* wasmtime_component_linker_instance_add_resource(void* instance, const char* name, size_t len, void* ty, rc_resource_destructor_t destructor, void* data, void* finalizer);
extern void* wasmtime_component_linker_root(void* linker);
extern void* wasmtime_component_linker_instance_add_instance(void* instance, const char* name, size_t len, void** out);
extern void* wasmtime_component_linker_instance_add_func(void* instance, const char* name, size_t len, rc_component_func_callback_t callback, void* data, void* finalizer);
extern void* wasmtime_component_linker_define_unknown_imports_as_traps(void* linker, void* component);
extern void* wasmtime_component_linker_add_wasip2(void* linker);
extern void* wasmtime_component_linker_instantiate(void* linker, void* context, void* component, void* instance_out);
extern bool wasmtime_component_instance_get_func(void* instance, void* context, void* export_index, void* func_out);
extern void* wasmtime_component_func_call(void* func, void* context, const wasmtime_component_val_t* args, size_t nargs, wasmtime_component_val_t* results, size_t nresults);
extern void wasmtime_component_linker_instance_delete(void* instance);
extern void wasmtime_component_linker_delete(void* linker);
extern void wasmtime_component_val_delete(wasmtime_component_val_t* value);
extern void wasmtime_component_val_clone(const wasmtime_component_val_t* source, wasmtime_component_val_t* destination);
extern void wasmtime_error_message(void* error, wasm_name_t* out);
extern void wasmtime_error_delete(void* error);
typedef struct wasi_config_t wasi_config_t;
extern wasi_config_t* wasi_config_new(void);
extern void wasi_config_inherit_stdout(wasi_config_t* config);
extern void wasi_config_inherit_stderr(wasi_config_t* config);
extern void* wasmtime_context_set_wasi(void* context, wasi_config_t* config);
extern void wasm_byte_vec_delete(wasm_name_t* value);

rc_component_func_callback_t rc_log_callback(void);
rc_component_func_callback_t rc_storage_revision_callback(void);
rc_component_func_callback_t rc_storage_get_callback(void);
rc_component_func_callback_t rc_storage_scan_callback(void);
rc_component_func_callback_t rc_storage_commit_callback(void);
rc_resource_destructor_t rc_resource_destructor(void);
rc_component_func_callback_t rc_process_create_group_callback(void);
rc_component_func_callback_t rc_process_group_spawn_callback(void);
rc_component_func_callback_t rc_process_group_signal_callback(void);
rc_component_func_callback_t rc_process_group_resize_callback(void);
rc_component_func_callback_t rc_process_group_close_callback(void);
rc_component_func_callback_t rc_process_child_state_callback(void);
rc_component_func_callback_t rc_process_child_poll_exit_callback(void);
rc_component_func_callback_t rc_process_stream_read_callback(void);
rc_component_func_callback_t rc_process_stream_write_callback(void);
rc_component_func_callback_t rc_process_stream_close_write_callback(void);
rc_component_func_callback_t rc_process_stream_close_callback(void);
rc_component_func_callback_t rc_environment_platform_callback(void);
rc_component_func_callback_t rc_environment_snapshot_callback(void);
rc_component_func_callback_t rc_environment_find_callback(void);
rc_component_func_callback_t rc_clock_now_callback(void);
rc_component_func_callback_t rc_clock_arm_callback(void);
rc_component_func_callback_t rc_timer_wait_callback(void);
rc_component_func_callback_t rc_timer_cancel_callback(void);
rc_component_func_callback_t rc_filesystem_read_callback(void);
rc_component_func_callback_t rc_filesystem_write_callback(void);
rc_component_func_callback_t rc_filesystem_list_callback(void);
rc_component_func_callback_t rc_filesystem_mkdir_callback(void);
rc_component_func_callback_t rc_filesystem_remove_callback(void);
rc_component_func_callback_t rc_filesystem_rename_callback(void);
rc_component_func_callback_t rc_filesystem_cwd_callback(void);
rc_component_func_callback_t rc_policy_start_callback(void);
rc_component_func_callback_t rc_policy_access_callback(void);
rc_component_func_callback_t rc_policy_resize_callback(void);
rc_component_func_callback_t rc_policy_signal_callback(void);
rc_component_func_callback_t rc_diagnostics_submit_callback(void);
rc_component_func_callback_t rc_http_send_callback(void);
rc_component_func_callback_t rc_keys_ensure_callback(void);
rc_component_func_callback_t rc_keys_open_callback(void);
rc_component_func_callback_t rc_keys_generate_callback(void);
rc_component_func_callback_t rc_keys_remove_callback(void);
rc_component_func_callback_t rc_keys_derive_callback(void);
rc_component_func_callback_t rc_secret_public_callback(void);
rc_component_func_callback_t rc_secret_sign_callback(void);
rc_component_func_callback_t rc_secret_agree_callback(void);
rc_component_func_callback_t rc_session_encrypt_callback(void);
rc_component_func_callback_t rc_session_decrypt_callback(void);
rc_component_func_callback_t rc_net_connect_callback(void);
rc_component_func_callback_t rc_net_write_callback(void);
rc_component_func_callback_t rc_net_read_callback(void);
rc_component_func_callback_t rc_net_close_write_callback(void);
rc_component_func_callback_t rc_net_close_callback(void);
rc_component_func_callback_t rc_caller_callback(void);


static inline uint8_t rc_val_kind(const wasmtime_component_val_t* v) { return v->kind; }
static inline const uint8_t* rc_string_data(const wasmtime_component_val_t* v) { return v->of.string.data; }
static inline size_t rc_string_len(const wasmtime_component_val_t* v) { return v->of.string.size; }
static inline size_t rc_list_len(const wasmtime_component_val_t* v) { return v->of.list.size; }
static inline wasmtime_component_val_t* rc_list_get(const wasmtime_component_val_t* v, size_t i) { return &v->of.list.data[i]; }
static inline size_t rc_record_len(const wasmtime_component_val_t* v) { return v->of.record.size; }
static inline const uint8_t* rc_record_key(const wasmtime_component_val_t* v, size_t i) { return v->of.record.data[i].name.data; }
static inline size_t rc_record_key_len(const wasmtime_component_val_t* v, size_t i) { return v->of.record.data[i].name.size; }
static inline wasmtime_component_val_t* rc_record_val(const wasmtime_component_val_t* v, size_t i) { return &v->of.record.data[i].val; }
static inline bool rc_result_ok(const wasmtime_component_val_t* v) { return v->of.result.is_ok; }
static inline wasmtime_component_val_t* rc_result_val(const wasmtime_component_val_t* v) { return v->of.result.val; }
static inline uint32_t rc_u32(const wasmtime_component_val_t* v) { return v->of.u32; }
static inline void rc_set_resource(wasmtime_component_val_t* v, void* resource) { v->kind = 21; v->of.resource = resource; }
static inline void* rc_resource_any(const wasmtime_component_val_t* v) { return v->of.resource; }
static inline void* rc_clone_resource(const wasmtime_component_val_t* v) { return wasmtime_component_resource_any_clone(v->of.resource); }
static inline void rc_clone_val(wasmtime_component_val_t* target, const wasmtime_component_val_t* source) { wasmtime_component_val_clone(source, target); }
static inline void rc_alloc_tuple(wasmtime_component_val_t* v, size_t n) { v->kind = 15; v->of.tuple.size = n; v->of.tuple.data = n ? (wasmtime_component_val_t*)calloc(n, sizeof(wasmtime_component_val_t)) : NULL; }
static inline wasmtime_component_val_t* rc_tuple_item(wasmtime_component_val_t* v, size_t i) { return &v->of.tuple.data[i]; }
static inline void rc_set_enum(wasmtime_component_val_t* v, const char* name, size_t len) { v->kind = 17; v->of.enumeration.data = (uint8_t*)name; v->of.enumeration.size = len; }
static inline void rc_set_bool(wasmtime_component_val_t* v, bool value) { v->kind = 0; v->of.boolean = value; }
static inline bool rc_bool(const wasmtime_component_val_t* v) { return v->of.boolean; }
static inline void rc_set_string(wasmtime_component_val_t* v, uint8_t* data, size_t len) { v->kind = 12; v->of.string.data = data; v->of.string.size = len; }
static inline void rc_set_list(wasmtime_component_val_t* v, wasmtime_component_val_t* data, size_t len) { v->kind = 13; v->of.list.data = data; v->of.list.size = len; }
static inline void rc_set_none_option(wasmtime_component_val_t* v) { v->kind = 18; v->of.option = NULL; }
static inline void rc_set_some_option(wasmtime_component_val_t* v, wasmtime_component_val_t* payload) { v->kind = 18; v->of.option = payload; }
static inline wasmtime_component_val_t* rc_option_get(const wasmtime_component_val_t* v) { return v->of.option; }
static inline void rc_set_u8(wasmtime_component_val_t* v, uint8_t value) { v->kind = 2; v->of.u8 = value; }
static inline uint8_t rc_u8(const wasmtime_component_val_t* v) { return v->of.u8; }
static inline void rc_set_u16(wasmtime_component_val_t* v, uint16_t value) { v->kind = 4; v->of.u16 = value; }
static inline uint16_t rc_u16(const wasmtime_component_val_t* v) { return v->of.u16; }
static inline void rc_set_u32(wasmtime_component_val_t* v, uint32_t value) { v->kind = 6; v->of.u32 = value; }
static inline void rc_set_u64(wasmtime_component_val_t* v, uint64_t value) { v->kind = 8; v->of.u64 = value; }
static inline uint64_t rc_u64(const wasmtime_component_val_t* v) { return v->of.u64; }
static inline void rc_set_result(wasmtime_component_val_t* v, bool ok, wasmtime_component_val_t* payload) { v->kind = 19; v->of.result.is_ok = ok; v->of.result.val = payload; }
static inline void rc_set_variant(wasmtime_component_val_t* v, const char* name, size_t len, wasmtime_component_val_t* payload) { v->kind = 16; v->of.variant.discriminant.data = (uint8_t*)name; v->of.variant.discriminant.size = len; v->of.variant.val = payload; }
static inline const uint8_t* rc_variant_name(const wasmtime_component_val_t* v) { return v->of.variant.discriminant.data; }
static inline size_t rc_variant_name_len(const wasmtime_component_val_t* v) { return v->of.variant.discriminant.size; }
static inline wasmtime_component_val_t* rc_variant_val(const wasmtime_component_val_t* v) { return v->of.variant.val; }
static inline void rc_alloc_list(wasmtime_component_val_t* v, size_t n) { v->kind = 13; v->of.list.size = n; v->of.list.data = n ? (wasmtime_component_val_t*)calloc(n, sizeof(wasmtime_component_val_t)) : NULL; }
static inline void rc_of_list(wasmtime_component_val_t* v, size_t n) { v->of.list.size = n; }
static inline wasmtime_component_val_t* rc_list_item(wasmtime_component_val_t* v, size_t i) { return &v->of.list.data[i]; }
static inline void rc_alloc_record(wasmtime_component_val_t* v, size_t n) { v->kind = 14; v->of.record.size = n; v->of.record.data = n ? (wasmtime_component_valrecord_entry_t*)calloc(n, sizeof(wasmtime_component_valrecord_entry_t)) : NULL; }
static inline wasmtime_component_val_t* rc_record_item(wasmtime_component_val_t* v, size_t i, const char* key, size_t len) { wasmtime_component_valrecord_entry_t* entry = &v->of.record.data[i]; entry->name.data = (uint8_t*)key; entry->name.size = len; return &entry->val; }
static inline void rc_set_none_option_result(wasmtime_component_val_t* results, size_t len) { if (len) rc_set_none_option(&results[0]); }

#endif
