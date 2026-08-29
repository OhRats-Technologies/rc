wit_bindgen::generate!({
    path: "../../wit",
    world: "mesh-policy",
    generate_all,
});

mod capability;
mod envelope;
mod topology;
mod validate;

use exports::ohrats::rc_mesh::{
    capabilities::Guest as CapabilitiesGuest, envelopes::Guest as EnvelopesGuest,
    topology::Guest as TopologyGuest,
};
use ohrats::{
    rc_mesh::types::{
        CapabilityAdvertisement, CapabilityRequirement, Envelope, LinkAdvertisement,
        NegotiatedCapability, Rejection, Route, ServiceRoute,
    },
    rc_plugin::types::Service,
};

struct MeshPolicy;

impl Guest for MeshPolicy {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:mesh-policy".into(),
            version: "0.1.0".into(),
            provides: vec![
                service("ohrats:rc-mesh/capabilities"),
                service("ohrats:rc-mesh/topology"),
                service("ohrats:rc-mesh/envelopes"),
            ],
            requires: Vec::new(),
            commands: Vec::new(),
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        Err(format!("unsupported command {command:?}"))
    }
}

impl CapabilitiesGuest for MeshPolicy {
    fn negotiate(
        local: CapabilityRequirement,
        remote: CapabilityAdvertisement,
    ) -> Result<Option<NegotiatedCapability>, Rejection> {
        capability::negotiate(local, remote)
    }
}

impl TopologyGuest for MeshPolicy {
    fn routes(
        realm_id: String,
        local_peer_id: String,
        trusted_peer_ids: Vec<String>,
        advertisements: Vec<LinkAdvertisement>,
        now_ms: u64,
    ) -> Result<Vec<Route>, Rejection> {
        topology::routes(
            &realm_id,
            &local_peer_id,
            &trusted_peer_ids,
            &advertisements,
            now_ms,
        )
    }

    fn service(
        realm_id: String,
        local_peer_id: String,
        trusted_peer_ids: Vec<String>,
        advertisements: Vec<LinkAdvertisement>,
        service: String,
        now_ms: u64,
    ) -> Result<Option<ServiceRoute>, Rejection> {
        topology::service(
            &realm_id,
            &local_peer_id,
            &trusted_peer_ids,
            &advertisements,
            &service,
            now_ms,
        )
    }
}

impl EnvelopesGuest for MeshPolicy {
    fn validate(value: Envelope, now_ms: u64, maximum_hops: u8) -> Result<(), Rejection> {
        envelope::validate(&value, now_ms, maximum_hops)
    }

    fn forward(value: Envelope, relay_peer_id: String) -> Result<Envelope, Rejection> {
        envelope::forward(value, &relay_peer_id)
    }
}

fn service(name: &str) -> Service {
    Service {
        name: name.into(),
        version: "0.1.0".into(),
        priority: 100,
        keys: Vec::new(),
    }
}

export!(MeshPolicy);
