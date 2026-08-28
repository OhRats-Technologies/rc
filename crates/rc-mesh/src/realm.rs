use crate::{
    PeerId, RealmId, ReplayGuard, Route, SignedAdvertisement, SignedMeshEnvelope, Topology,
};
use async_trait::async_trait;
use rc_context::Broker;
use std::sync::Arc;

#[async_trait]
pub trait PeerRouteProvider: Send + Sync {
    fn peer_id(&self) -> &PeerId;
    async fn send(&self, envelope: SignedMeshEnvelope) -> anyhow::Result<()>;
}

pub struct MeshRealm {
    pub id: RealmId,
    pub local: PeerId,
    pub topology: Topology,
    replay: ReplayGuard,
    routes: Broker<dyn PeerRouteProvider>,
}

impl MeshRealm {
    pub fn new(id: RealmId, local: PeerId) -> Self {
        Self {
            topology: Topology::new(id.clone(), local.clone()),
            id,
            local,
            replay: ReplayGuard::default(),
            routes: Broker::default(),
        }
    }

    pub fn register_route(
        &self,
        priority: i32,
        provider: Arc<dyn PeerRouteProvider>,
    ) -> rc_context::ProviderLease<dyn PeerRouteProvider> {
        self.routes.register(priority, provider)
    }

    pub fn insert_advertisement(
        &self,
        advertisement: SignedAdvertisement,
        now_ms: i64,
    ) -> anyhow::Result<bool> {
        Ok(self.topology.insert(advertisement, now_ms)?)
    }

    pub fn accept_envelope(
        &self,
        envelope: &SignedMeshEnvelope,
        source_public_key: &str,
        now_ms: i64,
    ) -> anyhow::Result<EnvelopeDisposition> {
        if envelope.header.realm_id != self.id {
            anyhow::bail!("mesh envelope belongs to another realm");
        }
        envelope.verify(source_public_key, now_ms)?;
        if !self.replay.accept(envelope, now_ms) {
            anyhow::bail!("mesh envelope replay rejected");
        }
        if envelope.header.destination == self.local {
            Ok(EnvelopeDisposition::Deliver)
        } else {
            let route = self
                .topology
                .route(&envelope.header.destination, now_ms)
                .ok_or_else(|| anyhow::anyhow!("mesh destination is unreachable"))?;
            Ok(EnvelopeDisposition::Forward(route))
        }
    }

    pub async fn forward(
        &self,
        envelope: &SignedMeshEnvelope,
        route: &Route,
    ) -> anyhow::Result<()> {
        let provider = self
            .routes
            .providers()
            .into_iter()
            .find(|provider| provider.peer_id() == &route.next_hop)
            .ok_or_else(|| anyhow::anyhow!("mesh next hop is unavailable"))?;
        provider.send(envelope.forward(&self.local)?).await
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum EnvelopeDisposition {
    Deliver,
    Forward(Route),
}
