use rc_mesh::*;

const NOW: i64 = 1_800_000_000_000;

fn identity(byte: u8) -> (String, String, PeerId) {
    use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
    let bytes = [byte; 32];
    let seed = URL_SAFE_NO_PAD.encode(bytes);
    let signing = ed25519_dalek::SigningKey::from_bytes(&bytes);
    let public = URL_SAFE_NO_PAD.encode(signing.verifying_key().to_bytes());
    let id = PeerId::from_public_key(&public).unwrap();
    (seed, public, id)
}

fn advertisement(
    realm: &RealmId,
    id: &PeerId,
    seed: &str,
    sequence: u64,
    neighbors: Vec<(PeerId, u32)>,
    services: Vec<(&str, u32)>,
) -> SignedAdvertisement {
    SignedAdvertisement::sign(
        LinkAdvertisement {
            v: ADVERTISEMENT_VERSION,
            realm_id: realm.clone(),
            origin: id.clone(),
            sequence,
            issued_at: NOW,
            expires_at: NOW + 60_000,
            capabilities: Vec::new(),
            neighbors: neighbors
                .into_iter()
                .map(|(peer_id, cost)| Neighbor { peer_id, cost })
                .collect(),
            services: services
                .into_iter()
                .map(|(name, cost)| ServiceAdvertisement {
                    name: name.into(),
                    cost,
                })
                .collect(),
        },
        seed,
    )
    .unwrap()
}

#[test]
fn discovers_transitive_routes_and_self_heals() {
    let realm = RealmId::new("workspace").unwrap();
    let (seed_a, public_a, a) = identity(1);
    let (seed_b, public_b, b) = identity(2);
    let (seed_c, public_c, c) = identity(3);
    let (seed_d, public_d, d) = identity(4);
    let topology = Topology::new(realm.clone(), a.clone());
    for (id, public) in [
        (&a, public_a),
        (&b, public_b),
        (&c, public_c),
        (&d, public_d),
    ] {
        topology.trust_peer(id.clone(), public);
    }
    topology
        .insert(
            advertisement(
                &realm,
                &a,
                &seed_a,
                1,
                vec![(b.clone(), 1), (d.clone(), 3)],
                vec![],
            ),
            NOW,
        )
        .unwrap();
    topology
        .insert(
            advertisement(
                &realm,
                &b,
                &seed_b,
                1,
                vec![(a.clone(), 1), (c.clone(), 1)],
                vec![("root", 2)],
            ),
            NOW,
        )
        .unwrap();
    topology
        .insert(
            advertisement(
                &realm,
                &c,
                &seed_c,
                1,
                vec![(b.clone(), 1), (d.clone(), 1)],
                vec![],
            ),
            NOW,
        )
        .unwrap();
    topology
        .insert(
            advertisement(
                &realm,
                &d,
                &seed_d,
                1,
                vec![(a.clone(), 3), (c.clone(), 1)],
                vec![],
            ),
            NOW,
        )
        .unwrap();

    let route = topology.route(&c, NOW).unwrap();
    assert_eq!(route.path, vec![a.clone(), b.clone(), c.clone()]);
    assert_eq!(topology.service("root", NOW).unwrap().provider, b.clone());

    topology
        .insert(advertisement(&realm, &b, &seed_b, 2, vec![], vec![]), NOW)
        .unwrap();
    let healed = topology.route(&c, NOW).unwrap();
    assert_eq!(healed.path, vec![a, d, c]);
}

#[test]
fn relays_opaque_signed_envelopes_and_rejects_replay() {
    let realm = RealmId::new("workspace").unwrap();
    let (seed_a, public_a, a) = identity(10);
    let (_, _, c) = identity(11);
    let (_, _, b) = identity(12);
    let envelope = SignedMeshEnvelope::sign(
        EnvelopeHeader {
            v: ENVELOPE_VERSION,
            realm_id: realm,
            message_id: "message-1".into(),
            source: a,
            destination: c,
            issued_at: NOW,
            expires_at: NOW + 60_000,
            max_hops: 4,
        },
        b"opaque ciphertext, not a command",
        &seed_a,
    )
    .unwrap();
    assert_eq!(
        envelope.verify(&public_a, NOW).unwrap(),
        b"opaque ciphertext, not a command"
    );
    let forwarded = envelope.forward(&b).unwrap();
    assert_eq!(forwarded.ciphertext, envelope.ciphertext);
    assert_eq!(forwarded.route, vec![b]);

    let guard = ReplayGuard::default();
    assert!(guard.accept(&envelope, NOW));
    assert!(!guard.accept(&envelope, NOW));

    let (seed_other, _, other) = identity(13);
    let same_id_other_source = SignedMeshEnvelope::sign(
        EnvelopeHeader {
            source: other,
            destination: envelope.header.destination.clone(),
            ..envelope.header.clone()
        },
        b"other source",
        &seed_other,
    )
    .unwrap();
    assert!(guard.accept(&same_id_other_source, NOW));
}

#[test]
fn rejects_invalid_advertisements_and_untrusted_edges() {
    let realm = RealmId::new("workspace").unwrap();
    let (seed_a, public_a, a) = identity(30);
    let (seed_b, public_b, b) = identity(31);
    let (_, _, untrusted) = identity(32);
    assert!(
        SignedAdvertisement::sign(
            LinkAdvertisement {
                v: ADVERTISEMENT_VERSION,
                realm_id: realm.clone(),
                origin: a.clone(),
                sequence: 1,
                issued_at: NOW,
                expires_at: NOW + 60_000,
                capabilities: Vec::new(),
                neighbors: vec![Neighbor {
                    peer_id: a.clone(),
                    cost: 1,
                }],
                services: Vec::new(),
            },
            &seed_a,
        )
        .is_err()
    );

    let topology = Topology::new(realm.clone(), a.clone());
    topology.trust_peer(a.clone(), public_a);
    topology.trust_peer(b.clone(), public_b);
    topology
        .insert(
            advertisement(
                &realm,
                &a,
                &seed_a,
                2,
                vec![(b.clone(), 1), (untrusted.clone(), 1)],
                Vec::new(),
            ),
            NOW,
        )
        .unwrap();
    topology
        .insert(
            advertisement(&realm, &b, &seed_b, 1, vec![(a.clone(), 1)], Vec::new()),
            NOW,
        )
        .unwrap();
    assert!(topology.route(&b, NOW).is_some());
    assert!(topology.route(&untrusted, NOW).is_none());
}

#[test]
fn state_digests_are_content_addressed_and_identity_bound() {
    let (seed, public, peer) = identity(21);
    let signed = SignedStateDigest::sign(
        MeshStateDigest {
            v: 1,
            realm_id: RealmId::new("workspace").unwrap(),
            origin: peer,
            sequence: 4,
            issued_at: NOW,
            expires_at: NOW + 60_000,
            authority: AuthorityHead {
                generation: 9,
                hash: "a".repeat(64),
                valid_until: NOW + 86_400_000,
            },
            device_operations_root: "b".repeat(64),
            revocation_epoch: 11,
            releases: vec![ReleaseHead {
                version: "0.18.0".into(),
                target: "darwin-arm64".into(),
                sha256: "c".repeat(64),
                size: 1024,
            }],
        },
        &seed,
    )
    .unwrap();
    signed.verify(&public, NOW).unwrap();

    let (_, wrong_public, _) = identity(22);
    assert!(signed.verify(&wrong_public, NOW).is_err());
}
