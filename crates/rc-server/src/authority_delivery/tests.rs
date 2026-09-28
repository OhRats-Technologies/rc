use super::*;

fn transition(previous: &str, generation: u64, next: &str, expires: i64) -> ServerToNode {
    ServerToNode::LockSync {
        snapshot: next.into(), previous_hash: previous.into(), previous_generation: generation,
        grant: serde_json::json!({"v":1,"clientId":"client","userId":"user","signingPublicKey":"key","issuedAt":1,"expiresAt":expires}).to_string(),
        credential_id: "credential".into(), assertion: "assertion".into(), signature: "owner-signature".into(),
    }
}

#[tokio::test]
async fn offline_updates_survive_restart_and_follow_only_acknowledged_parents() -> anyhow::Result<()>
{
    let root = std::env::temp_dir().join(format!("rc-authority-queue-{}", uuid::Uuid::new_v4()));
    std::fs::create_dir_all(&root)?;
    let path = root.join("db.sqlite3");
    let db = Database::open(&path)?;
    db.with_connection(|db| db.execute_batch("INSERT INTO users VALUES('user','User',1);
        INSERT INTO workspaces VALUES('workspace','Workspace','user',1);
        INSERT INTO devices(id,workspace_id,name,hostname,platform,arch,identity_public_key,transport_public_key,version,created_at,lock_hash,lock_generation)
        VALUES('device','workspace','Device','host','windows','amd64','identity','transport','1',1,'initial',0);"))?;
    let middle = "middle-snapshot";
    let final_snapshot = "final-snapshot";
    save(&db, "device", &transition("initial", 0, middle, 0))?;
    assert!(!deliver(&NodeHub::default(), &db, "device").await?);
    assert_eq!(
        parents(&db, "workspace")?,
        vec![(authority_hash(middle), 1)]
    );
    save(
        &db,
        "device",
        &transition(&authority_hash(middle), 1, final_snapshot, 0),
    )?;
    drop(db);
    let db = Database::open(&path)?;
    let Some(ServerToNode::LockSync {
        snapshot,
        signature,
        ..
    }) = pending(&db, "device")?
    else {
        panic!("missing durable update")
    };
    assert_eq!(snapshot, middle);
    assert_eq!(signature, "owner-signature");
    db.mark_lock_state("device", &authority_hash(middle), 1)?;
    let Some(ServerToNode::LockSync { snapshot, .. }) = pending(&db, "device")? else {
        panic!("missing next update")
    };
    assert_eq!(snapshot, final_snapshot);
    db.mark_lock_state("device", &authority_hash(final_snapshot), 2)?;
    assert!(pending(&db, "device")?.is_none());
    save(
        &db,
        "device",
        &transition(&authority_hash(final_snapshot), 2, "expired", 1),
    )?;
    assert!(pending(&db, "device")?.is_none());
    assert!(parents(&db, "workspace")?.is_empty());
    drop(db);
    std::fs::remove_dir_all(root)?;
    Ok(())
}
