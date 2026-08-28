use crate::{
    CapabilityAdvertisement, CapabilityRequirement, NegotiatedCapability, PeerId, RealmId,
    SignedAdvertisement,
};
use parking_lot::RwLock;
use std::{
    cmp::Reverse,
    collections::{BinaryHeap, HashMap},
};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Route {
    pub destination: PeerId,
    pub next_hop: PeerId,
    pub total_cost: u64,
    pub path: Vec<PeerId>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ServiceRoute {
    pub service: String,
    pub provider: PeerId,
    pub route: Route,
    pub total_cost: u64,
}

pub struct Topology {
    realm: RealmId,
    local: PeerId,
    keys: RwLock<HashMap<PeerId, String>>,
    advertisements: RwLock<HashMap<PeerId, SignedAdvertisement>>,
}

impl Topology {
    pub fn new(realm: RealmId, local: PeerId) -> Self {
        Self {
            realm,
            local,
            keys: RwLock::new(HashMap::new()),
            advertisements: RwLock::new(HashMap::new()),
        }
    }

    pub fn trust_peer(&self, peer: PeerId, public_key: String) {
        self.keys.write().insert(peer, public_key);
    }

    pub fn remove_peer(&self, peer: &PeerId) {
        self.keys.write().remove(peer);
        self.advertisements.write().remove(peer);
    }

    pub fn insert(
        &self,
        advertisement: SignedAdvertisement,
        now_ms: i64,
    ) -> Result<bool, TopologyError> {
        if advertisement.advertisement.realm_id != self.realm {
            return Err(TopologyError::Realm);
        }
        let origin = advertisement.advertisement.origin.clone();
        let key = self
            .keys
            .read()
            .get(&origin)
            .cloned()
            .ok_or(TopologyError::Untrusted)?;
        advertisement.verify(&key, now_ms)?;
        let mut advertisements = self.advertisements.write();
        if advertisements.get(&origin).is_some_and(|current| {
            current.advertisement.sequence >= advertisement.advertisement.sequence
        }) {
            return Ok(false);
        }
        advertisements.insert(origin, advertisement);
        Ok(true)
    }

    pub fn prune(&self, now_ms: i64) -> usize {
        let mut advertisements = self.advertisements.write();
        let before = advertisements.len();
        advertisements.retain(|_, value| value.advertisement.expires_at > now_ms);
        before - advertisements.len()
    }

    pub fn route(&self, destination: &PeerId, now_ms: i64) -> Option<Route> {
        self.routes(now_ms).remove(destination)
    }

    pub fn service(&self, service: &str, now_ms: i64) -> Option<ServiceRoute> {
        let routes = self.routes(now_ms);
        self.advertisements
            .read()
            .values()
            .filter(|entry| entry.advertisement.expires_at > now_ms)
            .filter_map(|entry| {
                let advertised = entry
                    .advertisement
                    .services
                    .iter()
                    .find(|value| value.name == service)?;
                let route = routes.get(&entry.advertisement.origin)?.clone();
                Some(ServiceRoute {
                    service: service.to_owned(),
                    provider: entry.advertisement.origin.clone(),
                    total_cost: route.total_cost + u64::from(advertised.cost),
                    route,
                })
            })
            .min_by_key(|route| (route.total_cost, route.provider.clone()))
    }

    pub fn capabilities(&self, peer: &PeerId, now_ms: i64) -> Option<Vec<CapabilityAdvertisement>> {
        let advertisements = self.advertisements.read();
        let entry = advertisements.get(peer)?;
        (entry.advertisement.expires_at > now_ms).then(|| entry.advertisement.capabilities.clone())
    }

    pub fn negotiate_capability(
        &self,
        peer: &PeerId,
        requirement: &CapabilityRequirement,
        now_ms: i64,
    ) -> Option<NegotiatedCapability> {
        self.capabilities(peer, now_ms)?
            .iter()
            .find_map(|capability| requirement.negotiate(capability))
    }

    pub fn routes(&self, now_ms: i64) -> HashMap<PeerId, Route> {
        let advertisements = self.advertisements.read();
        let trusted = self.keys.read();
        let mut graph = HashMap::<PeerId, Vec<(PeerId, u32)>>::new();
        for entry in advertisements.values() {
            if entry.advertisement.expires_at <= now_ms {
                continue;
            }
            graph.insert(
                entry.advertisement.origin.clone(),
                entry
                    .advertisement
                    .neighbors
                    .iter()
                    .filter(|neighbor| {
                        neighbor.peer_id == self.local || trusted.contains_key(&neighbor.peer_id)
                    })
                    .map(|neighbor| (neighbor.peer_id.clone(), neighbor.cost))
                    .collect(),
            );
        }
        shortest_paths(&graph, &self.local)
    }
}

fn shortest_paths(
    graph: &HashMap<PeerId, Vec<(PeerId, u32)>>,
    source: &PeerId,
) -> HashMap<PeerId, Route> {
    let mut distance = HashMap::<PeerId, u64>::new();
    let mut previous = HashMap::<PeerId, PeerId>::new();
    let mut queue = BinaryHeap::new();
    distance.insert(source.clone(), 0);
    queue.push(Reverse((0_u64, source.clone())));
    while let Some(Reverse((cost, peer))) = queue.pop() {
        if distance.get(&peer).is_none_or(|current| *current != cost) {
            continue;
        }
        for (neighbor, edge) in graph.get(&peer).into_iter().flatten() {
            let next = cost.saturating_add(u64::from(*edge));
            if distance.get(neighbor).is_none_or(|current| next < *current) {
                distance.insert(neighbor.clone(), next);
                previous.insert(neighbor.clone(), peer.clone());
                queue.push(Reverse((next, neighbor.clone())));
            }
        }
    }
    distance
        .into_iter()
        .filter(|(destination, _)| destination != source)
        .filter_map(|(destination, total_cost)| {
            let mut path = vec![destination.clone()];
            let mut cursor = destination.clone();
            while let Some(parent) = previous.get(&cursor) {
                path.push(parent.clone());
                if parent == source {
                    break;
                }
                cursor = parent.clone();
            }
            path.reverse();
            (path.first() == Some(source) && path.len() >= 2).then(|| Route {
                destination,
                next_hop: path[1].clone(),
                total_cost,
                path,
            })
        })
        .map(|route| (route.destination.clone(), route))
        .collect()
}

#[derive(Debug, thiserror::Error)]
pub enum TopologyError {
    #[error("mesh advertisement belongs to another realm")]
    Realm,
    #[error("mesh peer is not trusted in this realm")]
    Untrusted,
    #[error(transparent)]
    Advertisement(#[from] crate::AdvertisementError),
}
