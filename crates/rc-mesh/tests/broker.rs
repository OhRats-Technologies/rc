use async_trait::async_trait;
use bytes::Bytes;
use parking_lot::Mutex;
use rc_mesh::{
    CoordinatorPolicy, CoordinatorRole, MeshEnvelope, MeshPolicy, PeerId, RealmId, RevocationLease,
    RouteBroker, RouteDescriptor, RouteError, RouteProvider, RouteTarget, ServiceId,
};
use std::sync::Arc;

struct Provider {
    result: Result<(), RouteError>,
    calls: Arc<Mutex<Vec<String>>>,
    name: &'static str,
}

#[async_trait]
impl RouteProvider for Provider {
    async fn send(&self, _: &MeshEnvelope) -> Result<(), RouteError> {
        self.calls.lock().push(self.name.into());
        self.result.clone()
    }
}

fn peer(value: &str) -> PeerId {
    PeerId::new(value).unwrap()
}

fn realm(value: &str) -> RealmId {
    RealmId::new(value).unwrap()
}

fn envelope(realm: RealmId) -> MeshEnvelope {
    MeshEnvelope {
        version: 1,
        realm,
        message_id: "message-1".into(),
        source: peer("a"),
        destination: peer("c"),
        expires_at: 10_000,
        hop_limit: 4,
        payload: Bytes::from_static(b"opaque-ciphertext"),
    }
}

#[tokio::test]
async fn broker_fails_over_without_crossing_realms() {
    let broker = RouteBroker::default();
    let calls = Arc::new(Mutex::new(Vec::new()));
    let target = RouteTarget::Device(peer("c"));
    let realm_a = realm("workspace-a");
    let realm_b = realm("workspace-b");

    let _bad = broker.register(
        RouteDescriptor {
            realm: realm_a.clone(),
            target: target.clone(),
            via: peer("b"),
            cost: 1,
            expires_at: 0,
        },
        Arc::new(Provider {
            result: Err(RouteError::Disconnected),
            calls: calls.clone(),
            name: "b",
        }),
    );
    let _good = broker.register(
        RouteDescriptor {
            realm: realm_a.clone(),
            target: target.clone(),
            via: peer("d"),
            cost: 2,
            expires_at: 0,
        },
        Arc::new(Provider {
            result: Ok(()),
            calls: calls.clone(),
            name: "d",
        }),
    );
    let _other_realm = broker.register(
        RouteDescriptor {
            realm: realm_b.clone(),
            target: target.clone(),
            via: peer("x"),
            cost: 0,
            expires_at: 0,
        },
        Arc::new(Provider {
            result: Ok(()),
            calls: calls.clone(),
            name: "x",
        }),
    );

    broker.send(&target, &envelope(realm_a), 1).await.unwrap();
    assert_eq!(&*calls.lock(), &["b", "d"]);
    assert!(matches!(
        broker.send(&target, &envelope(realm("unknown")), 1).await,
        Err(RouteError::Unavailable)
    ));
}

#[test]
fn envelope_enforces_expiry_and_hop_limits() {
    let policy = MeshPolicy::default();
    let frame = envelope(realm("workspace"));
    frame.validate(1, policy.maximum_hops).unwrap();
    let next = frame.forwarded().unwrap();
    assert_eq!(next.hop_limit, 3);

    let mut expired = frame.clone();
    expired.expires_at = 1;
    assert!(expired.validate(1, policy.maximum_hops).is_err());

    let mut last_hop = frame;
    last_hop.hop_limit = 1;
    assert!(last_hop.forwarded().is_err());

    let mut oversized = envelope(realm("workspace"));
    oversized.payload = Bytes::from(vec![0; rc_mesh::MAX_ENVELOPE_PAYLOAD + 1]);
    assert!(oversized.validate(1, policy.maximum_hops).is_err());
}

#[test]
fn tier0_cannot_become_a_follower() {
    let upstream = ServiceId::new("root@other.example").unwrap();
    assert!(CoordinatorPolicy::from_parts(CoordinatorRole::Tier0, Some(upstream.clone())).is_err());
    let secondary = CoordinatorPolicy::secondary(upstream.clone());
    assert_eq!(secondary.upstream(), Some(&upstream));
    assert_eq!(CoordinatorPolicy::tier0().upstream(), None);
}

#[test]
fn revocation_knowledge_is_explicitly_leased() {
    let lease = RevocationLease {
        realm: realm("workspace"),
        epoch: 9,
        issued_at: 1_000,
        valid_until: 2_000,
        tombstone_root: "a".repeat(64),
    };
    lease.validate(1_500).unwrap();
    assert!(lease.validate(2_000).is_err());
}
