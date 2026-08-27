use super::oauth_store::{grant_workspaces, oauth_query, request_full, request_redirect};
use crate::auth_public_routes::ApiError;
use crate::{
    AppState, MCP_SCOPES, approve_oauth_grant, browser_user, create_oauth_request, exchange_token,
    mcp_resource, prepare_oauth_grant, register_mcp_client, revoke_mcp_grant,
};
use axum::{
    Json,
    body::Bytes,
    extract::{Path, Query, State},
    http::{HeaderMap, StatusCode},
    response::{Html, IntoResponse, Response},
};
use serde::Deserialize;

pub(super) async fn protected_metadata(State(state): State<AppState>) -> Json<serde_json::Value> {
    Json(serde_json::json!({
        "resource":mcp_resource(&state),"authorization_servers":[state.config.public_url.as_str()],
        "scopes_supported":MCP_SCOPES,"bearer_methods_supported":["header"]
    }))
}

pub(super) async fn oauth_metadata(State(state): State<AppState>) -> Json<serde_json::Value> {
    let base = state.config.public_url.trim_end_matches('/');
    Json(serde_json::json!({
        "issuer":base,"authorization_endpoint":format!("{base}/oauth/authorize"),
        "token_endpoint":format!("{base}/oauth/token"),"registration_endpoint":format!("{base}/oauth/register"),
        "response_types_supported":["code"],"grant_types_supported":["authorization_code","refresh_token"],
        "token_endpoint_auth_methods_supported":["none"],"code_challenge_methods_supported":["S256"],
        "scopes_supported":MCP_SCOPES,"authorization_response_iss_parameter_supported":true
    }))
}

pub(super) async fn register(
    State(state): State<AppState>,
    Json(body): Json<serde_json::Value>,
) -> Result<(StatusCode, Json<serde_json::Value>), ApiError> {
    Ok((
        StatusCode::CREATED,
        Json(register_mcp_client(&state, &body).map_err(ApiError::bad_request_owned)?),
    ))
}

pub(super) async fn authorize(
    State(state): State<AppState>,
    headers: HeaderMap,
    Query(query): Query<std::collections::HashMap<String, String>>,
) -> Response {
    let user = match browser_user(&state, &headers) {
        Ok(Some(user)) => user,
        Ok(None) => {
            let raw = url::form_urlencoded::Serializer::new(String::new())
                .extend_pairs(query.iter())
                .finish();
            let next = format!("/oauth/authorize?{raw}");
            let location = format!(
                "/?next={}",
                url::form_urlencoded::byte_serialize(next.as_bytes()).collect::<String>()
            );
            return (StatusCode::SEE_OTHER, [("location", location)]).into_response();
        }
        Err(error) => {
            tracing::error!(%error, "MCP authorization session lookup failed");
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Html(crate::page_html::error(500, "Internal server error")),
            )
                .into_response();
        }
    };
    let raw = url::form_urlencoded::Serializer::new(String::new())
        .extend_pairs(query.iter())
        .finish();
    let request = match create_oauth_request(&state, &user, &raw) {
        Ok(value) => value,
        Err(error) => return (StatusCode::BAD_REQUEST, error.to_string()).into_response(),
    };
    let request_id = request["requestId"].as_str().unwrap_or_default();
    let client = request["clientName"].as_str().unwrap_or("MCP client");
    let callback = request["redirectUri"].as_str().unwrap_or_default();
    let requested = request["requestedScopes"]
        .as_array()
        .cloned()
        .unwrap_or_default();
    let devices = match crate::devices_json(&state, &user.id).await {
        Ok(value) => value,
        Err(error) => {
            tracing::error!(%error, "MCP authorization device lookup failed");
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Html(crate::page_html::error(500, "Internal server error")),
            )
                .into_response();
        }
    };
    Html(super::page::authorize_page(
        request_id, client, &user.name, callback, &requested, &devices,
    ))
    .into_response()
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(super) struct PrepareInput {
    request_id: String,
    device_ids: Vec<String>,
    scopes: Vec<String>,
    lifetime: Option<String>,
}

pub(super) async fn prepare(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(input): Json<PrepareInput>,
) -> Result<Json<crate::PreparedGrant>, ApiError> {
    let user = require_browser_user(&state, &headers)?;
    Ok(Json(
        prepare_oauth_grant(
            &state,
            &user,
            &input.request_id,
            &input.device_ids,
            &input.scopes,
            input.lifetime.as_deref(),
        )
        .map_err(ApiError::bad_request_owned)?,
    ))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(super) struct ApproveInput {
    request_id: String,
    control_client_id: String,
    signature: String,
}

pub(super) async fn approve(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(input): Json<ApproveInput>,
) -> Result<Json<crate::ApprovedGrant>, ApiError> {
    let user = require_browser_user(&state, &headers)?;
    Ok(Json(
        approve_oauth_grant(
            &state,
            &user,
            &input.request_id,
            &input.control_client_id,
            &input.signature,
        )
        .map_err(ApiError::bad_request_owned)?,
    ))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(super) struct RequestOnly {
    request_id: String,
}

pub(super) async fn cancel(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(input): Json<RequestOnly>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let user = require_browser_user(&state, &headers)?;
    let row = request_redirect(&state, &user.id, &input.request_id)?;
    state.db.with_connection(|db| {
        db.execute(
            "DELETE FROM mcp_requests WHERE id=? AND user_id=?",
            rusqlite::params![input.request_id, user.id],
        )?;
        Ok(())
    })?;
    let mut redirect =
        url::Url::parse(&row.0).map_err(|_| ApiError::bad_request("invalid redirect"))?;
    redirect
        .query_pairs_mut()
        .append_pair("error", "access_denied")
        .append_pair(
            "error_description",
            "The user declined this MCP authorization request.",
        );
    if !row.1.is_empty() {
        redirect.query_pairs_mut().append_pair("state", &row.1);
    }
    redirect
        .query_pairs_mut()
        .append_pair("iss", state.config.public_url.trim_end_matches('/'));
    Ok(Json(serde_json::json!({"redirect":redirect.as_str()})))
}

pub(super) async fn switch_account(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(input): Json<RequestOnly>,
) -> Result<Response, ApiError> {
    let user = require_browser_user(&state, &headers)?;
    let row = request_full(&state, &user.id, &input.request_id)?;
    state.db.with_connection(|db| {
        db.execute("DELETE FROM mcp_requests WHERE id=?", [input.request_id])?;
        Ok(())
    })?;
    crate::revoke_browser_session(&state, &headers)
        .map_err(|_| ApiError::unauthorized("authentication required"))?;
    let next = oauth_query(&row);
    let location = format!(
        "/?next={}",
        url::form_urlencoded::byte_serialize(next.as_bytes()).collect::<String>()
    );
    let mut response = Json(serde_json::json!({"redirect":location})).into_response();
    let cookie = crate::clear_session_cookie(&state);
    let value = cookie
        .parse()
        .map_err(|_| ApiError::bad_gateway("failed to clear browser session"))?;
    response.headers_mut().insert("set-cookie", value);
    Ok(response)
}

pub(super) async fn token(
    State(state): State<AppState>,
    body: Bytes,
) -> Result<Json<serde_json::Value>, ApiError> {
    let form =
        std::str::from_utf8(&body).map_err(|_| ApiError::bad_request("invalid token request"))?;
    Ok(Json(
        exchange_token(&state, form).map_err(ApiError::bad_request_owned)?,
    ))
}

pub(super) async fn revoke(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let user = require_browser_user(&state, &headers)?;
    let workspaces = grant_workspaces(&state, &id)?;
    if !revoke_mcp_grant(&state, &user.id, &id)? {
        return Err(ApiError::not_found("MCP grant not found"));
    }
    Ok(Json(
        serde_json::json!({"ok":true,"workspaceIds":workspaces}),
    ))
}

fn require_browser_user(
    state: &AppState,
    headers: &HeaderMap,
) -> Result<crate::UserIdentity, ApiError> {
    browser_user(state, headers)
        .map_err(|_| ApiError::unauthorized("authentication required"))?
        .ok_or_else(|| ApiError::unauthorized("authentication required"))
}
