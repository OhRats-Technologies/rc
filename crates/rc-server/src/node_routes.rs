mod messages;
mod peer;

use crate::{
    AppState, EnrollmentDevice, EnrollmentInsert,
    node_auth::{HttpError, verify_node_request},
    webrtc_util::{complete_local_description, peer_connection},
};
use axum::{
    Json, Router,
    body::Bytes,
    extract::State,
    http::{HeaderMap, Method, StatusCode},
    response::IntoResponse,
    routing::{get, post},
};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use uuid::Uuid;
use webrtc::peer_connection::sdp::session_description::RTCSessionDescription;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/node/enroll", post(enroll))
        .route("/api/v1/node/ice", get(ice))
        .route("/api/v1/node/connect", post(connect))
        .route("/api/v1/node/self", get(self_status).delete(remove_self))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct EnrollRequest {
    token: String,
    name: String,
    hostname: String,
    platform: String,
    arch: String,
    identity_public_key: String,
    transport_public_key: String,
    version: String,
    capabilities: Vec<String>,
}
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct EnrollResponse {
    device_id: String,
}

async fn enroll(
    State(state): State<AppState>,
    Json(input): Json<EnrollRequest>,
) -> Result<impl IntoResponse, HttpError> {
    let name = input.name.trim().chars().take(120).collect::<String>();
    let hostname = input.hostname.trim().chars().take(255).collect::<String>();
    let platform = input.platform.trim().chars().take(32).collect::<String>();
    let arch = input.arch.trim().chars().take(32).collect::<String>();
    let version = input.version.trim().chars().take(64).collect::<String>();
    let capabilities_valid = input.capabilities.len() <= 32
        && input.capabilities.iter().all(|value| {
            !value.is_empty()
                && value.len() <= 64
                && value
                    .bytes()
                    .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'-')
        });
    if name.is_empty()
        || hostname.is_empty()
        || platform.is_empty()
        || arch.is_empty()
        || version.is_empty()
        || !capabilities_valid
        || !valid_key(&input.identity_public_key)
        || !valid_key(&input.transport_public_key)
    {
        return Err(HttpError(StatusCode::BAD_REQUEST, "invalid enrollment"));
    }
    let hash = hex_lower(&Sha256::digest(input.token.as_bytes()));
    let id = Uuid::new_v4().to_string();
    let result = state
        .db
        .enroll_device(
            &hash,
            &EnrollmentDevice {
                id: id.clone(),
                name: name.clone(),
                hostname,
                platform: platform.clone(),
                arch,
                identity_public_key: input.identity_public_key,
                transport_public_key: input.transport_public_key,
                version,
                capabilities: input.capabilities,
            },
        )
        .map_err(enroll_db_error)?;
    let workspace_id = match result {
        EnrollmentInsert::Inserted { workspace_id } => workspace_id,
        EnrollmentInsert::Invalid => {
            return Err(HttpError(
                StatusCode::UNAUTHORIZED,
                "invalid enrollment token",
            ));
        }
        EnrollmentInsert::DeviceLimit => {
            return Err(HttpError(
                StatusCode::CONFLICT,
                "workspace device limit reached",
            ));
        }
    };
    state
        .events
        .emit(
            &state.db,
            "device.enrolled",
            Some(&workspace_id),
            None,
            Some(&id),
            serde_json::json!({"name":name,"platform":platform}),
        )
        .map_err(db_error)?;
    Ok((StatusCode::CREATED, Json(EnrollResponse { device_id: id })))
}

async fn ice(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, HttpError> {
    verify_node_request(&state, &headers, &Method::GET, "/api/v1/node/ice", &[])?;
    let servers = state
        .turn
        .ice_servers()
        .await
        .map_err(|_| HttpError(StatusCode::BAD_GATEWAY, "TURN unavailable"))?;
    Ok(Json(serde_json::json!({ "iceServers": servers })))
}

#[derive(Deserialize)]
struct ConnectOffer {
    sdp: String,
}
#[derive(Serialize)]
struct ConnectAnswer {
    sdp: String,
}

async fn connect(
    State(state): State<AppState>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Json<ConnectAnswer>, HttpError> {
    let identity = verify_node_request(
        &state,
        &headers,
        &Method::POST,
        "/api/v1/node/connect",
        &body,
    )?;
    let input: ConnectOffer = serde_json::from_slice(&body)
        .map_err(|_| HttpError(StatusCode::BAD_REQUEST, "invalid SDP offer"))?;
    if input.sdp.len() > 256_000 {
        return Err(HttpError(
            StatusCode::PAYLOAD_TOO_LARGE,
            "SDP offer too large",
        ));
    }
    let servers = state
        .turn
        .ice_servers()
        .await
        .map_err(|_| HttpError(StatusCode::BAD_GATEWAY, "TURN unavailable"))?;
    let peer = peer_connection(&servers).await.map_err(webrtc_error)?;
    let connection_id = Uuid::new_v4().to_string();
    state.release_device_sessions(&identity.id);
    let replaced_online = state
        .nodes
        .insert_pending(&identity.id, connection_id.clone(), peer.clone())
        .await;
    if replaced_online {
        state.emit_device_presence(&identity.id, false);
    }
    peer::configure(&state, &identity.id, &connection_id, peer.clone());
    peer.set_remote_description(RTCSessionDescription::offer(input.sdp).map_err(webrtc_error)?)
        .await
        .map_err(webrtc_error)?;
    let answer = peer.create_answer(None).await.map_err(webrtc_error)?;
    let sdp = complete_local_description(&peer, answer)
        .await
        .map_err(webrtc_error)?;
    Ok(Json(ConnectAnswer { sdp }))
}

async fn self_status(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, HttpError> {
    let identity = verify_node_request(&state, &headers, &Method::GET, "/api/v1/node/self", &[])?;
    let row = state
        .db
        .node_status(&identity.id)
        .map_err(db_error)?
        .ok_or(HttpError(StatusCode::NOT_FOUND, "Node not found"))?;
    Ok(Json(
        serde_json::json!({ "name": row.name, "online": state.nodes.online(&identity.id).await, "version": row.version }),
    ))
}

async fn remove_self(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<StatusCode, HttpError> {
    let identity =
        verify_node_request(&state, &headers, &Method::DELETE, "/api/v1/node/self", &[])?;
    if state.db.revoke_device(&identity.id).map_err(db_error)? {
        state.disconnect_device(&identity.id).await;
        Ok(StatusCode::NO_CONTENT)
    } else {
        Err(HttpError(StatusCode::NOT_FOUND, "Node not found"))
    }
}

fn valid_key(value: &str) -> bool {
    URL_SAFE_NO_PAD
        .decode(value)
        .map(|bytes| bytes.len() == 32)
        .unwrap_or(false)
}
fn db_error(_: rusqlite::Error) -> HttpError {
    HttpError(StatusCode::INTERNAL_SERVER_ERROR, "database error")
}
fn enroll_db_error(error: rusqlite::Error) -> HttpError {
    if matches!(
        error,
        rusqlite::Error::SqliteFailure(
            rusqlite::ffi::Error {
                code: rusqlite::ErrorCode::ConstraintViolation,
                ..
            },
            _
        )
    ) {
        HttpError(StatusCode::CONFLICT, "Node identity is already enrolled")
    } else {
        db_error(error)
    }
}
fn webrtc_error(error: impl std::fmt::Display) -> HttpError {
    tracing::warn!(%error, "WebRTC error");
    HttpError(StatusCode::BAD_GATEWAY, "WebRTC negotiation failed")
}
fn hex_lower(bytes: &[u8]) -> String {
    bytes.iter().map(|byte| format!("{byte:02x}")).collect()
}
