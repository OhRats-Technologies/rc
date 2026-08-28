use rc_server::{Database, EventHub, ExecutionHistory, ExecutionPolicy, now_ms};
use std::time::{SystemTime, UNIX_EPOCH};

#[test]
fn default_policy_removes_completed_metadata_and_process_events() -> anyhow::Result<()> {
    let fixture = Fixture::new()?;
    seed_history(&fixture.path)?;
    let policy = ExecutionPolicy::new(ExecutionHistory::None, 168);
    policy.cleanup_startup(&fixture.db)?;

    assert_eq!(count(&fixture.path, "processes", "status='running'")?, 1);
    assert_eq!(count(&fixture.path, "processes", "status='exited'")?, 0);
    assert_eq!(count(&fixture.path, "events", "kind LIKE 'process.%'")?, 0);
    assert_eq!(count(&fixture.path, "events", "kind='device.renamed'")?, 1);
    Ok(())
}

#[tokio::test]
async fn process_events_are_live_but_not_audited_by_default() -> anyhow::Result<()> {
    let fixture = Fixture::new()?;
    seed_identity(&fixture.path)?;
    let hub = EventHub::new(ExecutionPolicy::default());
    let mut receiver = hub.subscribe();

    hub.emit(
        &fixture.db,
        "process.created",
        Some("workspace"),
        Some("user"),
        Some("device"),
        serde_json::json!({"processId":"process"}),
    )?;
    let event = receiver.recv().await?;
    assert!(!event.audit);
    assert_eq!(event.process_id.as_deref(), Some("process"));
    assert_eq!(count(&fixture.path, "events", "kind='process.created'")?, 0);

    hub.emit(
        &fixture.db,
        "device.renamed",
        Some("workspace"),
        Some("user"),
        Some("device"),
        serde_json::json!({"name":"Mac"}),
    )?;
    let event = receiver.recv().await?;
    assert!(event.audit);
    assert_eq!(count(&fixture.path, "events", "kind='device.renamed'")?, 1);

    hub.emit(
        &fixture.db,
        "device.online",
        Some("workspace"),
        None,
        Some("device"),
        serde_json::json!({}),
    )?;
    let event = receiver.recv().await?;
    assert!(!event.audit);
    assert_eq!(count(&fixture.path, "events", "kind='device.online'")?, 0);
    Ok(())
}

#[test]
fn metadata_policy_retains_completed_rows_while_none_finalizes_them() -> anyhow::Result<()> {
    let metadata = Fixture::new()?;
    seed_active_process(&metadata.path, "metadata-process")?;
    metadata
        .db
        .mark_process_exit("device", "metadata-process", 0, "")?;
    ExecutionPolicy::new(ExecutionHistory::Metadata, 168)
        .finalize(&metadata.db, "metadata-process")?;
    assert_eq!(
        count(&metadata.path, "processes", "id='metadata-process'")?,
        1
    );

    let private = Fixture::new()?;
    seed_active_process(&private.path, "private-process")?;
    private
        .db
        .mark_process_exit("device", "private-process", 0, "")?;
    let policy = ExecutionPolicy::default();
    policy.finalize(&private.db, "private-process")?;
    assert_eq!(
        count(&private.path, "processes", "id='private-process'")?,
        0
    );
    let recent = policy
        .recent_process(&private.db, "user", "private-process")?
        .expect("completion remains available ephemerally");
    assert_eq!(recent["status"], "exited");
    assert_eq!(recent["exit_code"], 0);
    assert_eq!(recent["ephemeral"], true);
    Ok(())
}

#[test]
fn schema_one_migrates_and_reconciles_interrupted_processes() -> anyhow::Result<()> {
    let root = temp_dir("migration");
    std::fs::create_dir_all(&root)?;
    let path = root.join("migration.sqlite3");
    let database = Database::open(&path)?;
    seed_identity(&path)?;
    seed_active_process(&path, "interrupted")?;
    with_connection(&path, |connection| {
        connection.execute("DROP TABLE runtime_settings", [])?;
        connection.execute("PRAGMA user_version=1", [])?;
        Ok(())
    })?;
    drop(database);

    let migrated = Database::open(&path)?;
    migrated.configure_execution_history(ExecutionHistory::None)?;
    assert_eq!(count(&path, "processes", "id='interrupted'")?, 0);
    let version = with_connection(&path, |connection| {
        connection.query_row("PRAGMA user_version", [], |row| row.get::<_, i64>(0))
    })?;
    assert_eq!(version, 2);
    drop(migrated);
    std::fs::remove_dir_all(root)?;
    Ok(())
}

struct Fixture {
    root: std::path::PathBuf,
    path: std::path::PathBuf,
    db: Database,
}

impl Fixture {
    fn new() -> anyhow::Result<Self> {
        let root = std::env::temp_dir().join(format!("rc-execution-{}", uuid::Uuid::new_v4()));
        std::fs::create_dir_all(&root)?;
        let path = root.join("rc.sqlite3");
        let db = Database::open(&path)?;
        Ok(Self { root, path, db })
    }
}

impl Drop for Fixture {
    fn drop(&mut self) {
        let _ = std::fs::remove_dir_all(&self.root);
    }
}

fn seed_history(path: &std::path::Path) -> rusqlite::Result<()> {
    seed_identity(path)?;
    with_connection(path, |connection| {
        connection.execute(
            "INSERT INTO processes(id,device_id,origin,status,terminal,exit_code,created_by,created_at,completed_at) VALUES('running','device','browser','running',0,NULL,'user',?,NULL)",
            [now_ms()],
        )?;
        connection.execute(
            "INSERT INTO processes(id,device_id,origin,status,terminal,exit_code,created_by,created_at,completed_at) VALUES('exited','device','browser','exited',0,0,'user',?,?)",
            rusqlite::params![now_ms()-1_000,now_ms()-500],
        )?;
        connection.execute(
            "INSERT INTO events(workspace_id,user_id,device_id,kind,detail,created_at) VALUES('workspace','user','device','process.exited','{}',?)",
            [now_ms()],
        )?;
        connection.execute(
            "INSERT INTO events(workspace_id,user_id,device_id,kind,detail,created_at) VALUES('workspace','user','device','device.renamed','{}',?)",
            [now_ms()],
        )?;
        Ok(())
    })
}

fn seed_active_process(path: &std::path::Path, id: &str) -> rusqlite::Result<()> {
    seed_identity(path)?;
    with_connection(path, |connection| {
        connection.execute(
            "INSERT INTO processes(id,device_id,origin,status,terminal,created_by,created_at) VALUES(?,'device','browser','running',0,'user',?)",
            rusqlite::params![id,now_ms()],
        )?;
        Ok(())
    })
}

fn seed_identity(path: &std::path::Path) -> rusqlite::Result<()> {
    with_connection(path, |connection| {
        connection.execute(
            "INSERT OR IGNORE INTO users(id,name,created_at) VALUES('user','User',?)",
            [now_ms()],
        )?;
        connection.execute(
            "INSERT OR IGNORE INTO workspaces(id,name,created_by,created_at) VALUES('workspace','Workspace','user',?)",
            [now_ms()],
        )?;
        connection.execute(
            "INSERT OR IGNORE INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES('workspace','user','owner',?)",
            [now_ms()],
        )?;
        connection.execute(
            "INSERT OR IGNORE INTO devices(id,workspace_id,name,hostname,platform,arch,identity_public_key,transport_public_key,version,capabilities,created_at) VALUES('device','workspace','Device','host','darwin','arm64','identity','transport','test','[]',?)",
            [now_ms()],
        )?;
        Ok(())
    })
}

fn count(path: &std::path::Path, table: &str, predicate: &str) -> rusqlite::Result<i64> {
    with_connection(path, |connection| {
        connection.query_row(
            &format!("SELECT count(*) FROM {table} WHERE {predicate}"),
            [],
            |row| row.get(0),
        )
    })
}

fn with_connection<T>(
    path: &std::path::Path,
    f: impl FnOnce(&rusqlite::Connection) -> rusqlite::Result<T>,
) -> rusqlite::Result<T> {
    let connection = rusqlite::Connection::open(path)?;
    connection.execute("PRAGMA foreign_keys=ON", [])?;
    f(&connection)
}

fn temp_dir(label: &str) -> std::path::PathBuf {
    std::env::temp_dir().join(format!(
        "rc-execution-{label}-{}-{}",
        std::process::id(),
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_nanos()
    ))
}
