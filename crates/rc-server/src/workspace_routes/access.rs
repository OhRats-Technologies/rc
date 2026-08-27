mod policy;
mod view;

use super::{INVITE_TTL_MS, owner, parse, principal};
use crate::auth_public_routes::ApiError;
use crate::{AppState, hash, now_ms, opaque};
use axum::{
    Json,
    body::Bytes,
    extract::{Path, State},
    http::{HeaderMap, Method, StatusCode},
};
use policy::{RemoveOutcome, RoleOutcome, browser_step_up};
use rusqlite::OptionalExtension;
use serde::Deserialize;
use uuid::Uuid;

#[derive(Deserialize)]
struct InviteInput {
    role: Option<String>,
}
pub(super) async fn invite(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<(StatusCode, Json<serde_json::Value>), ApiError> {
    let path = format!("/api/v1/workspaces/{id}/invites");
    let p = principal(
        &state,
        &headers,
        &Method::POST,
        &path,
        &body,
        Some("manage-workspaces"),
    )?;
    owner(&state, &p.user.id, &id)?;
    let input: InviteInput = parse(&body)?;
    let role = if input.role.as_deref() == Some("viewer") {
        "viewer"
    } else {
        "operator"
    };
    let token = format!("invite_{}", opaque(24));
    let invite_id = Uuid::new_v4().to_string();
    let expires = now_ms() + INVITE_TTL_MS;
    let inserted = state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        let pending: i64 = tx.query_row(
            "SELECT count(*) FROM workspace_invites WHERE workspace_id=? AND used_at IS NULL AND expires_at>?",
            rusqlite::params![id, now_ms()],
            |row| row.get(0),
        )?;
        if pending >= 25 {
            tx.commit()?;
            return Ok(false);
        }
        tx.execute(
            "INSERT INTO workspace_invites(id,workspace_id,token_hash,role,created_by,created_at,expires_at) VALUES(?,?,?,?,?,?,?)",
            rusqlite::params![invite_id,id,hash(&token),role,p.user.id,now_ms(),expires],
        )?;
        tx.commit()?;
        Ok(true)
    })?;
    if !inserted {
        return Err(ApiError::conflict("pending invite limit reached (25)"));
    }
    state.events.emit(
        &state.db,
        "workspace.invite.created",
        Some(&id),
        Some(&p.user.id),
        None,
        serde_json::json!({"inviteId":invite_id,"role":role}),
    )?;
    Ok((
        StatusCode::CREATED,
        Json(
            serde_json::json!({"token":token,"url":format!("{}/?invite={token}",state.config.public_url.trim_end_matches('/')),"expiresAt":expires}),
        ),
    ))
}

pub(super) async fn access(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/workspaces/{id}/access");
    let p = principal(&state, &headers, &Method::GET, &path, &[], Some("read"))?;
    owner(&state, &p.user.id, &id)?;
    let (members, invites) = view::load(&state, &id)?;
    Ok(Json(serde_json::json!({
        "members": members,
        "invites": invites,
    })))
}

#[derive(Deserialize)]
struct RoleInput {
    role: String,
}
pub(super) async fn member_role(
    State(state): State<AppState>,
    Path((id, member)): Path<(String, String)>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/workspaces/{id}/members/{member}");
    let p = principal(&state, &headers, &Method::PATCH, &path, &body, None)?;
    browser_step_up(&state, &headers, &p)?;
    let input: RoleInput = parse(&body)?;
    if !matches!(input.role.as_str(), "owner" | "operator" | "viewer") {
        return Err(ApiError::bad_request("invalid role"));
    }
    let current = match state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        let actor: Option<String> = tx
            .query_row(
                "SELECT role FROM workspace_members WHERE workspace_id=? AND user_id=?",
                rusqlite::params![id, p.user.id],
                |row| row.get(0),
            )
            .optional()?;
        if actor.as_deref() != Some("owner") {
            tx.commit()?;
            return Ok(RoleOutcome::OwnerRequired);
        }
        let current: Option<String> = tx
            .query_row(
                "SELECT role FROM workspace_members WHERE workspace_id=? AND user_id=?",
                rusqlite::params![id, member],
                |row| row.get(0),
            )
            .optional()?;
        let Some(current) = current else {
            tx.commit()?;
            return Ok(RoleOutcome::Missing);
        };
        if current == "owner" && input.role != "owner" {
            let owners: i64 = tx.query_row(
                "SELECT count(*) FROM workspace_members WHERE workspace_id=? AND role='owner'",
                [&id],
                |row| row.get(0),
            )?;
            if owners <= 1 {
                tx.commit()?;
                return Ok(RoleOutcome::LastOwner);
            }
        }
        tx.execute(
            "UPDATE workspace_members SET role=? WHERE workspace_id=? AND user_id=?",
            rusqlite::params![input.role, id, member],
        )?;
        tx.commit()?;
        Ok(RoleOutcome::Updated(current))
    })? {
        RoleOutcome::Updated(current) => current,
        RoleOutcome::OwnerRequired => return Err(ApiError::forbidden("owner required")),
        RoleOutcome::Missing => return Err(ApiError::not_found("member not found")),
        RoleOutcome::LastOwner => return Err(ApiError::conflict("promote another owner first")),
    };
    state.events.emit(
        &state.db,
        "workspace.member.role",
        Some(&id),
        Some(&p.user.id),
        None,
        serde_json::json!({"memberId":member,"from":current,"role":input.role}),
    )?;
    Ok(Json(serde_json::json!({"role":input.role})))
}

pub(super) async fn member_remove(
    State(state): State<AppState>,
    Path((id, member)): Path<(String, String)>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/workspaces/{id}/members/{member}");
    let p = principal(&state, &headers, &Method::DELETE, &path, &[], None)?;
    browser_step_up(&state, &headers, &p)?;
    if member == p.user.id {
        return Err(ApiError::conflict(
            "use Leave workspace for your own account",
        ));
    }
    let role = match state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        let actor: Option<String> = tx
            .query_row(
                "SELECT role FROM workspace_members WHERE workspace_id=? AND user_id=?",
                rusqlite::params![id, p.user.id],
                |row| row.get(0),
            )
            .optional()?;
        if actor.as_deref() != Some("owner") {
            tx.commit()?;
            return Ok(RemoveOutcome::OwnerRequired);
        }
        let role: Option<String> = tx
            .query_row(
                "SELECT role FROM workspace_members WHERE workspace_id=? AND user_id=?",
                rusqlite::params![id, member],
                |row| row.get(0),
            )
            .optional()?;
        let Some(role) = role else {
            tx.commit()?;
            return Ok(RemoveOutcome::Missing);
        };
        if role == "owner" {
            let owners: i64 = tx.query_row(
                "SELECT count(*) FROM workspace_members WHERE workspace_id=? AND role='owner'",
                [&id],
                |row| row.get(0),
            )?;
            if owners <= 1 {
                tx.commit()?;
                return Ok(RemoveOutcome::LastOwner);
            }
        }
        tx.execute(
            "DELETE FROM workspace_members WHERE workspace_id=? AND user_id=?",
            rusqlite::params![id, member],
        )?;
        tx.commit()?;
        Ok(RemoveOutcome::Removed(role))
    })? {
        RemoveOutcome::Removed(role) => role,
        RemoveOutcome::OwnerRequired => return Err(ApiError::forbidden("owner required")),
        RemoveOutcome::Missing => return Err(ApiError::not_found("member not found")),
        RemoveOutcome::LastOwner => return Err(ApiError::conflict("workspace needs an owner")),
    };
    state.events.emit(
        &state.db,
        "workspace.member.removed",
        Some(&id),
        Some(&p.user.id),
        None,
        serde_json::json!({"memberId":member,"role":role}),
    )?;
    Ok(Json(serde_json::json!({"ok": true})))
}

pub(super) async fn invite_remove(
    State(state): State<AppState>,
    Path((id, invite)): Path<(String, String)>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/workspaces/{id}/invites/{invite}");
    let p = principal(
        &state,
        &headers,
        &Method::DELETE,
        &path,
        &[],
        Some("manage-workspaces"),
    )?;
    owner(&state, &p.user.id, &id)?;
    if state.db.with_connection(|db| {
        db.execute(
            "DELETE FROM workspace_invites WHERE id=? AND workspace_id=? AND used_at IS NULL",
            rusqlite::params![invite, id],
        )
    })? == 0
    {
        return Err(ApiError::not_found("invite not found"));
    }
    state.events.emit(
        &state.db,
        "workspace.invite.revoked",
        Some(&id),
        Some(&p.user.id),
        None,
        serde_json::json!({"inviteId":invite}),
    )?;
    Ok(Json(serde_json::json!({"ok": true})))
}
