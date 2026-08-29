use crate::{
    ohrats::rc_mesh::types::{LinkAdvertisement, Rejection, Route, ServiceRoute},
    validate,
};
use std::{
    cmp::Reverse,
    collections::{BTreeMap, BTreeSet, BinaryHeap},
};

pub fn routes(
    realm_id: &str,
    local_peer_id: &str,
    trusted_peer_ids: &[String],
    advertisements: &[LinkAdvertisement],
    now_ms: u64,
) -> Result<Vec<Route>, Rejection> {
    validate::identifier(realm_id)?;
    validate::identifier(local_peer_id)?;
    let trusted = trusted_peer_ids
        .iter()
        .map(|peer| {
            validate::identifier(peer)?;
            Ok(peer.clone())
        })
        .collect::<Result<BTreeSet<_>, Rejection>>()?;
    let mut origins = BTreeSet::new();
    let mut graph = BTreeMap::<String, Vec<(String, u32)>>::new();
    for item in advertisements {
        validate::advertisement(item)?;
        if item.realm_id != realm_id {
            return Err(Rejection::RealmMismatch);
        }
        if item.expires_at_ms <= now_ms {
            continue;
        }
        if item.origin_peer_id != local_peer_id && !trusted.contains(&item.origin_peer_id) {
            return Err(Rejection::UntrustedPeer);
        }
        if !origins.insert(item.origin_peer_id.clone()) {
            return Err(Rejection::InvalidAdvertisement);
        }
        graph.insert(
            item.origin_peer_id.clone(),
            item.neighbors
                .iter()
                .filter(|neighbor| {
                    neighbor.peer_id == local_peer_id || trusted.contains(&neighbor.peer_id)
                })
                .map(|neighbor| (neighbor.peer_id.clone(), neighbor.cost))
                .collect(),
        );
    }
    Ok(shortest_paths(&graph, local_peer_id))
}

pub fn service(
    realm_id: &str,
    local_peer_id: &str,
    trusted_peer_ids: &[String],
    advertisements: &[LinkAdvertisement],
    service: &str,
    now_ms: u64,
) -> Result<Option<ServiceRoute>, Rejection> {
    if service.is_empty() || service.len() > 128 {
        return Err(Rejection::InvalidAdvertisement);
    }
    let routes = routes(
        realm_id,
        local_peer_id,
        trusted_peer_ids,
        advertisements,
        now_ms,
    )?;
    let by_peer = routes
        .into_iter()
        .map(|route| (route.destination_peer_id.clone(), route))
        .collect::<BTreeMap<_, _>>();
    Ok(advertisements
        .iter()
        .filter(|item| item.expires_at_ms > now_ms)
        .filter_map(|item| {
            let advertised = item.services.iter().find(|value| value.name == service)?;
            let route = by_peer.get(&item.origin_peer_id)?.clone();
            Some(ServiceRoute {
                service: service.into(),
                provider_peer_id: item.origin_peer_id.clone(),
                total_cost: route.total_cost.saturating_add(u64::from(advertised.cost)),
                route,
            })
        })
        .min_by_key(|value| (value.total_cost, value.provider_peer_id.clone())))
}

fn shortest_paths(graph: &BTreeMap<String, Vec<(String, u32)>>, source: &str) -> Vec<Route> {
    let mut distance = BTreeMap::<String, u64>::new();
    let mut previous = BTreeMap::<String, String>::new();
    let mut queue = BinaryHeap::new();
    distance.insert(source.into(), 0);
    queue.push(Reverse((0_u64, source.to_owned())));
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
            (path.first().is_some_and(|peer| peer == source) && path.len() >= 2).then(|| Route {
                destination_peer_id: destination,
                next_hop_peer_id: path[1].clone(),
                total_cost,
                path,
            })
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ohrats::rc_mesh::types::{Neighbor, ServiceAdvertisement};

    #[test]
    fn chooses_shortest_trusted_path() {
        let ads = vec![
            advertisement("a", vec![neighbor("b", 2)]),
            advertisement("b", vec![neighbor("c", 3)]),
        ];
        let result = routes("realm", "a", &["b".into(), "c".into()], &ads, 1_000).unwrap();
        let c = result
            .iter()
            .find(|route| route.destination_peer_id == "c")
            .unwrap();
        assert_eq!(c.next_hop_peer_id, "b");
        assert_eq!(c.total_cost, 5);
    }

    fn advertisement(origin: &str, neighbors: Vec<Neighbor>) -> LinkAdvertisement {
        LinkAdvertisement {
            version: 1,
            realm_id: "realm".into(),
            origin_peer_id: origin.into(),
            sequence: 1,
            issued_at_ms: 500,
            expires_at_ms: 5_000,
            capabilities: Vec::new(),
            neighbors,
            services: Vec::<ServiceAdvertisement>::new(),
        }
    }

    fn neighbor(peer_id: &str, cost: u32) -> Neighbor {
        Neighbor {
            peer_id: peer_id.into(),
            cost,
        }
    }
}
