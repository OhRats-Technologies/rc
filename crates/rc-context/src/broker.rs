use parking_lot::RwLock;
use std::{
    collections::BTreeMap,
    sync::{
        Arc, Weak,
        atomic::{AtomicU64, Ordering},
    },
};

#[derive(Clone)]
pub struct Broker<T: ?Sized + Send + Sync + 'static> {
    inner: Arc<BrokerInner<T>>,
}

struct BrokerInner<T: ?Sized> {
    next: AtomicU64,
    providers: RwLock<BTreeMap<(i32, u64), Arc<T>>>,
}

impl<T: ?Sized + Send + Sync + 'static> Default for Broker<T> {
    fn default() -> Self {
        Self {
            inner: Arc::new(BrokerInner {
                next: AtomicU64::new(1),
                providers: RwLock::new(BTreeMap::new()),
            }),
        }
    }
}

impl<T: ?Sized + Send + Sync + 'static> Broker<T> {
    pub fn register(&self, priority: i32, provider: Arc<T>) -> ProviderLease<T> {
        let id = self.inner.next.fetch_add(1, Ordering::Relaxed);
        self.inner
            .providers
            .write()
            .insert((-priority, id), provider);
        ProviderLease {
            broker: Arc::downgrade(&self.inner),
            key: (-priority, id),
            active: true,
        }
    }

    pub fn best(&self) -> Option<Arc<T>> {
        self.inner
            .providers
            .read()
            .first_key_value()
            .map(|(_, value)| value.clone())
    }

    pub fn providers(&self) -> Vec<Arc<T>> {
        self.inner.providers.read().values().cloned().collect()
    }
}

pub struct ProviderLease<T: ?Sized + Send + Sync + 'static> {
    broker: Weak<BrokerInner<T>>,
    key: (i32, u64),
    active: bool,
}

impl<T: ?Sized + Send + Sync + 'static> ProviderLease<T> {
    pub fn remove(mut self) {
        self.release();
    }

    fn release(&mut self) {
        if self.active {
            if let Some(broker) = self.broker.upgrade() {
                broker.providers.write().remove(&self.key);
            }
            self.active = false;
        }
    }
}

impl<T: ?Sized + Send + Sync + 'static> Drop for ProviderLease<T> {
    fn drop(&mut self) {
        self.release();
    }
}

#[cfg(test)]
mod tests {
    use super::Broker;
    use std::sync::Arc;

    #[test]
    fn falls_back_when_the_preferred_provider_disappears() {
        let broker = Broker::default();
        let fallback = broker.register(10, Arc::new("fallback"));
        let preferred = broker.register(20, Arc::new("preferred"));
        assert_eq!(*broker.best().unwrap(), "preferred");
        drop(preferred);
        assert_eq!(*broker.best().unwrap(), "fallback");
        drop(fallback);
        assert!(broker.best().is_none());
    }
}
