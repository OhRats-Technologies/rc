use crate::{
    bindings::ohrats::rc_storage::{
        durable_store::Host,
        types::{Change, CommitError, Entry, Page},
    },
    database::Database,
    host::HostState,
};
mod error;
use error::CommitFailure;
use rusqlite::{OptionalExtension, TransactionBehavior, params};
use std::collections::BTreeSet;

const MAX_BUCKET_BYTES: usize = 128;
const MAX_KEY_BYTES: usize = 16 * 1024;
const MAX_VALUE_BYTES: usize = 4 * 1024 * 1024;
const MAX_TRANSACTION_BYTES: usize = 8 * 1024 * 1024;
const MAX_CHANGES: usize = 1024;
const MAX_SCAN: u32 = 1000;

impl Host for HostState {
    fn revision(&mut self) -> Result<u64, String> {
        revision(&self.environment.database, self.plugin_id()).map_err(display)
    }

    fn get(&mut self, bucket: String, key: Vec<u8>) -> Result<Option<Vec<u8>>, String> {
        get(&self.environment.database, self.plugin_id(), &bucket, &key).map_err(display)
    }

    fn scan(
        &mut self,
        bucket: String,
        prefix: Vec<u8>,
        after: Option<Vec<u8>>,
        limit: u32,
    ) -> Result<Page, String> {
        scan(
            &self.environment.database,
            self.plugin_id(),
            &bucket,
            &prefix,
            after.as_deref(),
            limit,
        )
        .map_err(display)
    }

    fn commit(&mut self, expected_revision: u64, changes: Vec<Change>) -> Result<u64, CommitError> {
        commit(
            &self.environment.database,
            self.plugin_id(),
            expected_revision,
            changes,
        )
    }
}

fn revision(database: &Database, owner: &str) -> anyhow::Result<u64> {
    validate_owner(owner)?;
    database
        .with_connection(|connection| query_revision(connection, owner))
        .map_err(Into::into)
}

fn get(
    database: &Database,
    owner: &str,
    bucket: &str,
    key: &[u8],
) -> anyhow::Result<Option<Vec<u8>>> {
    validate_owner(owner)?;
    validate_bucket(bucket)?;
    validate_key(key)?;
    database
        .with_connection(|connection| {
            connection
                .query_row(
                    "SELECT value FROM rc_component_entries
                     WHERE owner=? AND bucket=? AND key=?",
                    params![owner, bucket, key],
                    |row| row.get(0),
                )
                .optional()
        })
        .map_err(Into::into)
}

fn scan(
    database: &Database,
    owner: &str,
    bucket: &str,
    prefix: &[u8],
    after: Option<&[u8]>,
    limit: u32,
) -> anyhow::Result<Page> {
    validate_owner(owner)?;
    validate_bucket(bucket)?;
    validate_cursor(prefix, "prefix")?;
    if let Some(after) = after {
        validate_cursor(after, "after key")?;
    }
    anyhow::ensure!(
        (1..=MAX_SCAN).contains(&limit),
        "scan limit must be 1 to {MAX_SCAN}"
    );
    database
        .with_connection(|connection| {
            let revision = query_revision(connection, owner)?;
            let mut statement = connection.prepare(
                "SELECT key,value FROM rc_component_entries
             WHERE owner=?1 AND bucket=?2
               AND key > coalesce(?3, X'')
               AND substr(key,1,?4)=?5
             ORDER BY key LIMIT ?6",
            )?;
            let rows = statement.query_map(
                params![
                    owner,
                    bucket,
                    after,
                    prefix.len() as i64,
                    prefix,
                    i64::from(limit) + 1
                ],
                |row| {
                    Ok(Entry {
                        bucket: bucket.to_owned(),
                        key: row.get(0)?,
                        value: row.get(1)?,
                    })
                },
            )?;
            let mut entries = rows.collect::<rusqlite::Result<Vec<_>>>()?;
            let next_key = if entries.len() > limit as usize {
                entries.pop();
                entries.last().map(|entry| entry.key.clone())
            } else {
                None
            };
            Ok(Page {
                revision,
                entries,
                next_key,
            })
        })
        .map_err(Into::into)
}

fn commit(
    database: &Database,
    owner: &str,
    expected_revision: u64,
    changes: Vec<Change>,
) -> Result<u64, CommitError> {
    validate_changes(owner, &changes).map_err(failure)?;
    let expected = i64::try_from(expected_revision)
        .map_err(|_| failure("expected revision exceeds SQLite range"))?;
    database
        .with_connection_mut_fallible(|connection| {
            let transaction =
                connection.transaction_with_behavior(TransactionBehavior::Immediate)?;
            let current = query_revision(&transaction, owner)?;
            if current != expected_revision {
                return Err(CommitFailure::Conflict(current));
            }
            for change in changes {
                match change {
                    Change::Put(entry) => {
                        transaction.execute(
                            "INSERT INTO rc_component_entries(owner,bucket,key,value)
                             VALUES(?,?,?,?)
                             ON CONFLICT(owner,bucket,key) DO UPDATE SET value=excluded.value",
                            params![owner, entry.bucket, entry.key, entry.value],
                        )?;
                    }
                    Change::Delete(deletion) => {
                        transaction.execute(
                            "DELETE FROM rc_component_entries
                             WHERE owner=? AND bucket=? AND key=?",
                            params![owner, deletion.bucket, deletion.key],
                        )?;
                    }
                }
            }
            let next = expected
                .checked_add(1)
                .ok_or(rusqlite::Error::InvalidQuery)?;
            transaction.execute(
                "INSERT INTO rc_component_revisions(owner,revision) VALUES(?,?)
                 ON CONFLICT(owner) DO UPDATE SET revision=excluded.revision",
                params![owner, next],
            )?;
            transaction.commit()?;
            Ok(next as u64)
        })
        .map_err(|error| match error {
            CommitFailure::Conflict(current) => CommitError::Conflict(current),
            CommitFailure::Database(error) => CommitError::Failure(error.to_string()),
        })
}

fn query_revision(connection: &rusqlite::Connection, owner: &str) -> rusqlite::Result<u64> {
    let value = connection
        .query_row(
            "SELECT revision FROM rc_component_revisions WHERE owner=?",
            [owner],
            |row| row.get::<_, i64>(0),
        )
        .optional()?
        .unwrap_or(0);
    u64::try_from(value).map_err(|_| rusqlite::Error::IntegralValueOutOfRange(0, value))
}

fn validate_changes(owner: &str, changes: &[Change]) -> anyhow::Result<()> {
    validate_owner(owner)?;
    anyhow::ensure!(
        !changes.is_empty(),
        "durable commit requires at least one change"
    );
    anyhow::ensure!(
        changes.len() <= MAX_CHANGES,
        "durable commit exceeds {MAX_CHANGES} changes"
    );
    let mut keys = BTreeSet::new();
    let mut bytes = 0usize;
    for change in changes {
        let (bucket, key, value_bytes) = match change {
            Change::Put(entry) => (&entry.bucket, entry.key.as_slice(), entry.value.len()),
            Change::Delete(deletion) => (&deletion.bucket, deletion.key.as_slice(), 0),
        };
        validate_bucket(bucket)?;
        validate_key(key)?;
        anyhow::ensure!(
            value_bytes <= MAX_VALUE_BYTES,
            "durable value exceeds 4 MiB"
        );
        anyhow::ensure!(
            keys.insert((bucket.clone(), key.to_vec())),
            "duplicate durable key in one commit"
        );
        bytes = bytes
            .checked_add(bucket.len() + key.len() + value_bytes)
            .ok_or_else(|| anyhow::anyhow!("durable commit size overflow"))?;
    }
    anyhow::ensure!(
        bytes <= MAX_TRANSACTION_BYTES,
        "durable commit exceeds 8 MiB"
    );
    Ok(())
}

fn validate_owner(value: &str) -> anyhow::Result<()> {
    anyhow::ensure!(
        !value.is_empty() && value.len() <= 128,
        "invalid durable owner"
    );
    Ok(())
}

fn validate_bucket(value: &str) -> anyhow::Result<()> {
    anyhow::ensure!(
        !value.is_empty()
            && value.len() <= MAX_BUCKET_BYTES
            && !value.chars().any(char::is_control),
        "invalid durable bucket"
    );
    Ok(())
}

fn validate_key(value: &[u8]) -> anyhow::Result<()> {
    anyhow::ensure!(
        !value.is_empty() && value.len() <= MAX_KEY_BYTES,
        "invalid durable key"
    );
    Ok(())
}

fn validate_cursor(value: &[u8], label: &str) -> anyhow::Result<()> {
    anyhow::ensure!(value.len() <= MAX_KEY_BYTES, "invalid durable {label}");
    Ok(())
}

fn failure(error: impl std::fmt::Display) -> CommitError {
    CommitError::Failure(error.to_string())
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}

#[cfg(test)]
mod tests;
