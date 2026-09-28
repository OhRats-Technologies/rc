//! Store only owner-signed authority transitions; reconnect never manufactures authority.
use crate::{Database, NodeHub, authority_hash, now_ms};
use rc_protocol::{ControlGrant, ServerToNode};
use rusqlite::{OptionalExtension, params};

pub(crate) fn parents(db: &Database, workspace: &str) -> anyhow::Result<Vec<(String, i64)>> {
    Ok(db.with_connection(|db| {
        db.execute("DELETE FROM authority_deliveries WHERE expires_at<>0 AND expires_at<=?", [now_ms()])?;
        let mut query = db.prepare("SELECT DISTINCT q.next_hash,q.generation+1 FROM authority_deliveries q JOIN devices d ON d.id=q.device_id WHERE d.workspace_id=?")?;
        query.query_map([workspace], |row| Ok((row.get(0)?, row.get(1)?)))?.collect()
    })?)
}

pub(crate) fn save(db: &Database, device: &str, message: &ServerToNode) -> anyhow::Result<()> {
    let ServerToNode::LockSync {
        snapshot,
        previous_hash,
        previous_generation,
        grant,
        ..
    } = message
    else {
        anyhow::bail!("only signed authority transitions can be queued");
    };
    let proof: ControlGrant = serde_json::from_str(grant)?;
    let encoded = serde_json::to_string(message)?;
    db.with_connection(|db| {
        db.execute("INSERT INTO authority_deliveries(device_id,previous_hash,generation,next_hash,message,expires_at) VALUES(?,?,?,?,?,?) ON CONFLICT(device_id,previous_hash,generation) DO UPDATE SET next_hash=excluded.next_hash,message=excluded.message,expires_at=excluded.expires_at",
            params![device,previous_hash,*previous_generation as i64,authority_hash(snapshot),encoded,proof.expires_at])?;
        Ok(())
    })?;
    Ok(())
}

pub(crate) fn device_parents(db: &Database, device: &str) -> anyhow::Result<Vec<(String, i64)>> {
    Ok(db.with_connection(|db| {
        let mut query = db.prepare("SELECT next_hash,generation+1 FROM authority_deliveries WHERE device_id=? AND (expires_at=0 OR expires_at>?)")?;
        query.query_map(params![device,now_ms()], |row| Ok((row.get(0)?, row.get(1)?)))?.collect()
    })?)
}

pub(crate) fn pending(db: &Database, device: &str) -> anyhow::Result<Option<ServerToNode>> {
    let message: Option<String> = db.with_connection(|db| {
        // Only discard transitions the Node has already passed. Future edges may
        // be needed after it acknowledges an intermediate signed snapshot.
        db.execute("DELETE FROM authority_deliveries WHERE device_id=? AND (generation<(SELECT lock_generation FROM devices WHERE id=?) OR (generation=(SELECT lock_generation FROM devices WHERE id=?) AND previous_hash<>(SELECT lock_hash FROM devices WHERE id=?)))",
            params![device,device,device,device])?;
        db.query_row("SELECT q.message FROM authority_deliveries q JOIN devices d ON d.id=q.device_id WHERE d.id=? AND q.previous_hash=d.lock_hash AND q.generation=d.lock_generation AND (q.expires_at=0 OR q.expires_at>?)",
            params![device,now_ms()], |row| row.get(0)).optional()
    })?;
    message
        .map(|value| serde_json::from_str(&value))
        .transpose()
        .map_err(Into::into)
}

pub(crate) async fn deliver(nodes: &NodeHub, db: &Database, device: &str) -> anyhow::Result<bool> {
    let Some(message) = pending(db, device)? else {
        return Ok(false);
    };
    Ok(nodes.send(device, &message).await.is_ok())
}

#[cfg(test)]
mod tests;
