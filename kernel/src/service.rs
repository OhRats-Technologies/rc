use crate::{
    bindings::{
        Plugin,
        ohrats::rc_plugin::service_registry::{
            Host as ServiceRegistryHost, Provider as ProviderDescription,
        },
    },
    component::LoadedComponent,
    descriptor::SelectionMode,
    host::{self, HostState},
};
use semver::{Version, VersionReq};
use std::{
    collections::BTreeMap,
    sync::{Arc, Mutex, RwLock},
};
use wasmtime::{
    Store,
    component::{Func, Val},
};

mod call;
mod link;

pub use link::{active_instance, linker};

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

#[derive(Clone)]
pub(crate) struct PinnedProvider(Provider);

impl PinnedProvider {
    pub(crate) fn component_id(&self) -> &str {
        &self.0.component_id
    }

    pub(crate) fn call(
        &self,
        service: &str,
        function: &str,
        params: &[Val],
    ) -> wasmtime::Result<Vec<Val>> {
        call::provider_owned(&self.0, service, function, params)
    }
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
            .matching(service, requirement)?
            .into_iter()
            .find(|provider| key.is_none_or(|key| provider.keys.iter().any(|value| value == key)))
            .ok_or_else(|| {
                wasmtime::format_err!("service {service} {requirement} is unavailable")
            })?;
        call::provider(&provider, service, function, params, results)
    }

    pub fn call_all(
        &self,
        service: &str,
        requirement: &VersionReq,
        function: &str,
        params: &[Val],
    ) -> wasmtime::Result<Vec<(String, wasmtime::Result<Vec<Val>>)>> {
        Ok(self
            .matching(service, requirement)?
            .into_iter()
            .map(|provider| {
                let result = call::provider_owned(&provider, service, function, params);
                (provider.component_id, result)
            })
            .collect())
    }

    pub(crate) fn pinned(
        &self,
        service: &str,
        requirement: &VersionReq,
    ) -> wasmtime::Result<Vec<PinnedProvider>> {
        Ok(self
            .matching(service, requirement)?
            .into_iter()
            .map(PinnedProvider)
            .collect())
    }

    fn matching(&self, service: &str, requirement: &VersionReq) -> wasmtime::Result<Vec<Provider>> {
        Ok(self
            .providers
            .read()
            .map_err(|_| wasmtime::format_err!("service registry poisoned"))?
            .get(service)
            .into_iter()
            .flatten()
            .filter(|provider| requirement.matches(&provider.version))
            .cloned()
            .collect())
    }
}
