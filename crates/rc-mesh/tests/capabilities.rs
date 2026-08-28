use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_mesh::*;

const NOW: i64 = 1_800_000_000_000;

fn identity(byte: u8) -> (String, String, PeerId) {
    let bytes = [byte; 32];
    let seed = URL_SAFE_NO_PAD.encode(bytes);
    let signing = ed25519_dalek::SigningKey::from_bytes(&bytes);
    let public = URL_SAFE_NO_PAD.encode(signing.verifying_key().to_bytes());
    let peer = PeerId::from_public_key(&public).unwrap();
    (seed, public, peer)
}

fn signed_capabilities(
    realm: &RealmId,
    origin: &PeerId,
    seed: &str,
    capabilities: Vec<CapabilityAdvertisement>,
) -> SignedAdvertisement {
    SignedAdvertisement::sign(
        LinkAdvertisement {
            v: ADVERTISEMENT_VERSION,
            realm_id: realm.clone(),
            origin: origin.clone(),
            sequence: 1,
            issued_at: NOW,
            expires_at: NOW + 60_000,
            capabilities,
            neighbors: Vec::new(),
            services: Vec::new(),
        },
        seed,
    )
    .unwrap()
}

#[test]
fn negotiates_signed_peer_capabilities() {
    let realm = RealmId::new("workspace").unwrap();
    let (_, _, local) = identity(40);
    let (remote_seed, remote_public, remote) = identity(41);
    let topology = Topology::new(realm.clone(), local);
    topology.trust_peer(remote.clone(), remote_public);
    topology
        .insert(
            signed_capabilities(
                &realm,
                &remote,
                &remote_seed,
                vec![
                    CapabilityAdvertisement::new(
                        "rc.transport.quic",
                        [1, 2],
                        ["datagram", "relay"],
                    )
                    .unwrap(),
                ],
            ),
            NOW,
        )
        .unwrap();

    let local = CapabilityAdvertisement::new(
        "rc.transport.quic",
        [1, 2, 3],
        ["datagram", "ipv6", "relay"],
    )
    .unwrap();
    let requirement = CapabilityRequirement::from_local(&local, ["datagram"]).unwrap();
    assert_eq!(
        topology.negotiate_capability(&remote, &requirement, NOW),
        Some(NegotiatedCapability {
            id: "rc.transport.quic".into(),
            version: 2,
            features: vec!["datagram".into(), "relay".into()],
        })
    );
    assert!(
        topology
            .negotiate_capability(&remote, &requirement, NOW + 60_000)
            .is_none()
    );
}

#[test]
fn rejects_duplicate_capability_ids_in_one_advertisement() {
    let realm = RealmId::new("workspace").unwrap();
    let (seed, _, peer) = identity(42);
    let capability = CapabilityAdvertisement::new("rc.transport.quic", [1], ["datagram"]).unwrap();
    assert!(
        SignedAdvertisement::sign(
            LinkAdvertisement {
                v: ADVERTISEMENT_VERSION,
                realm_id: realm,
                origin: peer,
                sequence: 1,
                issued_at: NOW,
                expires_at: NOW + 60_000,
                capabilities: vec![capability.clone(), capability],
                neighbors: Vec::new(),
                services: Vec::new(),
            },
            &seed,
        )
        .is_err()
    );
}

#[test]
fn empty_capability_lists_preserve_the_original_signed_shape() {
    let realm = RealmId::new("workspace").unwrap();
    let (_, _, peer) = identity(43);
    let advertisement = LinkAdvertisement {
        v: ADVERTISEMENT_VERSION,
        realm_id: realm,
        origin: peer,
        sequence: 1,
        issued_at: NOW,
        expires_at: NOW + 60_000,
        capabilities: Vec::new(),
        neighbors: Vec::new(),
        services: Vec::new(),
    };
    let encoded = serde_json::to_value(advertisement).unwrap();
    assert!(encoded.get("capabilities").is_none());
}
