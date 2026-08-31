use anyhow::Context as _;
use rusqlite::{Connection, MAIN_DB};
use std::{
    path::Path,
    sync::{Arc, Mutex, MutexGuard},
};

const SCHEMA_VERSION: i64 = 1;
const SCHEMA: &str = "
BEGIN IMMEDIATE;
CREATE TABLE rc_component_revisions(
    owner TEXT PRIMARY KEY,
    revision INTEGER NOT NULL CHECK(revision >= 0)
);
CREATE TABLE rc_component_entries(
    owner TEXT NOT NULL,
    bucket TEXT NOT NULL,
    key BLOB NOT NULL,
    value BLOB NOT NULL,
    PRIMARY KEY(owner, bucket, key)
) WITHOUT ROWID;
PRAGMA user_version=1;
COMMIT;
";

#[derive(Clone)]
pub struct Database {
    connection: Arc<Mutex<Connection>>,
    path: Arc<std::path::PathBuf>,
}

impl Database {
    pub fn open(path: &Path) -> anyhow::Result<Self> {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)?;
            rc_platform::protect_private_path(parent, true)?;
        }
        if path.exists() {
            rc_platform::validate_private_path(path, false)?;
        }
        let connection = Connection::open(path)?;
        secure_database(path)?;
        connection.execute_batch(
            "PRAGMA journal_mode=WAL;
             PRAGMA synchronous=FULL;
             PRAGMA foreign_keys=ON;
             PRAGMA busy_timeout=5000;
             PRAGMA wal_autocheckpoint=1000;",
        )?;
        let version: i64 = connection.query_row("PRAGMA user_version", [], |row| row.get(0))?;
        if version == 0 {
            let tables: i64 = connection.query_row(
                "SELECT count(*) FROM sqlite_master
                 WHERE type='table' AND name NOT LIKE 'sqlite_%'",
                [],
                |row| row.get(0),
            )?;
            anyhow::ensure!(tables == 0, "unversioned kernel database is not empty");
            connection.execute_batch(SCHEMA)?;
        } else {
            anyhow::ensure!(
                version == SCHEMA_VERSION,
                "unsupported kernel database schema {version}"
            );
        }
        check_connection(&connection)?;
        Ok(Self {
            connection: Arc::new(Mutex::new(connection)),
            path: Arc::new(std::fs::canonicalize(path)?),
        })
    }

    pub fn with_connection<T>(
        &self,
        operation: impl FnOnce(&Connection) -> rusqlite::Result<T>,
    ) -> rusqlite::Result<T> {
        let connection = self.lock()?;
        operation(&connection)
    }

    pub fn with_connection_mut_fallible<T, E>(
        &self,
        operation: impl FnOnce(&mut Connection) -> Result<T, E>,
    ) -> Result<T, E>
    where
        E: From<rusqlite::Error>,
    {
        let mut connection = self.lock().map_err(E::from)?;
        operation(&mut connection)
    }

    pub fn integrity_check(&self) -> anyhow::Result<()> {
        let connection = self.lock()?;
        check_connection(&connection)
    }

    pub fn backup_to(&self, destination: &Path) -> anyhow::Result<()> {
        anyhow::ensure!(!destination.exists(), "backup destination already exists");
        let parent = destination
            .parent()
            .filter(|value| !value.as_os_str().is_empty())
            .unwrap_or_else(|| Path::new("."));
        std::fs::create_dir_all(parent)?;
        let canonical_parent = std::fs::canonicalize(parent)?;
        let destination = canonical_parent.join(
            destination
                .file_name()
                .ok_or_else(|| anyhow::anyhow!("backup path has no filename"))?,
        );
        anyhow::ensure!(
            destination != *self.path,
            "backup destination is the live database"
        );
        let name = destination
            .file_name()
            .and_then(|value| value.to_str())
            .unwrap_or("kernel.sqlite3");
        let temporary = parent.join(format!(".{name}.{}.tmp", std::process::id()));
        let _ = std::fs::remove_file(&temporary);
        let result = (|| -> anyhow::Result<()> {
            self.lock()?
                .backup(MAIN_DB, &temporary, None)
                .context("write SQLite backup")?;
            let backup = Connection::open(&temporary).context("open SQLite backup")?;
            check_connection(&backup).context("check SQLite backup")?;
            drop(backup);
            secure_database(&temporary).context("protect SQLite backup")?;
            std::fs::File::open(&temporary)
                .context("open protected SQLite backup")?
                .sync_all()
                .context("sync SQLite backup")?;
            std::fs::rename(&temporary, &destination).context("activate SQLite backup")?;
            sync_directory(&canonical_parent).context("sync SQLite backup directory")?;
            Ok(())
        })();
        if result.is_err() {
            let _ = std::fs::remove_file(&temporary);
        }
        result
    }

    fn lock(&self) -> rusqlite::Result<MutexGuard<'_, Connection>> {
        self.connection
            .lock()
            .map_err(|_| rusqlite::Error::InvalidQuery)
    }
}

fn check_connection(connection: &Connection) -> anyhow::Result<()> {
    let result: String = connection.query_row("PRAGMA quick_check(1)", [], |row| row.get(0))?;
    anyhow::ensure!(
        result == "ok",
        "kernel database integrity check failed: {result}"
    );
    Ok(())
}

fn secure_database(path: &Path) -> std::io::Result<()> {
    rc_platform::protect_private_path(path, false)
}

#[cfg(unix)]
fn sync_directory(path: &Path) -> std::io::Result<()> {
    std::fs::File::open(path)?.sync_all()
}

#[cfg(not(unix))]
fn sync_directory(_: &Path) -> std::io::Result<()> {
    Ok(())
}

#[cfg(test)]
mod tests;
