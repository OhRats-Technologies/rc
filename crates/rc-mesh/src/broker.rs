use crate::{MeshEnvelope, RealmId, RouteDescriptor, RouteError, RouteProvider, RouteTarget};
use parking_lot::RwLock;
use std::{
    collections::HashMap,
    sync::{
        Arc, Weak,
        atomic::{AtomicU64, Ordering},
    },
};

#[derive(Clone, Default)]
pub struct RouteBroker {
    inner: Arc<BrokerInner>,
}

#[derive(Default)]
struct BrokerInner {
    next_id: AtomicU64,
    routes: RwLock<HashMap<RouteKey, Vec<RouteEntry>>>,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct RouteKey {
    realm: RealmId,
    target: RouteTarget,
}

#[derive(Clone)]
struct RouteEntry {
    id: u64,
    descriptor: RouteDescriptor,
    provider: Arc<dyn RouteProvider>,
}

pub struct RouteLease {
    broker: Weak<BrokerInner>,
    key: RouteKey,
    id: u64,
    active: bool,
}

impl RouteBroker {
    pub fn register(
        &self,
        descriptor: RouteDescriptor,
        provider: Arc<dyn RouteProvider>,
    ) -> RouteLease {
        let id = self.inner.next_id.fetch_add(1, Ordering::Relaxed) + 1;
        let key = RouteKey {
            realm: descriptor.realm.clone(),
            target: descriptor.target.clone(),
        };
        let entry = RouteEntry {
            id,
            descriptor,
            provider,
        };
        let mut routes = self.inner.routes.write();
        let entries = routes.entry(key.clone()).or_default();
        entries.push(entry);
        entries.sort_by_key(|entry| (entry.descriptor.cost, entry.id));
        RouteLease {
            broker: Arc::downgrade(&self.inner),
            key,
            id,
            active: true,
        }
    }

    pub fn candidates(
        &self,
        realm: &RealmId,
        target: &RouteTarget,
        now_ms: i64,
    ) -> Vec<RouteDescriptor> {
        let key = RouteKey {
            realm: realm.clone(),
            target: target.clone(),
        };
        self.inner
            .routes
            .read()
            .get(&key)
            .into_iter()
            .flatten()
            .filter(|entry| {
                entry.descriptor.expires_at == 0 || entry.descriptor.expires_at > now_ms
            })
            .map(|entry| entry.descriptor.clone())
            .collect()
    }

    pub async fn send(
        &self,
        target: &RouteTarget,
        envelope: &MeshEnvelope,
        now_ms: i64,
    ) -> Result<(), RouteError> {
        let key = RouteKey {
            realm: envelope.realm.clone(),
            target: target.clone(),
        };
        let entries = self
            .inner
            .routes
            .read()
            .get(&key)
            .cloned()
            .unwrap_or_default();
        let mut attempted = false;
        for entry in entries {
            if entry.descriptor.expires_at != 0 && entry.descriptor.expires_at <= now_ms {
                continue;
            }
            attempted = true;
            if entry.provider.send(envelope).await.is_ok() {
                return Ok(());
            }
        }
        if attempted {
            Err(RouteError::Disconnected)
        } else {
            Err(RouteError::Unavailable)
        }
    }

    pub fn remove_expired(&self, now_ms: i64) {
        let mut routes = self.inner.routes.write();
        routes.retain(|_, entries| {
            entries.retain(|entry| {
                entry.descriptor.expires_at == 0 || entry.descriptor.expires_at > now_ms
            });
            !entries.is_empty()
        });
    }
}

impl RouteLease {
    pub fn revoke(mut self) {
        self.remove();
    }

    fn remove(&mut self) {
        if !self.active {
            return;
        }
        if let Some(broker) = self.broker.upgrade() {
            let mut routes = broker.routes.write();
            if let Some(entries) = routes.get_mut(&self.key) {
                entries.retain(|entry| entry.id != self.id);
                if entries.is_empty() {
                    routes.remove(&self.key);
                }
            }
        }
        self.active = false;
    }
}

impl Drop for RouteLease {
    fn drop(&mut self) {
        self.remove();
    }
}
