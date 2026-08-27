use super::{parse, principal};
use crate::auth_public_routes::ApiError;
use crate::{AppState, hash, now_ms};
use axum::{
    Json,
    body::Bytes,
    extract::{Path, State},
    http::{HeaderMap, Method},
};
use rusqlite::OptionalExtension;
use serde::Deserialize;

#[derive(Deserialize)]
struct JoinInput {
    token: String,
}

pub(super) async fn join(
    State(state): State<AppState>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Json<serde_json::Value>, ApiError> {
    let principal = principal(
        &state,
        &headers,
        &Method::POST,
        "/api/v1/workspaces/join",
        &body,
        Some("manage-workspaces"),
    )?;
    let input: JoinInput = parse(&body)?;
    let (workspace, role) = state
        .db
        .with_connection_mut(|db| {
            let tx = db.transaction()?;
            let invite = tx
                .query_row(
                    "SELECT id,workspace_id,role FROM workspace_invites WHERE token_hash=? AND used_at IS NULL AND expires_at>?",
                    rusqlite::params![hash(input.token.trim()), now_ms()],
                    |row| {
                        Ok((
                            row.get::<_, String>(0)?,
                            row.get::<_, String>(1)?,
                            row.get::<_, String>(2)?,
                        ))
                    },
                )
                .optional()?;
            let Some((invite_id, workspace, role)) = invite else {
                return Err(rusqlite::Error::QueryReturnedNoRows);
            };
            let joined_at = now_ms();
            if tx.execute(
                "UPDATE workspace_invites SET used_at=? WHERE id=? AND used_at IS NULL",
                rusqlite::params![joined_at, invite_id],
            )? != 1
            {
                return Err(rusqlite::Error::InvalidQuery);
            }
            tx.execute(
                "INSERT OR IGNORE INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,?,?)",
                rusqlite::params![workspace, principal.user.id, role, joined_at],
            )?;
            tx.commit()?;
            Ok((workspace, role))
        })
        .map_err(|_| ApiError::unauthorized("invalid or expired invite"))?;
    state.events.emit(
        &state.db,
        "workspace.member.joined",
        Some(&workspace),
        Some(&principal.user.id),
        None,
        serde_json::json!({"role": role}),
    )?;
    Ok(Json(serde_json::json!({"workspaceId": workspace})))
}

pub(super) async fn leave(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/workspaces/{id}/leave");
    let principal = principal(
        &state,
        &headers,
        &Method::POST,
        &path,
        &[],
        Some("manage-workspaces"),
    )?;
    leave_workspace(&state, &principal.user.id, &id)?;
    Ok(Json(serde_json::json!({"ok": true})))
}

pub(crate) fn leave_workspace(
    state: &AppState,
    user_id: &str,
    workspace_id: &str,
) -> Result<(), ApiError> {
    let role = match state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        let role: Option<String> = tx
            .query_row(
                "SELECT role FROM workspace_members WHERE workspace_id=? AND user_id=?",
                rusqlite::params![workspace_id, user_id],
                |row| row.get(0),
            )
            .optional()?;
        let Some(role) = role else {
            tx.commit()?;
            return Ok(LeaveOutcome::Missing);
        };
        if role == "owner" {
            let owners: i64 = tx.query_row(
                "SELECT count(*) FROM workspace_members WHERE workspace_id=? AND role='owner'",
                [workspace_id],
                |row| row.get(0),
            )?;
            if owners <= 1 {
                tx.commit()?;
                return Ok(LeaveOutcome::LastOwner);
            }
        }
        tx.execute(
            "DELETE FROM workspace_members WHERE workspace_id=? AND user_id=?",
            rusqlite::params![workspace_id, user_id],
        )?;
        tx.commit()?;
        Ok(LeaveOutcome::Left(role))
    })? {
        LeaveOutcome::Left(role) => role,
        LeaveOutcome::Missing => return Err(ApiError::not_found("workspace not found")),
        LeaveOutcome::LastOwner => {
            return Err(ApiError::conflict("promote another owner before leaving"));
        }
    };
    state.events.emit(
        &state.db,
        "workspace.member.left",
        Some(workspace_id),
        Some(user_id),
        None,
        serde_json::json!({"role": role}),
    )?;
    Ok(())
}

enum LeaveOutcome {
    Left(String),
    Missing,
    LastOwner,
}
