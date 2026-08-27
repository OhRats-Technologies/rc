use super::principal;
use crate::auth_public_routes::ApiError;
use crate::{AppState, control_proof, now_ms, verify_control_client_signature};
use axum::{
    Json,
    body::Bytes,
    extract::{Path, Query, State},
    http::{HeaderMap, Method, StatusCode},
    response::{IntoResponse, Response},
};
use base64::{Engine as _, engine::general_purpose::STANDARD};
use rusqlite::OptionalExtension;
use serde::Deserialize;
use uuid::Uuid;

const MAX_SSH_KEYS: i64 = 20;

pub(super) async fn list(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let principal = principal(
        &state,
        &headers,
        &Method::GET,
        "/api/v1/ssh/keys",
        &[],
        None,
    )?;
    human(&principal)?;
    let keys = state.db.with_connection(|db| {
        let mut statement = db.prepare(
            "SELECT id,name,algorithm,public_key,created_at,last_used \
             FROM ssh_keys WHERE user_id=? ORDER BY created_at DESC",
        )?;
        statement
            .query_map([principal.user.id], |row| {
                Ok(serde_json::json!({
                    "id": row.get::<_, String>(0)?,
                    "name": row.get::<_, String>(1)?,
                    "algorithm": row.get::<_, String>(2)?,
                    "public_key": row.get::<_, String>(3)?,
                    "created_at": row.get::<_, i64>(4)?,
                    "last_used": row.get::<_, Option<i64>>(5)?,
                }))
            })?
            .collect::<Result<Vec<_>, _>>()
    })?;
    Ok(Json(serde_json::json!({"keys": keys})))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(super) struct CreateKey {
    name: Option<String>,
    public_key: String,
    client_id: String,
    signature: String,
}
pub(super) async fn create(
    State(state): State<AppState>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<(StatusCode, Json<serde_json::Value>), ApiError> {
    let principal = principal(
        &state,
        &headers,
        &Method::POST,
        "/api/v1/ssh/keys",
        &body,
        None,
    )?;
    human(&principal)?;
    let input: CreateKey =
        serde_json::from_slice(&body).map_err(|_| ApiError::bad_request("invalid request"))?;
    let (algorithm, key_data, normalized) = normalize_key(&input.public_key)?;
    if control_proof(&state, &principal.user.id, &input.client_id)?.is_none() {
        return Err(ApiError::unauthorized(
            "active passkey-backed control authorization required",
        ));
    }
    let payload = format!(
        "rc-ssh-key-v1\n{}\n{}",
        input.client_id,
        input.public_key.trim()
    );
    if !verify_control_client_signature(
        &state,
        &principal.user.id,
        &input.client_id,
        &payload,
        &input.signature,
    )? {
        return Err(ApiError::unauthorized("invalid SSH key authorization"));
    }
    let count = state.db.with_connection(|db| {
        db.query_row(
            "SELECT count(*) FROM ssh_keys WHERE user_id=?",
            [&principal.user.id],
            |row| row.get::<_, i64>(0),
        )
    })?;
    if count >= MAX_SSH_KEYS {
        return Err(ApiError::conflict("SSH key limit reached"));
    }
    let id = Uuid::new_v4().to_string();
    let name = input
        .name
        .unwrap_or_else(|| "SSH key".into())
        .trim()
        .chars()
        .take(80)
        .collect::<String>();
    let name = if name.is_empty() {
        "SSH key".to_owned()
    } else {
        name
    };
    let created_at = now_ms();
    let inserted = state.db.with_connection(|db| {
        db.execute(
            "INSERT INTO ssh_keys(id,user_id,name,algorithm,key_data,public_key,client_id,created_at) \
             VALUES(?,?,?,?,?,?,?,?)",
            rusqlite::params![
                id,
                principal.user.id,
                name,
                algorithm,
                key_data,
                normalized,
                input.client_id,
                created_at,
            ],
        )
    });
    match inserted {
        Ok(_) => {}
        Err(rusqlite::Error::SqliteFailure(code, _))
            if code.code == rusqlite::ErrorCode::ConstraintViolation =>
        {
            return Err(ApiError::conflict("SSH key already registered"));
        }
        Err(error) => return Err(error.into()),
    }
    Ok((
        StatusCode::CREATED,
        Json(serde_json::json!({
            "id": id,
            "name": name,
            "algorithm": algorithm,
            "publicKey": normalized,
            "createdAt": created_at,
        })),
    ))
}

pub(super) async fn remove(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/ssh/keys/{id}");
    let principal = principal(&state, &headers, &Method::DELETE, &path, &[], None)?;
    human(&principal)?;
    if state.db.with_connection(|db| {
        db.execute(
            "DELETE FROM ssh_keys WHERE id=? AND user_id=?",
            rusqlite::params![id, principal.user.id],
        )
    })? == 0
    {
        return Err(ApiError::not_found("SSH key not found"));
    }
    Ok(Json(serde_json::json!({"ok": true})))
}

#[derive(Deserialize)]
pub(super) struct AuthorizedQuery {
    r#type: String,
    key: String,
}
pub(super) async fn authorized(
    State(state): State<AppState>,
    Query(query): Query<AuthorizedQuery>,
) -> Response {
    let row = state.db.with_connection(|db| {
        db.query_row(
            "SELECT k.id,k.algorithm,k.key_data FROM ssh_keys k \
             JOIN clients c ON c.id=k.client_id \
             WHERE k.algorithm=? AND k.key_data=? \
               AND (c.expires_at=0 OR c.expires_at>?)",
            rusqlite::params![query.r#type, query.key, now_ms()],
            |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                ))
            },
        )
        .optional()
    });
    let row = match row {
        Ok(row) => row,
        Err(error) => {
            tracing::error!(%error, "SSH authorized-key lookup failed");
            return StatusCode::INTERNAL_SERVER_ERROR.into_response();
        }
    };
    let Some((id, algorithm, key)) = row else {
        return StatusCode::NOT_FOUND.into_response();
    };
    let options = format!(
        "no-agent-forwarding,no-port-forwarding,no-X11-forwarding,no-user-rc,command=\"/usr/local/bin/rc-ssh-bridge {id}\""
    );
    (
        [("content-type", "text/plain")],
        format!("{options} {algorithm} {key}\n"),
    )
        .into_response()
}

fn human(principal: &crate::AuthPrincipal) -> Result<(), ApiError> {
    if !principal.browser
        && principal
            .client
            .as_ref()
            .is_none_or(|client| client.kind != "cli")
    {
        Err(ApiError::unauthorized("human authorization required"))
    } else {
        Ok(())
    }
}
fn normalize_key(input: &str) -> Result<(String, String, String), ApiError> {
    let input = input.trim();
    if input.is_empty() || input.len() > 16384 {
        return Err(ApiError::bad_request("invalid SSH public key"));
    }
    let mut parts = input.split_whitespace();
    let algorithm = parts.next().unwrap_or_default();
    let data = parts.next().unwrap_or_default();
    if !(algorithm.starts_with("ssh-")
        || algorithm.starts_with("ecdsa-")
        || algorithm.starts_with("sk-"))
    {
        return Err(ApiError::bad_request("invalid SSH public key"));
    }
    let bytes = STANDARD
        .decode(data)
        .map_err(|_| ApiError::bad_request("invalid SSH public key"))?;
    if bytes.len() < 16 || embedded_algorithm(&bytes) != Some(algorithm) {
        return Err(ApiError::bad_request("invalid SSH public key"));
    }
    Ok((
        algorithm.to_owned(),
        data.to_owned(),
        format!("{algorithm} {data}"),
    ))
}

fn embedded_algorithm(blob: &[u8]) -> Option<&str> {
    let length = u32::from_be_bytes(blob.get(..4)?.try_into().ok()?) as usize;
    let algorithm = blob.get(4..4_usize.checked_add(length)?)?;
    std::str::from_utf8(algorithm).ok()
}

#[cfg(test)]
mod tests {
    use super::normalize_key;
    use base64::{Engine as _, engine::general_purpose::STANDARD};

    fn public_key(label: &str, embedded: &str) -> String {
        let mut blob = Vec::new();
        blob.extend_from_slice(&(embedded.len() as u32).to_be_bytes());
        blob.extend_from_slice(embedded.as_bytes());
        blob.extend_from_slice(&[7; 32]);
        format!("{label} {} comment", STANDARD.encode(blob))
    }

    #[test]
    fn normalizes_a_well_formed_ssh_key() {
        let input = public_key("ssh-ed25519", "ssh-ed25519");
        let (algorithm, _, normalized) = normalize_key(&input).expect("valid key");
        assert_eq!(algorithm, "ssh-ed25519");
        assert!(!normalized.contains("comment"));
    }

    #[test]
    fn rejects_a_mismatched_embedded_algorithm() {
        let input = public_key("ssh-ed25519", "ssh-rsa");
        assert!(normalize_key(&input).is_err());
    }
}
