use crate::{Database, now_ms, passkey_authority_material};
use rc_protocol::{
    AuthorityApiKey, AuthorityCredential, AuthorityMcpGrant, AuthorityMember, AuthoritySnapshot,
};
use sha2::{Digest, Sha256};
use std::collections::{BTreeMap, BTreeSet};
use webauthn_rs::prelude::Passkey;

pub fn authority_snapshot(db: &Database, workspace_id: &str) -> anyhow::Result<AuthoritySnapshot> {
    let members = authority_members(db, workspace_id)?;
    let api_keys = authority_api_keys(db, workspace_id)?;
    let mcp_grants = authority_mcp_grants(db, workspace_id)?;
    Ok(AuthoritySnapshot {
        v: 1,
        workspace_id: workspace_id.to_owned(),
        members,
        api_keys,
        mcp_grants,
    })
}

pub fn canonical_authority(db: &Database, workspace_id: &str) -> anyhow::Result<String> {
    Ok(serde_json::to_string(&authority_snapshot(
        db,
        workspace_id,
    )?)?)
}

pub fn authority_hash(snapshot: &str) -> String {
    Sha256::digest(snapshot.as_bytes())
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}

pub fn bootstrap_snapshot_for_device(
    db: &Database,
    device_id: &str,
) -> anyhow::Result<Option<String>> {
    let workspace = db.with_connection(|db| {
        use rusqlite::OptionalExtension;
        db.query_row(
            "SELECT workspace_id FROM devices WHERE id=?",
            [device_id],
            |row| row.get::<_, String>(0),
        )
        .optional()
    })?;
    workspace
        .map(|workspace_id| canonical_authority(db, &workspace_id))
        .transpose()
}

fn authority_members(db: &Database, workspace_id: &str) -> anyhow::Result<Vec<AuthorityMember>> {
    let rows = db.with_connection(|db| {
        let mut statement = db.prepare(
            "SELECT wm.user_id,wm.role,p.credential_json FROM workspace_members wm LEFT JOIN passkeys p ON p.user_id=wm.user_id WHERE wm.workspace_id=? ORDER BY wm.user_id,p.id",
        )?;
        statement
            .query_map([workspace_id], |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, Option<String>>(2)?,
                ))
            })?
            .collect::<Result<Vec<_>, _>>()
    })?;
    let mut grouped = BTreeMap::<String, AuthorityMember>::new();
    for (user_id, role, credential_json) in rows {
        let member = grouped
            .entry(user_id.clone())
            .or_insert_with(|| AuthorityMember {
                user_id,
                role,
                credentials: Vec::new(),
            });
        if let Some(json) = credential_json {
            let passkey: Passkey = serde_json::from_str(&json)?;
            let (id, public_key) = passkey_authority_material(&passkey)?;
            member
                .credentials
                .push(AuthorityCredential { id, public_key });
        }
    }
    Ok(grouped.into_values().collect())
}

fn authority_api_keys(db: &Database, workspace_id: &str) -> anyhow::Result<Vec<AuthorityApiKey>> {
    let now = now_ms();
    Ok(db.with_connection(|db| {
        let mut statement = db.prepare(
            "SELECT c.id,c.user_id,c.public_key,c.scopes,c.expires_at FROM clients c JOIN workspace_members wm ON wm.user_id=c.user_id WHERE wm.workspace_id=? AND c.kind='api' AND c.public_key<>'' AND (c.expires_at=0 OR c.expires_at>?) ORDER BY c.id",
        )?;
        statement
            .query_map(rusqlite::params![workspace_id, now], |row| {
                let scopes: String = row.get(3)?;
                Ok(AuthorityApiKey {
                    id: row.get(0)?,
                    user_id: row.get(1)?,
                    public_key: row.get(2)?,
                    scopes: serde_json::from_str(&scopes).unwrap_or_default(),
                    expires_at: row.get(4)?,
                })
            })?
            .collect::<Result<Vec<_>, _>>()
    })?)
}

fn authority_mcp_grants(
    db: &Database,
    workspace_id: &str,
) -> anyhow::Result<Vec<AuthorityMcpGrant>> {
    let devices = db.with_connection(|db| {
        let mut statement =
            db.prepare("SELECT id FROM devices WHERE workspace_id=? ORDER BY id")?;
        statement
            .query_map([workspace_id], |row| row.get::<_, String>(0))?
            .collect::<Result<BTreeSet<_>, _>>()
    })?;
    if devices.is_empty() {
        return Ok(Vec::new());
    }
    let rows = db.with_connection(|db| {
        let mut statement = db.prepare(
            "SELECT id,user_id,grant FROM mcp_grants WHERE revoked_at IS NULL AND (expires_at=0 OR expires_at>?) ORDER BY id",
        )?;
        statement
            .query_map([now_ms()], |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                ))
            })?
            .collect::<Result<Vec<_>, _>>()
    })?;
    let mut grants = Vec::new();
    for (id, user_id, grant) in rows {
        let value: serde_json::Value = serde_json::from_str(&grant)?;
        let terminal = value
            .get("scopes")
            .and_then(|value| value.as_array())
            .is_some_and(|scopes| {
                scopes
                    .iter()
                    .any(|scope| scope.as_str() == Some("mcp:terminal"))
            });
        let applies = value
            .get("deviceIds")
            .and_then(|value| value.as_array())
            .is_some_and(|ids| {
                ids.iter()
                    .filter_map(|id| id.as_str())
                    .any(|id| devices.contains(id))
            });
        let owner = db.with_connection(|db| {
            use rusqlite::OptionalExtension;
            db.query_row(
                "SELECT 1 FROM workspace_members WHERE workspace_id=? AND user_id=? AND role='owner'",
                rusqlite::params![workspace_id, user_id],
                |_| Ok(()),
            )
            .optional()
            .map(|value| value.is_some())
        })?;
        if terminal && owner && applies {
            grants.push(AuthorityMcpGrant {
                id,
                user_id,
                hash: authority_hash(&grant),
            });
        }
    }
    Ok(grants)
}
