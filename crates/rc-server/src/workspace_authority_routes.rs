use crate::auth_public_routes::ApiError;
use crate::{
    AppState, authority_hash, canonical_authority, control_proof, now_ms, require_principal,
    verify_control_client_signature, workspace_role,
};
use axum::{
    Json, Router,
    body::Bytes,
    extract::{Path, State},
    http::{HeaderMap, Method},
    routing::{get, post},
};
use rc_protocol::ServerToNode;
use serde::Deserialize;
use std::collections::{BTreeMap, BTreeSet};

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/workspaces/{id}/authority", get(status))
        .route("/api/v1/workspaces/{id}/authority/sync", post(sync))
}

async fn status(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/workspaces/{id}/authority");
    let p = principal(&state, &headers, &Method::GET, &path, &[], Some("read"))?;
    if workspace_role(&state, &p.user.id, &id)?.is_none() {
        return Err(ApiError::not_found("workspace not found"));
    }
    let snapshot = canonical_authority(&state.db, &id)?;
    let hash = authority_hash(&snapshot);
    let devices = state.db.with_connection(|db| {
        let mut s =
            db.prepare("SELECT lock_hash,lock_generation FROM devices WHERE workspace_id=?")?;
        s.query_map([id], |r| Ok((r.get::<_, String>(0)?, r.get::<_, i64>(1)?)))?
            .collect::<Result<Vec<_>, _>>()
    })?;
    let synced = devices.iter().filter(|(h, _)| h == &hash).count();
    let mut parents = BTreeMap::new();
    for (h, g) in &devices {
        let h = h.to_lowercase();
        if h.len() == 64 && h.chars().all(|c| c.is_ascii_hexdigit()) && h != hash {
            parents.insert(
                format!("{g}:{h}"),
                serde_json::json!({"hash":h,"generation":g}),
            );
        }
    }
    Ok(Json(
        serde_json::json!({"hash":hash,"devices":devices.len(),"synced":synced,"parents":parents.into_values().collect::<Vec<_>>()}),
    ))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct SyncInput {
    client_id: String,
    transitions: Vec<Transition>,
}
#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Transition {
    from_hash: String,
    generation: u64,
    signature: String,
}

async fn sync(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/workspaces/{id}/authority/sync");
    let p = principal(&state, &headers, &Method::POST, &path, &body, None)?;
    if workspace_role(&state, &p.user.id, &id)?.as_deref() != Some("owner") {
        return Err(ApiError::forbidden("owner required"));
    }
    let input: SyncInput =
        serde_json::from_slice(&body).map_err(|_| ApiError::bad_request("invalid request"))?;
    if input.transitions.is_empty() || input.transitions.len() > 100 {
        return Err(ApiError::bad_request("invalid authority transition"));
    }
    let proof = control_proof(&state, &p.user.id, &input.client_id)?
        .ok_or_else(|| ApiError::unauthorized("control client authorization expired"))?;
    let snapshot = canonical_authority(&state.db, &id)?;
    let digest = authority_hash(&snapshot);
    let mut signatures = BTreeMap::new();
    let mut seen = BTreeSet::new();
    for t in input.transitions {
        let from = t.from_hash.to_lowercase();
        let key = format!("{}:{}", t.generation, from);
        if from.len() != 64
            || !from.chars().all(|c| c.is_ascii_hexdigit())
            || !seen.insert(key.clone())
        {
            return Err(ApiError::bad_request("invalid authority transition"));
        }
        let payload = format!("rc-authority-v3\n{}\n{}\n{}", t.generation, from, digest);
        if !verify_control_client_signature(
            &state,
            &p.user.id,
            &input.client_id,
            &payload,
            &t.signature,
        )? {
            return Err(ApiError::forbidden(
                "invalid authority transition signature",
            ));
        }
        signatures.insert(key, t.signature);
    }
    let devices = state.db.with_connection(|db| {
        let mut s =
            db.prepare("SELECT id,lock_hash,lock_generation FROM devices WHERE workspace_id=?")?;
        s.query_map([&id], |r| {
            Ok((
                r.get::<_, String>(0)?,
                r.get::<_, String>(1)?,
                r.get::<_, i64>(2)?,
            ))
        })?
        .collect::<Result<Vec<_>, _>>()
    })?;
    let mut online = 0usize;
    for (device, previous_hash, generation) in &devices {
        let key = format!("{}:{}", generation, previous_hash.to_lowercase());
        let Some(signature) = signatures.get(&key) else {
            continue;
        };
        if state
            .nodes
            .send(
                device,
                &ServerToNode::LockSync {
                    snapshot: snapshot.clone(),
                    previous_hash: previous_hash.clone(),
                    previous_generation: *generation as u64,
                    grant: proof.grant.clone(),
                    credential_id: proof.credential_id.clone(),
                    assertion: proof.assertion.clone(),
                    signature: signature.clone(),
                },
            )
            .await
            .is_ok()
        {
            online += 1;
        }
    }
    Ok(Json(
        serde_json::json!({"ok":true,"devices":devices.len(),"online":online,"lockHash":digest,"at":now_ms()}),
    ))
}
fn principal(
    state: &AppState,
    h: &HeaderMap,
    m: &Method,
    path: &str,
    b: &[u8],
    scope: Option<&'static str>,
) -> Result<crate::AuthPrincipal, ApiError> {
    require_principal(state, h, m, path, b, scope).map_err(|e| ApiError(e.status(), e.to_string()))
}
