use crate::{
    bindings::{
        Plugin,
        ohrats::rc_plugin::service_registry::{
            Host as ServiceRegistryHost, Provider as ProviderDescription,
        },
    },
    component::LoadedComponent,
    descriptor::{SelectionMode, ValidatedDescriptor, ValidatedRequirement},
    host::{self, HostState},
};
use semver::{Version, VersionReq};
use std::{
    cell::RefCell,
    collections::BTreeMap,
    sync::{Arc, Mutex, RwLock},
};
use wasmtime::{
    Store,
    component::{Func, Instance, Linker, Val},
};

pub type InstanceHandle = Arc<Mutex<ActiveInstance>>;

pub struct ActiveInstance {
    pub store: Store<HostState>,
    pub bindings: Plugin,
    exports: BTreeMap<(String, String), Func>,
}

impl ActiveInstance {
    pub fn deactivate(&mut self) {
        let bindings = &self.bindings;
        let store = &mut self.store;
        let _ = store.set_fuel(host::ACTIVATION_FUEL);
        let _ = bindings.call_deactivate(store);
    }

    pub fn invoke(&mut self, command: &str, args: &[String]) -> anyhow::Result<u32> {
        let bindings = &self.bindings;
        let store = &mut self.store;
        store.set_fuel(host::INVOCATION_FUEL)?;
        match bindings.call_invoke(store, command, args)? {
            Ok(code) => Ok(code),
            Err(error) => anyhow::bail!(error),
        }
    }
}

#[derive(Clone)]
struct Provider {
    component_id: String,
    version: Version,
    priority: i32,
    keys: Vec<String>,
    handle: InstanceHandle,
}

impl ServiceRegistryHost for HostState {
    fn providers(
        &mut self,
        name: String,
        version: String,
    ) -> Result<Vec<ProviderDescription>, String> {
        let requirement = VersionReq::parse(&version).map_err(|error| error.to_string())?;
        self.registry
            .providers
            .read()
            .map_err(|_| "service registry poisoned".to_owned())
            .map(|values| {
                values
                    .get(&name)
                    .into_iter()
                    .flatten()
                    .filter(|provider| requirement.matches(&provider.version))
                    .map(|provider| ProviderDescription {
                        component_id: provider.component_id.clone(),
                        version: provider.version.to_string(),
                        priority: provider.priority,
                        keys: provider.keys.clone(),
                    })
                    .collect()
            })
    }
}

#[derive(Clone, Default)]
pub struct ServiceRegistry {
    providers: Arc<RwLock<BTreeMap<String, Vec<Provider>>>>,
}

impl ServiceRegistry {
    pub fn refresh<'a>(&self, components: impl IntoIterator<Item = &'a LoadedComponent>) {
        let mut providers = BTreeMap::<String, Vec<Provider>>::new();
        for component in components {
            let Some(handle) = component.active_handle() else {
                continue;
            };
            for service in &component.descriptor.provides {
                providers
                    .entry(service.name.clone())
                    .or_default()
                    .push(Provider {
                        component_id: component.descriptor.id.clone(),
                        version: service.version.clone(),
                        priority: service.priority,
                        keys: service.keys.clone(),
                        handle: handle.clone(),
                    });
            }
        }
        for values in providers.values_mut() {
            values.sort_by(|left, right| {
                right
                    .priority
                    .cmp(&left.priority)
                    .then_with(|| right.version.cmp(&left.version))
                    .then_with(|| left.component_id.cmp(&right.component_id))
            });
        }
        *self.providers.write().expect("service registry poisoned") = providers;
    }

    fn call(
        &self,
        service: &str,
        requirement: &VersionReq,
        selection: SelectionMode,
        function: &str,
        params: &[Val],
        results: &mut [Val],
    ) -> wasmtime::Result<()> {
        let key = match selection {
            SelectionMode::Single => None,
            SelectionMode::Keyed => Some(match params.first() {
                Some(Val::String(value)) => value.as_str(),
                _ => {
                    return Err(wasmtime::format_err!(
                        "keyed service {service} requires a string key as its first argument"
                    ));
                }
            }),
        };
        let provider = self
            .providers
            .read()
            .map_err(|_| wasmtime::format_err!("service registry poisoned"))?
            .get(service)
            .and_then(|values| {
                values.iter().find(|provider| {
                    requirement.matches(&provider.version)
                        && key.is_none_or(|key| provider.keys.iter().any(|value| value == key))
                })
            })
            .cloned()
            .ok_or_else(|| {
                wasmtime::format_err!("service {service} {requirement} is unavailable")
            })?;
        let key = format!("{}#{service}#{function}", provider.component_id);
        let _guard = CallGuard::enter(key)?;
        let mut active = provider
            .handle
            .lock()
            .map_err(|_| wasmtime::format_err!("provider {} is poisoned", provider.component_id))?;
        let func = *active
            .exports
            .get(&(service.to_owned(), function.to_owned()))
            .ok_or_else(|| wasmtime::format_err!("provider is missing {service}#{function}"))?;
        active.store.set_fuel(host::SERVICE_FUEL)?;
        func.call(&mut active.store, params, results)
    }
}

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
        instance.func_new(function, move |_store, _ty, params, results| {
            registry.call(
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

thread_local! {
    static CALL_STACK: RefCell<Vec<String>> = const { RefCell::new(Vec::new()) };
}

struct CallGuard;

impl CallGuard {
    fn enter(key: String) -> wasmtime::Result<Self> {
        CALL_STACK.with(|stack| {
            let mut stack = stack.borrow_mut();
            if stack.contains(&key) {
                return Err(wasmtime::format_err!(
                    "component service cycle detected at {key}"
                ));
            }
            stack.push(key);
            Ok(Self)
        })
    }
}

impl Drop for CallGuard {
    fn drop(&mut self) {
        CALL_STACK.with(|stack| {
            stack.borrow_mut().pop();
        });
    }
}
