wit_bindgen::generate!({ path: "../../wit", world: "events-fixture", generate_all });

mod retention_checks;

use ohrats::{
    rc_events::{
        append, feed, query, retention,
        retention::Policy,
        types::{
            AppendRequest, Authorization, Detail, DeviceDetail, Filter, LifecycleKind,
            WorkspaceDetail,
        },
    },
    rc_plugin::types::{Command, Requirement, Selection},
};

struct EventsFixture;

impl Guest for EventsFixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:events-fixture".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: ["append", "query", "feed", "retention"]
                .into_iter()
                .map(requirement)
                .collect(),
            commands: vec![
                command(
                    "events-seed",
                    "Seed durable lifecycle events",
                    "rc events-seed <fixture>",
                ),
                command(
                    "events-verify",
                    "Verify durable lifecycle events",
                    "rc events-verify <fixture>",
                ),
            ],
        }
    }
    fn activate() -> Result<(), String> {
        Ok(())
    }
    fn deactivate() {}
    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        match command.as_str() {
            "events-seed" => seed(&args),
            "events-verify" => verify(&args),
            _ => Err(format!("unsupported command {command:?}")),
        }
    }
}

fn seed(args: &[String]) -> Result<u32, String> {
    let [fixture] = args else {
        return Err("usage: rc events-seed <fixture>".into());
    };
    valid_fixture(fixture)?;
    retention::configure(Policy { maximum_events: 8 })?;
    let workspace = format!("workspace-{fixture}");
    let first = append::append(&workspace_event(
        LifecycleKind::WorkspaceCreated,
        &workspace,
        "created",
        Some("retry-create"),
    ))?;
    let retry = append::append(&workspace_event(
        LifecycleKind::WorkspaceCreated,
        &workspace,
        "created",
        Some("retry-create"),
    ))?;
    if first.cursor != retry.cursor {
        return Err("idempotent retry allocated another cursor".into());
    }
    let conflict = workspace_event(
        LifecycleKind::WorkspaceCreated,
        &workspace,
        "different",
        Some("retry-create"),
    );
    if append::append(&conflict).is_ok() {
        return Err("idempotency key accepted a different event".into());
    }
    for (kind, device) in [
        (LifecycleKind::DeviceEnrolled, "device-a"),
        (LifecycleKind::DeviceOnline, "device-a"),
        (LifecycleKind::DeviceEnrolled, "device-b"),
    ] {
        append::append(&device_event(
            kind,
            &workspace,
            &format!("{device}-{fixture}"),
        ))?;
    }
    let mut invalid = workspace_event(
        LifecycleKind::WorkspaceRenamed,
        &workspace,
        &"x".repeat(121),
        None,
    );
    if append::append(&invalid).is_ok() {
        return Err("oversize event detail was accepted".into());
    }
    invalid.detail = Detail::Device(DeviceDetail {
        workspace_id: workspace,
        device_id: "device-bad".into(),
        name: None,
    });
    if append::append(&invalid).is_ok() {
        return Err("mismatched event detail was accepted".into());
    }
    println!("seed cursor: {}", first.cursor);
    Ok(0)
}

fn verify(args: &[String]) -> Result<u32, String> {
    let [fixture] = args else {
        return Err("usage: rc events-verify <fixture>".into());
    };
    valid_fixture(fixture)?;
    let workspace = format!("workspace-{fixture}");
    let auth = Authorization {
        requester_account_id: "account-fixture".into(),
        workspace_ids: vec![workspace.clone()],
        include_own_account_events: true,
    };
    let empty = Filter {
        kinds: Vec::new(),
        workspace_id: Some(workspace.clone()),
        device_id: None,
        account_id: None,
    };
    let page1 = query::query(&auth, 0, 2, &empty)?;
    let page2 = feed::poll(&auth, page1.next_cursor, 2, &empty)?;
    if page1.events.len() != 2
        || page2.events.len() != 2
        || page1.events[0].cursor >= page1.events[1].cursor
        || page1.next_cursor >= page2.next_cursor
    {
        return Err("event order or pagination cursor is invalid".into());
    }
    let filtered = Filter {
        kinds: vec![LifecycleKind::DeviceOnline],
        workspace_id: Some(workspace.clone()),
        device_id: Some(format!("device-a-{fixture}")),
        account_id: None,
    };
    if query::query(&auth, 0, 10, &filtered)?.events.len() != 1 {
        return Err("event filtering failed".into());
    }
    let no_match = Filter {
        kinds: vec![LifecycleKind::AccountDeleted],
        workspace_id: None,
        device_id: None,
        account_id: None,
    };
    let skipped = feed::poll(&auth, 0, 10, &no_match)?;
    if !skipped.events.is_empty() || skipped.next_cursor != page2.next_cursor {
        return Err("filtered feed did not advance its ordered cursor".into());
    }
    let denied = Authorization {
        requester_account_id: "account-fixture".into(),
        workspace_ids: Vec::new(),
        include_own_account_events: true,
    };
    if query::query(&denied, 0, 10, &empty).is_ok() {
        return Err("unauthorized workspace filter was accepted".into());
    }
    let hidden = Filter {
        kinds: Vec::new(),
        workspace_id: None,
        device_id: Some("not-visible".into()),
        account_id: None,
    };
    let skipped = feed::poll(&auth, 0, 10, &hidden)?;
    if !skipped.events.is_empty() || skipped.next_cursor == 0 {
        return Err("feed cursor did not advance over filtered events".into());
    }
    retention::configure(Policy { maximum_events: 2 })?;
    append::append(&device_event(
        LifecycleKind::DeviceOffline,
        &workspace,
        &format!("device-a-{fixture}"),
    ))?;
    let newest = append::append(&device_event(
        LifecycleKind::DeviceRevoked,
        &workspace,
        &format!("device-b-{fixture}"),
    ))?;
    let retained = query::query(&auth, 1, 10, &empty)?;
    if retained.events.len() != 2
        || !retained.reset_required
        || retained.events.last().map(|e| e.cursor) != Some(newest.cursor)
    {
        return Err("retention pruning or reset cursor failed".into());
    }
    if retention::configure(Policy { maximum_events: 0 }).is_ok() {
        return Err("invalid retention limit was accepted".into());
    }
    retention_checks::verify_idempotency(&workspace)?;
    println!("events state: ok");
    Ok(0)
}

fn workspace_event(
    kind: LifecycleKind,
    workspace: &str,
    name: &str,
    idempotency: Option<&str>,
) -> AppendRequest {
    AppendRequest {
        kind,
        occurred_at_ms: 1_000,
        actor_account_id: Some("account-fixture".into()),
        detail: Detail::Workspace(WorkspaceDetail {
            workspace_id: workspace.into(),
            name: Some(name.into()),
        }),
        idempotency_key: idempotency.map(Into::into),
    }
}
fn device_event(kind: LifecycleKind, workspace: &str, device: &str) -> AppendRequest {
    AppendRequest {
        kind,
        occurred_at_ms: 2_000,
        actor_account_id: Some("account-fixture".into()),
        detail: Detail::Device(DeviceDetail {
            workspace_id: workspace.into(),
            device_id: device.into(),
            name: None,
        }),
        idempotency_key: None,
    }
}
fn valid_fixture(value: &str) -> Result<(), String> {
    if !value.is_empty()
        && value.len() <= 40
        && value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'-')
    {
        Ok(())
    } else {
        Err("invalid fixture".into())
    }
}
fn command(name: &str, summary: &str, usage: &str) -> Command {
    Command {
        name: name.into(),
        summary: summary.into(),
        usage: usage.into(),
    }
}
fn requirement(name: &str) -> Requirement {
    Requirement {
        name: format!("ohrats:rc-events/{name}"),
        version: "^0.1".into(),
        selection: Selection::Single,
    }
}
export!(EventsFixture);
