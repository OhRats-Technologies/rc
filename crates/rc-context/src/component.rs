use crate::{Context, EffectScope, ServiceKey};
use async_trait::async_trait;
use std::{any::Any, sync::Arc};

pub struct ProvidedService {
    pub(crate) key: ServiceKey,
    pub(crate) value: Arc<dyn Any + Send + Sync>,
}

impl ProvidedService {
    pub fn new<T: Send + Sync + 'static>(value: Arc<T>) -> Self {
        Self::named(std::any::type_name::<T>(), value)
    }

    pub fn named<T: Send + Sync + 'static>(name: &'static str, value: Arc<T>) -> Self {
        Self {
            key: ServiceKey::named::<T>(name),
            value,
        }
    }

    pub fn key(&self) -> ServiceKey {
        self.key
    }
}

#[must_use = "activation effects must be owned by the component runtime"]
#[derive(Default)]
pub struct Activation {
    pub services: Vec<ProvidedService>,
    pub effects: EffectScope,
}

impl Activation {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn provide<T: Send + Sync + 'static>(&mut self, value: Arc<T>) {
        self.services.push(ProvidedService::new(value));
    }

    pub fn provide_named<T: Send + Sync + 'static>(&mut self, name: &'static str, value: Arc<T>) {
        self.services.push(ProvidedService::named(name, value));
    }
}

#[async_trait]
pub trait Component: Send + Sync {
    fn name(&self) -> &'static str;

    fn requirements(&self) -> Vec<ServiceKey> {
        Vec::new()
    }

    async fn activate(&self, context: &Context, activation: &mut Activation) -> anyhow::Result<()>;
}
