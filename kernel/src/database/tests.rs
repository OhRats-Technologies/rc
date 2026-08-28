use super::Database;
use rusqlite::{Connection, ErrorCode, params};
use std::time::Duration;

#[test]
fn opens_a_private_wal_database() -> anyhow::Result<()> {
    let directory = tempfile::tempdir()?;
    let path = directory.path().join("kernel.sqlite3");
    let database = Database::open(&path)?;
    database.integrity_check()?;
    let mode: String = database.with_connection(|connection| {
        connection.query_row("PRAGMA journal_mode", [], |row| row.get(0))
    })?;
    assert_eq!(mode, "wal");
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        assert_eq!(std::fs::metadata(path)?.permissions().mode() & 0o777, 0o600);
    }
    Ok(())
}

#[test]
fn creates_a_consistent_online_backup() -> anyhow::Result<()> {
    let directory = tempfile::tempdir()?;
    let source = directory.path().join("kernel.sqlite3");
    let backup = directory.path().join("backup.sqlite3");
    let database = Database::open(&source)?;
    database.with_connection(|connection| {
        connection.execute(
            "INSERT INTO rc_component_entries(owner,bucket,key,value) VALUES(?,?,?,?)",
            params!["component", "items", b"key", b"value"],
        )?;
        Ok::<(), rusqlite::Error>(())
    })?;
    database.backup_to(&backup)?;
    let copy = Connection::open(&backup)?;
    let value: Vec<u8> = copy.query_row(
        "SELECT value FROM rc_component_entries WHERE owner='component'",
        [],
        |row| row.get(0),
    )?;
    assert_eq!(value, b"value");
    assert!(database.backup_to(&backup).is_err());
    Ok(())
}

#[test]
fn rolls_back_an_unfinished_transaction() -> anyhow::Result<()> {
    let directory = tempfile::tempdir()?;
    let path = directory.path().join("kernel.sqlite3");
    let database = Database::open(&path)?;
    database.with_connection_mut_fallible(|connection| {
        let transaction = connection.transaction()?;
        transaction.execute(
            "INSERT INTO rc_component_entries(owner,bucket,key,value) VALUES(?,?,?,?)",
            params!["component", "items", b"key", b"value"],
        )?;
        drop(transaction);
        Ok::<(), rusqlite::Error>(())
    })?;
    drop(database);
    let reopened = Database::open(&path)?;
    let count: i64 = reopened.with_connection(|connection| {
        connection.query_row("SELECT count(*) FROM rc_component_entries", [], |row| {
            row.get(0)
        })
    })?;
    assert_eq!(count, 0);
    Ok(())
}

#[test]
fn sqlite_rejects_a_competing_writer_while_locked() -> anyhow::Result<()> {
    let directory = tempfile::tempdir()?;
    let path = directory.path().join("kernel.sqlite3");
    drop(Database::open(&path)?);
    let first = Connection::open(&path)?;
    first.execute_batch("BEGIN IMMEDIATE")?;
    let second = Connection::open(&path)?;
    second.busy_timeout(Duration::from_millis(10))?;
    let error = second
        .execute(
            "INSERT INTO rc_component_revisions(owner,revision) VALUES('other',1)",
            [],
        )
        .expect_err("competing writer unexpectedly acquired the lock");
    assert!(matches!(
        error.sqlite_error_code(),
        Some(ErrorCode::DatabaseBusy | ErrorCode::DatabaseLocked)
    ));
    first.execute_batch("ROLLBACK")?;
    Ok(())
}
