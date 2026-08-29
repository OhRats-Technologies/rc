use super::{ActiveInstance, ServiceRegistry};
use crate::{
    bindings::Plugin,
    descriptor::{ValidatedDescriptor, ValidatedRequirement},
    host::{self, HostState},
};
use std::collections::BTreeMap;
use wasmtime::{
    Store,
    component::{Func, Instance, Linker},
};

pub fn linker(
    engine: &wasmtime::Engine,
    component: &wasmtime::component::Component,
    descriptor: &ValidatedDescriptor,
    registry: &ServiceRegistry,
    metadata_only: bool,
) -> anyhow::Result<Linker<HostState>> {
    let mut linker = Linker::new(engine);
    host::add_base_imports(&mut linker)?;
    if metadata_only {
        linker.define_unknown_imports_as_traps(component)?;
        return Ok(linker);
    }
    for requirement in &descriptor.requires {
        link_requirement(&mut linker, requirement, registry)?;
    }
    Ok(linker)
}

fn link_requirement(
    linker: &mut Linker<HostState>,
    requirement: &ValidatedRequirement,
    registry: &ServiceRegistry,
) -> anyhow::Result<()> {
    let mut instance = linker.instance(&requirement.interface)?;
    for function in &requirement.functions {
        let registry = registry.clone();
        let service = requirement.name.clone();
        let version = requirement.version.clone();
        let selection = requirement.selection;
        let function_name = function.clone();
        instance.func_new(function, move |store, _ty, params, results| {
            let caller = store.data().plugin_id().to_owned();
            registry.call(
                &caller,
                &service,
                &version,
                selection,
                &function_name,
                params,
                results,
            )
        })?;
    }
    Ok(())
}

pub fn active_instance(
    component: &wasmtime::component::Component,
    instance: &Instance,
    mut store: Store<HostState>,
    bindings: Plugin,
    descriptor: &ValidatedDescriptor,
) -> anyhow::Result<ActiveInstance> {
    let exports = exported_functions(component, instance, &mut store, descriptor)?;
    Ok(ActiveInstance {
        store,
        bindings,
        exports,
    })
}

fn exported_functions(
    component: &wasmtime::component::Component,
    instance: &Instance,
    store: &mut Store<HostState>,
    descriptor: &ValidatedDescriptor,
) -> anyhow::Result<BTreeMap<(String, String), Func>> {
    let mut exports = BTreeMap::new();
    for service in &descriptor.provides {
        let parent = component
            .get_export_index(None, service.interface.as_str())
            .ok_or_else(|| anyhow::anyhow!("missing service export {}", service.interface))?;
        for function in &service.functions {
            let index = component
                .get_export_index(Some(&parent), function.as_str())
                .ok_or_else(|| {
                    anyhow::anyhow!("missing service function {}#{function}", service.interface)
                })?;
            let func = instance.get_func(&mut *store, index).ok_or_else(|| {
                anyhow::anyhow!(
                    "service export {}#{function} is not a function",
                    service.interface
                )
            })?;
            exports.insert((service.name.clone(), function.clone()), func);
        }
    }
    Ok(exports)
}
