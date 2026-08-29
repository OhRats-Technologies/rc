wit_bindgen::generate!({
    path: "../../wit",
    world: "mesh-policy-fixture",
    generate_all,
});

use ohrats::{
    rc_mesh::{
        capabilities, envelopes, topology,
        types::{
            CapabilityAdvertisement, CapabilityRequirement, Envelope, LinkAdvertisement, Neighbor,
            Rejection, ServiceAdvertisement,
        },
    },
    rc_plugin::types::{Command, Requirement, Selection},
};

struct MeshPolicyFixture;

impl Guest for MeshPolicyFixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:mesh-policy-fixture".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: vec![
                requirement("ohrats:rc-mesh/capabilities"),
                requirement("ohrats:rc-mesh/topology"),
                requirement("ohrats:rc-mesh/envelopes"),
            ],
            commands: vec![Command {
                name: "mesh-policy-verify".into(),
                summary: "Verify deterministic mesh policy contracts".into(),
                usage: "rc mesh-policy-verify".into(),
            }],
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }
    fn deactivate() {}

    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        if command != "mesh-policy-verify" || !args.is_empty() {
            return Err(format!("unsupported command {command:?}"));
        }
        verify()?;
        println!("mesh policy fixture: ok");
        Ok(0)
    }
}

fn verify() -> Result<(), String> {
    capability()?;
    topology_contract()?;
    envelope_contract()?;
    Ok(())
}

fn capability() -> Result<(), String> {
    let negotiated = capabilities::negotiate(
        &CapabilityRequirement {
            id: "rc.transport.quic".into(),
            versions: vec![1, 2, 3],
            supported_features: vec!["datagram".into(), "relay".into()],
            required_features: vec!["datagram".into()],
        },
        &CapabilityAdvertisement {
            id: "rc.transport.quic".into(),
            versions: vec![1, 2],
            features: vec!["datagram".into()],
        },
    )
    .map_err(rejection)?
    .ok_or("compatible capability was not negotiated")?;
    if negotiated.version != 2 || negotiated.features != vec!["datagram"] {
        return Err("capability negotiation was not deterministic".into());
    }
    if !matches!(
        capabilities::negotiate(
            &CapabilityRequirement {
                id: "rc.transport.quic".into(),
                versions: vec![2, 1],
                supported_features: vec!["datagram".into()],
                required_features: Vec::new(),
            },
            &CapabilityAdvertisement {
                id: "rc.transport.quic".into(),
                versions: vec![1, 2],
                features: vec!["datagram".into()],
            },
        ),
        Err(Rejection::InvalidCapability)
    ) {
        return Err("noncanonical capability input was accepted".into());
    }
    Ok(())
}

fn topology_contract() -> Result<(), String> {
    let advertisements = vec![
        advertisement("peer-a", vec![neighbor("peer-b", 2)], Vec::new()),
        advertisement("peer-b", vec![neighbor("peer-c", 3)], Vec::new()),
        advertisement(
            "peer-c",
            Vec::new(),
            vec![ServiceAdvertisement {
                name: "artifact-cache".into(),
                cost: 7,
            }],
        ),
    ];
    let trusted = vec!["peer-b".to_owned(), "peer-c".to_owned()];
    let routes = topology::routes("realm-a", "peer-a", &trusted, &advertisements, 1_000)
        .map_err(rejection)?;
    let route = routes
        .iter()
        .find(|route| route.destination_peer_id == "peer-c")
        .ok_or("route to peer-c was not planned")?;
    if route.next_hop_peer_id != "peer-b" || route.total_cost != 5 {
        return Err("shortest mesh route was incorrect".into());
    }
    let service = topology::service(
        "realm-a",
        "peer-a",
        &trusted,
        &advertisements,
        "artifact-cache",
        1_000,
    )
    .map_err(rejection)?
    .ok_or("service route was not planned")?;
    if service.provider_peer_id != "peer-c" || service.total_cost != 12 {
        return Err("service route cost was incorrect".into());
    }
    Ok(())
}

fn envelope_contract() -> Result<(), String> {
    let value = Envelope {
        version: 1,
        realm_id: "realm-a".into(),
        message_id: "message-a".into(),
        source_peer_id: "peer-a".into(),
        destination_peer_id: "peer-c".into(),
        issued_at_ms: 500,
        expires_at_ms: 5_000,
        max_hops: 4,
        ciphertext: b"opaque-ciphertext".to_vec(),
        route: Vec::new(),
    };
    envelopes::validate(&value, 1_000, 8).map_err(rejection)?;
    let forwarded = envelopes::forward(&value, "peer-b").map_err(rejection)?;
    if forwarded.route != vec!["peer-b"] {
        return Err("mesh relay route was not recorded".into());
    }
    if !matches!(
        envelopes::forward(&forwarded, "peer-b"),
        Err(Rejection::RouteRejected)
    ) {
        return Err("mesh relay loop was accepted".into());
    }
    let mut expired = value;
    expired.expires_at_ms = 999;
    if !matches!(
        envelopes::validate(&expired, 1_000, 8),
        Err(Rejection::Expired)
    ) {
        return Err("expired mesh envelope was accepted".into());
    }
    Ok(())
}

fn advertisement(
    origin_peer_id: &str,
    neighbors: Vec<Neighbor>,
    services: Vec<ServiceAdvertisement>,
) -> LinkAdvertisement {
    LinkAdvertisement {
        version: 1,
        realm_id: "realm-a".into(),
        origin_peer_id: origin_peer_id.into(),
        sequence: 1,
        issued_at_ms: 500,
        expires_at_ms: 5_000,
        capabilities: Vec::new(),
        neighbors,
        services,
    }
}

fn neighbor(peer_id: &str, cost: u32) -> Neighbor {
    Neighbor {
        peer_id: peer_id.into(),
        cost,
    }
}

fn requirement(name: &str) -> Requirement {
    Requirement {
        name: name.into(),
        version: "^0.1".into(),
        selection: Selection::Single,
    }
}

fn rejection(value: Rejection) -> String {
    let label = match value {
        Rejection::InvalidIdentifier => "invalid identifier",
        Rejection::InvalidCapability => "invalid capability",
        Rejection::InvalidAdvertisement => "invalid advertisement",
        Rejection::Expired => "expired",
        Rejection::RealmMismatch => "realm mismatch",
        Rejection::UntrustedPeer => "untrusted peer",
        Rejection::InvalidEnvelope => "invalid envelope",
        Rejection::RouteRejected => "route rejected",
    };
    format!("mesh policy rejected fixture: {label}")
}

export!(MeshPolicyFixture);
