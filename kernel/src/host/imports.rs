use super::HostState;

macro_rules! link {
    ($linker:expr, $($module:ident)::+) => {
        $($module)::+::add_to_linker::<HostState, wasmtime::component::HasSelf<HostState>>(
            $linker,
            |state| state,
        )?
    };
}

pub fn add_base_imports(linker: &mut wasmtime::component::Linker<HostState>) -> anyhow::Result<()> {
    wasmtime_wasi::p2::add_to_linker_sync(linker)?;
    link!(linker, crate::bindings::ohrats::rc_plugin::host);
    link!(linker, crate::bindings::ohrats::rc_plugin::call_context);
    link!(linker, crate::bindings::ohrats::rc_plugin::component_store);
    link!(linker, crate::bindings::ohrats::rc_plugin::artifact_cache);
    link!(linker, crate::bindings::ohrats::rc_plugin::state_store);
    link!(linker, crate::bindings::ohrats::rc_plugin::local_files);
    link!(linker, crate::bindings::ohrats::rc_plugin::catalog_store);
    link!(linker, crate::bindings::ohrats::rc_plugin::service_registry);
    link!(linker, crate::bindings::ohrats::rc_plugin::http_client);
    link!(linker, crate::bindings::ohrats::rc_storage::durable_store);
    link!(
        linker,
        crate::bindings::ohrats::rc_artifact_cache::local_storage
    );
    link!(linker, crate::bindings::ohrats::rc_keys::host_custody);
    link!(
        linker,
        crate::bindings::ohrats::rc_process::environment_host
    );
    link!(linker, crate::bindings::ohrats::rc_process::filesystem_host);
    link!(linker, crate::bindings::ohrats::rc_process::clock_host);
    link!(linker, crate::bindings::ohrats::rc_process::process_host);
    link!(linker, crate::bindings::ohrats::rc_updater::artifact_source);
    link!(
        linker,
        crate::bindings::ohrats::rc_updater::native_replacement
    );
    Ok(())
}
