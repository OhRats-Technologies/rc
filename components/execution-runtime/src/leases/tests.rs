use super::*;

fn attached(now: u64) -> Leases {
    Leases::new(now, Kind::Attached, 60_000, 350, None)
}

#[test]
fn attached_execution_expires_without_an_initial_attachment() {
    let mut leases = attached(1_000);
    assert!(!leases.expired(60_999));
    assert!(leases.expired(61_000));
}

#[test]
fn reattach_cancels_deadline_and_supersedes_stale_writer() {
    let mut leases = attached(1_000);
    leases.attach("old".into()).unwrap();
    leases.detach("old", 2_000);
    leases.attach("new".into()).unwrap();
    leases.detach("old", 70_000);
    assert!(!leases.expired(1_000_000));
    leases.detach("new", 1_000_000);
    assert!(!leases.expired(1_059_999));
    assert!(leases.expired(1_060_000));
}

#[test]
fn managed_execution_ignores_attachment_but_obeys_max_runtime() {
    let mut leases = Leases::new(5_000, Kind::Managed, 60_000, 350, Some(10_000));
    assert!(leases.attach("writer".into()).is_err());
    leases.detach("writer", 6_000);
    assert!(!leases.expired(14_999));
    assert!(leases.expired(15_000));
}

#[test]
fn terminate_escalates_after_its_grace() {
    let mut leases = Leases::new(0, Kind::Scheduled, 60_000, 350, None);
    leases.terminate(10_000);
    assert!(!leases.expired(10_349));
    assert!(leases.expired(10_350));
}

#[test]
fn scheduled_execution_obeys_its_max_runtime() {
    let mut leases = Leases::new(7_000, Kind::Scheduled, 60_000, 350, Some(2_500));
    assert!(!leases.expired(9_499));
    assert!(leases.expired(9_500));
}
