use super::require_browser;
use crate::auth_public_routes::ApiError;
use crate::{
    AppState, RegistrationKind, consume_step_up, finish_registration, insert_passkey,
    recent_browser_session, start_registration,
};
use axum::{
    Json,
    extract::{Path, State},
    http::{HeaderMap, StatusCode},
};
use base64::Engine as _;
use rusqlite::OptionalExtension;
use serde::Deserialize;

pub(super) async fn list(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let user = require_browser(&state, &headers)?;
    let rows = state.db.with_connection(|db| {
        let mut statement = db.prepare(
            "SELECT id,created_at,last_used FROM passkeys WHERE user_id=? ORDER BY created_at",
        )?;
        statement
            .query_map([user.id], |row| {
                Ok(serde_json::json!({
                    "id": row.get::<_, String>(0)?,
                    "created_at": row.get::<_, i64>(1)?,
                    "last_used": row.get::<_, Option<i64>>(2)?,
                }))
            })?
            .collect::<Result<Vec<_>, _>>()
    })?;
    Ok(Json(serde_json::json!({"passkeys": rows})))
}

pub(super) async fn options(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let user = require_browser(&state, &headers)?;
    if !recent_browser_session(&state, &headers, &user.id)? {
        consume_step_up(&state, &headers, &user.id)
            .map_err(|_| ApiError::unauthorized("fresh passkey verification required"))?;
    }
    let (ceremony_id, options) = start_registration(
        &state,
        RegistrationKind::AddPasskey,
        &user.id,
        &user.name,
        None,
    )?;
    Ok(Json(serde_json::json!({
        "ceremonyId": ceremony_id,
        "options": options.public_key,
    })))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(super) struct VerifyInput {
    ceremony_id: String,
    response: serde_json::Value,
}

pub(super) async fn verify(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(input): Json<VerifyInput>,
) -> Result<(StatusCode, Json<serde_json::Value>), ApiError> {
    let user = require_browser(&state, &headers)?;
    let finished = finish_registration(
        &state,
        RegistrationKind::AddPasskey,
        &input.ceremony_id,
        input.response,
    )
    .map_err(|_| ApiError::unauthorized("passkey verification failed"))?;
    if finished.user_id != user.id {
        return Err(ApiError::unauthorized("passkey verification failed"));
    }
    state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        insert_passkey(&tx, &user.id, &finished.passkey)?;
        tx.commit()
    })?;
    Ok((StatusCode::CREATED, Json(serde_json::json!({"ok": true}))))
}

pub(super) async fn remove(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let user = require_browser(&state, &headers)?;
    consume_step_up(&state, &headers, &user.id)
        .map_err(|_| ApiError::unauthorized("fresh passkey verification required"))?;
    let deleted = state
        .db
        .with_connection_mut(|db| {
            let tx = db.transaction()?;
            let count: i64 = tx.query_row(
                "SELECT count(*) FROM passkeys WHERE user_id=?",
                [&user.id],
                |row| row.get(0),
            )?;
            if count <= 1 {
                return Ok(false);
            }
            let credential: Option<String> = tx
                .query_row(
                    "SELECT credential_json FROM passkeys WHERE id=? AND user_id=?",
                    rusqlite::params![id, user.id],
                    |row| row.get(0),
                )
                .optional()?;
            let Some(json) = credential else {
                return Err(rusqlite::Error::QueryReturnedNoRows);
            };
            let passkey: webauthn_rs::prelude::Passkey =
                serde_json::from_str(&json).map_err(|_| rusqlite::Error::InvalidQuery)?;
            let credential_id =
                base64::engine::general_purpose::URL_SAFE_NO_PAD.encode(passkey.cred_id().as_ref());
            tx.execute(
                "DELETE FROM clients WHERE user_id=? AND credential_id=?",
                rusqlite::params![user.id, credential_id],
            )?;
            tx.execute(
                "DELETE FROM passkeys WHERE id=? AND user_id=?",
                rusqlite::params![id, user.id],
            )?;
            tx.commit()?;
            Ok(true)
        })
        .map_err(|_| ApiError::not_found("passkey not found"))?;
    if !deleted {
        return Err(ApiError::conflict(
            "add another passkey before removing your last one",
        ));
    }
    Ok(Json(serde_json::json!({"ok": true})))
}
