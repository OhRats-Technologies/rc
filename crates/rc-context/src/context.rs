use crate::ServiceKey;
use parking_lot::RwLock;
use std::{
    any::Any,
    collections::HashMap,
    sync::{
        Arc,
        atomic::{AtomicU64, Ordering},
    },
};

static NEXT_OWNER: AtomicU64 = AtomicU64::new(1);

#[derive(Clone)]
pub struct Context {
    inner: Arc<ContextInner>,
}

struct ContextInner {
    realm: Arc<str>,
    parent: Option<Context>,
    services: RwLock<HashMap<ServiceKey, ServiceEntry>>,
}

#[derive(Clone)]
struct ServiceEntry {
    owner: u64,
    value: Arc<dyn Any + Send + Sync>,
}

pub struct ServiceLease {
    context: Context,
    key: ServiceKey,
    owner: u64,
    active: bool,
}

impl Context {
    pub fn root(realm: impl Into<Arc<str>>) -> Self {
        Self {
            inner: Arc::new(ContextInner {
                realm: realm.into(),
                parent: None,
                services: RwLock::new(HashMap::new()),
            }),
        }
    }

    pub fn child(&self, realm: impl Into<Arc<str>>) -> Self {
        Self {
            inner: Arc::new(ContextInner {
                realm: realm.into(),
                parent: Some(self.clone()),
                services: RwLock::new(HashMap::new()),
            }),
        }
    }

    pub fn realm(&self) -> &str {
        &self.inner.realm
    }

    pub fn provide<T: Send + Sync + 'static>(&self, value: Arc<T>) -> anyhow::Result<ServiceLease> {
        self.provide_named(std::any::type_name::<T>(), value)
    }

    pub fn provide_named<T: Send + Sync + 'static>(
        &self,
        name: &'static str,
        value: Arc<T>,
    ) -> anyhow::Result<ServiceLease> {
        let key = ServiceKey::named::<T>(name);
        let owner = next_owner();
        self.insert_raw(key, owner, value)?;
        Ok(ServiceLease {
            context: self.clone(),
            key,
            owner,
            active: true,
        })
    }

    pub fn get<T: Send + Sync + 'static>(&self) -> Option<Arc<T>> {
        self.get_named(std::any::type_name::<T>())
    }

    pub fn get_named<T: Send + Sync + 'static>(&self, name: &'static str) -> Option<Arc<T>> {
        self.get_raw(ServiceKey::named::<T>(name))
            .and_then(|value| value.downcast::<T>().ok())
    }

    pub fn contains(&self, key: ServiceKey) -> bool {
        self.get_raw(key).is_some()
    }

    pub(crate) fn contains_local(&self, key: ServiceKey) -> bool {
        self.inner.services.read().contains_key(&key)
    }

    pub(crate) fn insert_raw(
        &self,
        key: ServiceKey,
        owner: u64,
        value: Arc<dyn Any + Send + Sync>,
    ) -> anyhow::Result<()> {
        let mut services = self.inner.services.write();
        if services.contains_key(&key) {
            anyhow::bail!(
                "service {} is already provided in realm {}",
                key.name(),
                self.realm()
            );
        }
        services.insert(key, ServiceEntry { owner, value });
        Ok(())
    }

    pub(crate) fn remove_raw(&self, key: ServiceKey, owner: u64) -> bool {
        let mut services = self.inner.services.write();
        if services.get(&key).is_some_and(|entry| entry.owner == owner) {
            services.remove(&key);
            true
        } else {
            false
        }
    }

    pub(crate) fn next_owner() -> u64 {
        next_owner()
    }

    fn get_raw(&self, key: ServiceKey) -> Option<Arc<dyn Any + Send + Sync>> {
        self.inner
            .services
            .read()
            .get(&key)
            .map(|entry| entry.value.clone())
            .or_else(|| {
                self.inner
                    .parent
                    .as_ref()
                    .and_then(|parent| parent.get_raw(key))
            })
    }
}

impl ServiceLease {
    pub fn revoke(mut self) {
        self.remove();
    }

    fn remove(&mut self) {
        if self.active {
            self.context.remove_raw(self.key, self.owner);
            self.active = false;
        }
    }
}

impl Drop for ServiceLease {
    fn drop(&mut self) {
        self.remove();
    }
}

fn next_owner() -> u64 {
    NEXT_OWNER.fetch_add(1, Ordering::Relaxed)
}
