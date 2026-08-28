use crate::{Activation, Component, Context, ServiceKey};
use std::{collections::HashMap, sync::Arc};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ComponentState {
    Waiting,
    Active,
}

#[derive(Debug, thiserror::Error)]
pub enum RuntimeError {
    #[error("component {0} is already registered")]
    DuplicateComponent(&'static str),
    #[error("component {component} attempted to replace service {service}")]
    DuplicateService {
        component: &'static str,
        service: &'static str,
    },
    #[error("component {component} failed to activate: {source}")]
    Activation {
        component: &'static str,
        #[source]
        source: anyhow::Error,
    },
    #[error("replacement component must keep the existing name {0}")]
    MismatchedReplacement(&'static str),
    #[error("component replacement failed and rollback also failed: {0}")]
    Rollback(String),
}

pub struct Runtime {
    context: Context,
    components: Vec<Arc<dyn Component>>,
    active: HashMap<&'static str, ActiveComponent>,
}

struct ActiveComponent {
    owner: u64,
    services: Vec<ServiceKey>,
    activation: Activation,
}

impl Runtime {
    pub fn new(context: Context) -> Self {
        Self {
            context,
            components: Vec::new(),
            active: HashMap::new(),
        }
    }

    pub fn context(&self) -> &Context {
        &self.context
    }

    pub fn register(&mut self, component: Arc<dyn Component>) -> Result<(), RuntimeError> {
        if self
            .components
            .iter()
            .any(|existing| existing.name() == component.name())
        {
            return Err(RuntimeError::DuplicateComponent(component.name()));
        }
        self.components.push(component);
        Ok(())
    }

    pub fn state(&self, name: &str) -> Option<ComponentState> {
        self.components
            .iter()
            .find(|component| component.name() == name)
            .map(|component| {
                if self.active.contains_key(component.name()) {
                    ComponentState::Active
                } else {
                    ComponentState::Waiting
                }
            })
    }

    pub async fn replace(
        &mut self,
        name: &'static str,
        replacement: Arc<dyn Component>,
    ) -> Result<(), RuntimeError> {
        if replacement.name() != name {
            return Err(RuntimeError::MismatchedReplacement(name));
        }
        let Some(index) = self
            .components
            .iter()
            .position(|component| component.name() == name)
        else {
            return self.register(replacement);
        };
        let previous = self.components[index].clone();
        self.deactivate(name).await;
        self.deactivate_unsatisfied().await;
        self.components[index] = replacement;
        match self.reconcile().await {
            Ok(()) => Ok(()),
            Err(error) => {
                self.deactivate(name).await;
                self.deactivate_unsatisfied().await;
                self.components[index] = previous;
                self.reconcile().await.map_err(|rollback| {
                    RuntimeError::Rollback(format!("replacement: {error}; rollback: {rollback}"))
                })?;
                Err(error)
            }
        }
    }

    pub async fn reconcile(&mut self) -> Result<(), RuntimeError> {
        self.deactivate_unsatisfied().await;
        loop {
            let mut progress = false;
            for component in self.components.clone() {
                if self.active.contains_key(component.name())
                    || !requirements_available(&self.context, component.as_ref())
                {
                    continue;
                }
                self.activate(component).await?;
                progress = true;
            }
            if !progress {
                break;
            }
        }
        Ok(())
    }

    pub async fn shutdown(&mut self) {
        let names: Vec<_> = self
            .components
            .iter()
            .rev()
            .map(|component| component.name())
            .collect();
        for name in names {
            self.deactivate(name).await;
        }
    }

    async fn activate(&mut self, component: Arc<dyn Component>) -> Result<(), RuntimeError> {
        let mut activation = Activation::new();
        if let Err(source) = component.activate(&self.context, &mut activation).await {
            activation.effects.revert().await;
            return Err(RuntimeError::Activation {
                component: component.name(),
                source,
            });
        }
        if let Some(service) = activation
            .services
            .iter()
            .find(|service| self.context.contains_local(service.key))
        {
            activation.effects.revert().await;
            return Err(RuntimeError::DuplicateService {
                component: component.name(),
                service: service.key.name(),
            });
        }
        let owner = Context::next_owner();
        let mut inserted = Vec::new();
        for service in &activation.services {
            if let Err(error) = self
                .context
                .insert_raw(service.key, owner, service.value.clone())
            {
                for key in inserted {
                    self.context.remove_raw(key, owner);
                }
                activation.effects.revert().await;
                return Err(RuntimeError::Activation {
                    component: component.name(),
                    source: error,
                });
            }
            inserted.push(service.key);
        }
        self.active.insert(
            component.name(),
            ActiveComponent {
                owner,
                services: inserted,
                activation,
            },
        );
        Ok(())
    }

    async fn deactivate_unsatisfied(&mut self) {
        loop {
            let names: Vec<_> = self
                .components
                .iter()
                .rev()
                .filter(|component| {
                    self.active.contains_key(component.name())
                        && !requirements_available(&self.context, component.as_ref())
                })
                .map(|component| component.name())
                .collect();
            if names.is_empty() {
                break;
            }
            for name in names {
                self.deactivate(name).await;
            }
        }
    }

    async fn deactivate(&mut self, name: &'static str) {
        let Some(mut active) = self.active.remove(name) else {
            return;
        };
        for key in active.services.iter().rev() {
            self.context.remove_raw(*key, active.owner);
        }
        active.activation.effects.revert().await;
    }
}

fn requirements_available(context: &Context, component: &dyn Component) -> bool {
    component
        .requirements()
        .into_iter()
        .all(|key| context.contains(key))
}
