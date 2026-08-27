use super::{Database, now_ms};
use rusqlite::{OptionalExtension, params};

impl Database {
    pub fn device_public_key(&self, id: &str) -> rusqlite::Result<Option<String>> {
        self.lock()?
            .query_row(
                "SELECT identity_public_key FROM devices WHERE id=?",
                [id],
                |row| row.get(0),
            )
            .optional()
    }

    pub fn revoked_public_key(&self, id: &str) -> rusqlite::Result<Option<String>> {
        self.lock()?
            .query_row(
                "SELECT identity_public_key FROM revoked_devices WHERE id=?",
                [id],
                |row| row.get(0),
            )
            .optional()
    }

    pub fn enroll_device(
        &self,
        token_hash: &str,
        device: &EnrollmentDevice,
    ) -> rusqlite::Result<EnrollmentInsert> {
        let mut db = self.lock()?;
        let tx = db.transaction()?;
        let now = now_ms();
        let workspace: Option<String> = tx
            .query_row(
                "SELECT workspace_id FROM enrollment_tokens WHERE token_hash=? AND used_at IS NULL AND expires_at>?",
                params![token_hash, now],
                |row| row.get(0),
            )
            .optional()?;
        let Some(workspace) = workspace else {
            tx.commit()?;
            return Ok(EnrollmentInsert::Invalid);
        };
        let count: i64 = tx.query_row(
            "SELECT count(*) FROM devices WHERE workspace_id=?",
            [&workspace],
            |row| row.get(0),
        )?;
        if count >= 25 {
            tx.commit()?;
            return Ok(EnrollmentInsert::DeviceLimit);
        }
        tx.execute(
            "INSERT INTO devices(id,workspace_id,name,hostname,platform,arch,identity_public_key,transport_public_key,version,capabilities,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)",
            params![device.id, workspace, device.name, device.hostname, device.platform, device.arch, device.identity_public_key,
                device.transport_public_key, device.version, serde_json::to_string(&device.capabilities).unwrap_or_else(|_| "[]".into()), now],
        )?;
        if tx.execute(
            "UPDATE enrollment_tokens SET used_at=? WHERE token_hash=? AND used_at IS NULL AND expires_at>?",
            params![now, token_hash, now],
        )? != 1
        {
            return Err(rusqlite::Error::InvalidQuery);
        }
        tx.commit()?;
        Ok(EnrollmentInsert::Inserted {
            workspace_id: workspace,
        })
    }

    pub fn insert_device(&self, device: &NewDevice) -> rusqlite::Result<()> {
        self.lock()?.execute(
            "INSERT INTO devices(id,workspace_id,name,hostname,platform,arch,identity_public_key,transport_public_key,version,capabilities,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)",
            params![device.id, device.workspace_id, device.name, device.hostname, device.platform, device.arch, device.identity_public_key,
                device.transport_public_key, device.version, serde_json::to_string(&device.capabilities).unwrap_or_else(|_| "[]".into()), now_ms()],
        )?;
        Ok(())
    }

    pub fn node_status(&self, id: &str) -> rusqlite::Result<Option<NodeStatusRow>> {
        self.lock()?
            .query_row("SELECT name,version FROM devices WHERE id=?", [id], |row| {
                Ok(NodeStatusRow {
                    name: row.get(0)?,
                    version: row.get(1)?,
                })
            })
            .optional()
    }

    pub fn device_workspace(&self, id: &str) -> rusqlite::Result<Option<String>> {
        self.lock()?
            .query_row("SELECT workspace_id FROM devices WHERE id=?", [id], |row| {
                row.get(0)
            })
            .optional()
    }

    pub fn revoke_device(&self, id: &str) -> rusqlite::Result<bool> {
        let mut db = self.lock()?;
        let tx = db.transaction()?;
        let key: Option<String> = tx
            .query_row(
                "SELECT identity_public_key FROM devices WHERE id=?",
                [id],
                |row| row.get(0),
            )
            .optional()?;
        let Some(key) = key else {
            tx.commit()?;
            return Ok(false);
        };
        tx.execute(
            "INSERT OR REPLACE INTO revoked_devices(id,identity_public_key,revoked_at) VALUES(?,?,?)",
            params![id, key, now_ms()],
        )?;
        tx.execute("DELETE FROM devices WHERE id=?", [id])?;
        tx.commit()?;
        Ok(true)
    }

    pub fn touch_node(
        &self,
        id: &str,
        hello: &rc_protocol::NodeHello,
    ) -> rusqlite::Result<Option<NodeTouch>> {
        let mut db = self.lock()?;
        let tx = db.transaction()?;
        let row: Option<(String, String)> = tx
            .query_row(
                "SELECT workspace_id,version FROM devices WHERE id=?",
                [id],
                |row| Ok((row.get(0)?, row.get(1)?)),
            )
            .optional()?;
        let Some((workspace_id, previous_version)) = row else {
            tx.commit()?;
            return Ok(None);
        };
        tx.execute(
            "UPDATE devices SET hostname=?,platform=?,arch=?,version=?,capabilities=?,transport_public_key=?,lock_hash=?,lock_generation=?,last_seen=? WHERE id=?",
            params![hello.hostname, hello.platform, hello.arch, hello.version, serde_json::to_string(&hello.capabilities).unwrap_or_else(|_| "[]".into()),
                hello.transport_public_key, hello.lock_hash, hello.lock_generation as i64, now_ms(), id],
        )?;
        tx.commit()?;
        Ok(Some(NodeTouch {
            workspace_id,
            version_changed: previous_version != hello.version,
        }))
    }

    pub fn mark_lock_state(&self, id: &str, hash: &str, generation: u64) -> rusqlite::Result<()> {
        self.lock()?.execute(
            "UPDATE devices SET lock_hash=?,lock_generation=? WHERE id=?",
            params![hash, generation as i64, id],
        )?;
        Ok(())
    }
}

#[derive(Debug, Clone)]
pub struct NewDevice {
    pub id: String,
    pub workspace_id: String,
    pub name: String,
    pub hostname: String,
    pub platform: String,
    pub arch: String,
    pub identity_public_key: String,
    pub transport_public_key: String,
    pub version: String,
    pub capabilities: Vec<String>,
}

#[derive(Debug, Clone)]
pub struct EnrollmentDevice {
    pub id: String,
    pub name: String,
    pub hostname: String,
    pub platform: String,
    pub arch: String,
    pub identity_public_key: String,
    pub transport_public_key: String,
    pub version: String,
    pub capabilities: Vec<String>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum EnrollmentInsert {
    Inserted { workspace_id: String },
    Invalid,
    DeviceLimit,
}

#[derive(Debug, Clone)]
pub struct NodeStatusRow {
    pub name: String,
    pub version: String,
}

#[derive(Debug, Clone)]
pub struct NodeTouch {
    pub workspace_id: String,
    pub version_changed: bool,
}
