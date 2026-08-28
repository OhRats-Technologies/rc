use crate::RouteBroker;
use async_trait::async_trait;
use rc_context::{Activation, Component, Context};
use std::sync::Arc;

pub struct RouteBrokerComponent;

#[async_trait]
impl Component for RouteBrokerComponent {
    fn name(&self) -> &'static str {
        "mesh.route-broker"
    }

    async fn activate(&self, _: &Context, activation: &mut Activation) -> anyhow::Result<()> {
        activation.provide(Arc::new(RouteBroker::default()));
        Ok(())
    }
}
