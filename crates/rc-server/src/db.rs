use rusqlite::{Connection, OptionalExtension, params};
mod device;
mod process;
mod runtime;
mod schema;
pub use device::*;
pub use process::*;
use std::{
    path::Path,
    sync::{Arc, Mutex, MutexGuard},
};

#[derive(Clone)]
pub struct Database(Arc<Mutex<Connection>>);

impl Database {
    pub(crate) fn with_connection<T>(
        &self,
        f: impl FnOnce(&Connection) -> rusqlite::Result<T>,
    ) -> rusqlite::Result<T> {
        let connection = self.lock()?;
        f(&connection)
    }

    pub(crate) fn with_connection_mut<T>(
        &self,
        f: impl FnOnce(&mut Connection) -> rusqlite::Result<T>,
    ) -> rusqlite::Result<T> {
        let mut connection = self.lock()?;
        f(&mut connection)
    }

    fn lock(&self) -> rusqlite::Result<MutexGuard<'_, Connection>> {
        self.0.lock().map_err(|_| rusqlite::Error::InvalidQuery)
    }

    pub fn open(path: &Path) -> anyhow::Result<Self> {
        let connection = Connection::open(path)?;
        connection.execute_batch(
            "PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;",
        )?;
        let current: i64 = connection.query_row("PRAGMA user_version", [], |row| row.get(0))?;
        if current == 0 {
            let tables: i64 = connection.query_row(
                "SELECT count(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'",
                [],
                |row| row.get(0),
            )?;
            if tables != 0 {
                anyhow::bail!(
                    "unversioned RC database is incompatible with the Rust schema; use a fresh rc-v2.sqlite3 database and re-enroll devices"
                );
            }
            connection.execute_batch(schema::SCHEMA)?;
        } else if current == 1 {
            connection.execute_batch(schema::MIGRATE_1_TO_2)?;
        } else if current != 2 {
            anyhow::bail!("unsupported RC database schema {current}");
        }
        secure_database(path)?;
        Ok(Self(Arc::new(Mutex::new(connection))))
    }

    pub fn client_auth(&self, id: &str) -> rusqlite::Result<Option<ClientAuthRow>> {
        self.lock()?
            .query_row(
                "SELECT user_id,kind,public_key,scopes,grant,credential_id,assertion,expires_at FROM clients WHERE id=? AND (expires_at=0 OR expires_at>?)",
                params![id, now_ms()],
                |row| {
                    Ok(ClientAuthRow {
                        user_id: row.get(0)?,
                        kind: row.get(1)?,
                        public_key: row.get(2)?,
                        scopes: row.get(3)?,
                        grant: row.get(4)?,
                        credential_id: row.get(5)?,
                        assertion: row.get(6)?,
                        expires_at: row.get(7)?,
                    })
                },
            )
            .optional()
    }

    pub fn touch_client(&self, id: &str) -> rusqlite::Result<()> {
        self.lock()?.execute(
            "UPDATE clients SET last_used=? WHERE id=?",
            params![now_ms(), id],
        )?;
        Ok(())
    }

    pub fn remember_nonce(
        &self,
        principal: &str,
        hash: &str,
        expires_at: i64,
    ) -> rusqlite::Result<bool> {
        let db = self.lock()?;
        db.execute("DELETE FROM request_nonces WHERE expires_at<?", [now_ms()])?;
        Ok(db.execute(
            "INSERT OR IGNORE INTO request_nonces(principal,nonce_hash,expires_at) VALUES(?,?,?)",
            params![principal, hash, expires_at],
        )? == 1)
    }
}

#[cfg(unix)]
fn secure_database(path: &Path) -> std::io::Result<()> {
    use std::os::unix::fs::PermissionsExt;
    std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o600))
}

#[cfg(not(unix))]
fn secure_database(_: &Path) -> std::io::Result<()> {
    Ok(())
}

#[derive(Debug, Clone)]
pub struct ClientAuthRow {
    pub user_id: String,
    pub kind: String,
    pub public_key: String,
    pub scopes: String,
    pub grant: Option<String>,
    pub credential_id: Option<String>,
    pub assertion: Option<String>,
    pub expires_at: i64,
}

pub fn now_ms() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

#[cfg(test)]
mod tests {
    use super::Database;

    #[test]
    fn rejects_unversioned_legacy_database_with_clear_error() -> anyhow::Result<()> {
        let directory = std::env::temp_dir().join(format!(
            "rc-legacy-db-{}-{}",
            std::process::id(),
            rand::random::<u64>()
        ));
        std::fs::create_dir_all(&directory)?;
        let path = directory.join("rc.db");
        let connection = rusqlite::Connection::open(&path)?;
        connection.execute("CREATE TABLE users(id TEXT PRIMARY KEY)", [])?;
        drop(connection);

        let error = match Database::open(&path) {
            Ok(_) => anyhow::bail!("legacy database was accepted"),
            Err(error) => error,
        };
        assert!(
            error
                .to_string()
                .contains("incompatible with the Rust schema")
        );
        std::fs::remove_dir_all(directory)?;
        Ok(())
    }

    #[cfg(unix)]
    #[test]
    fn creates_private_database_file() -> anyhow::Result<()> {
        use std::os::unix::fs::PermissionsExt;

        let directory = std::env::temp_dir().join(format!(
            "rc-private-db-{}-{}",
            std::process::id(),
            rand::random::<u64>()
        ));
        std::fs::create_dir_all(&directory)?;
        let path = directory.join("rc-v2.sqlite3");
        let database = Database::open(&path)?;
        assert_eq!(
            std::fs::metadata(&path)?.permissions().mode() & 0o777,
            0o600
        );
        drop(database);
        std::fs::remove_dir_all(directory)?;
        Ok(())
    }
}
