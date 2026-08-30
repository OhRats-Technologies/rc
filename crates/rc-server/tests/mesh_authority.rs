use rc_server::{Database, authority_snapshot, now_ms};
use std::time::{SystemTime, UNIX_EPOCH};

#[test]
fn rc_lock_snapshots_publish_pinned_mesh_device_identities() -> anyhow::Result<()> {
    let root = temp_dir();
    std::fs::create_dir_all(&root)?;
    let path = root.join("mesh-authority.sqlite3");
    drop(Database::open(&path)?);
    let connection = rusqlite::Connection::open(&path)?;
    connection.execute("PRAGMA foreign_keys=ON", [])?;
    {
        let now = now_ms();
        connection.execute(
            "INSERT INTO users(id,name,created_at) VALUES('owner','Owner',?)",
            [now],
        )?;
        connection.execute(
            "INSERT INTO workspaces(id,name,created_by,created_at) VALUES('realm','Realm','owner',?)",
            [now],
        )?;
        connection.execute(
            "INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES('realm','owner','owner',?)",
            [now],
        )?;
        connection.execute(
            "INSERT INTO devices(id,workspace_id,name,hostname,platform,arch,identity_public_key,transport_public_key,version,capabilities,created_at) \
             VALUES('b','realm','B','b','linux','amd64','identity-b','transport-b','test','[]',?),\
                   ('c','realm','C','c','linux','amd64','identity-c','transport-c','test','[]',?)",
            rusqlite::params![now, now],
        )?;
        connection.execute(
            "INSERT INTO schedule_grants(schedule_id,workspace_id,device_id,user_id,spec_hash,max_runtime_ms,expires_at,created_at) VALUES('nightly','realm','b','owner',?,60000,0,?),('expired','realm','b','owner',?,60000,?,?)",
            rusqlite::params!["a".repeat(64), now, "b".repeat(64), now - 1, now],
        )?;
    }
    drop(connection);
    let database = Database::open(&path)?;

    let snapshot = authority_snapshot(&database, "realm")?;
    assert_eq!(snapshot.workspace_id, "realm");
    assert_eq!(snapshot.devices.len(), 2);
    assert_eq!(snapshot.devices[0].id, "b");
    assert_eq!(snapshot.devices[0].identity_public_key, "identity-b");
    assert_eq!(snapshot.devices[0].transport_public_key, "transport-b");
    assert_eq!(snapshot.devices[1].id, "c");
    assert_eq!(snapshot.schedule_grants.len(), 1);
    assert_eq!(snapshot.schedule_grants[0].schedule_id, "nightly");
    assert_eq!(snapshot.schedule_grants[0].device_id, "b");
    assert_eq!(snapshot.schedule_grants[0].spec_hash, "a".repeat(64));
    drop(database);
    std::fs::remove_dir_all(root)?;
    Ok(())
}

fn temp_dir() -> std::path::PathBuf {
    std::env::temp_dir().join(format!(
        "rc-server-mesh-authority-{}-{}",
        std::process::id(),
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_nanos()
    ))
}
