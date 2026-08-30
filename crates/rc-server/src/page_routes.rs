mod actions;
mod auth;
mod context;
mod data;
mod docs;
mod fallback;
mod responses;

pub(crate) use fallback::route as fallback;

use responses::{internal, not_found, redirect};

use crate::{
    AppState, cli_authorization_preview, mcp_resource, process_json, processes_for_device,
    workspace_json, workspace_role,
};
use axum::{
    Router,
    extract::{Path, Query, State},
    http::{HeaderMap, StatusCode},
    response::{Html, IntoResponse, Response},
    routing::get,
};

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/", get(auth::home))
        .route("/login", get(auth::login))
        .route("/signup", get(auth::signup))
        .route("/setup/{token}", get(auth::setup_link))
        .route("/devices", get(devices))
        .route("/devices/enroll", get(enroll))
        .route("/devices/{id}", get(device))
        .route("/devices/{device}/processes/{process}", get(process))
        .route("/workspaces/{id}/access", get(workspace_access))
        .route("/workspaces/{id}/activity", get(workspace_activity))
        .route("/account", get(account))
        .route("/api", get(api_keys))
        .route("/cli/login", get(cli_login))
        .route("/integrations/mcp", get(mcp_integrations))
        .route("/docs", get(docs::index))
        .route("/docs/{topic}", get(docs::topic))
        .route("/install.sh", get(docs::install_script))
        .route("/robots.txt", get(docs::robots))
        .merge(actions::routes())
}

async fn devices(State(state): State<AppState>, headers: HeaderMap) -> Response {
    let context = match context::load(&state, &headers, "/devices").await {
        Ok(Some(value)) => value,
        Ok(None) => return redirect("/login"),
        Err(error) => return internal(error),
    };
    Html(crate::page_html::devices(&context)).into_response()
}

async fn enroll(
    State(state): State<AppState>,
    headers: HeaderMap,
    Query(query): Query<std::collections::HashMap<String, String>>,
) -> Response {
    let context = match context::load(&state, &headers, "/devices/enroll").await {
        Ok(Some(value)) => value,
        Ok(None) => return redirect("/login"),
        Err(error) => return internal(error),
    };
    let selected = query
        .get("workspace")
        .cloned()
        .or_else(|| {
            context
                .workspaces
                .iter()
                .find(|workspace| workspace["role"] == "owner")
                .and_then(|workspace| workspace["id"].as_str().map(str::to_owned))
        })
        .unwrap_or_default();
    Html(crate::page_html::enroll(&context, &selected)).into_response()
}

async fn device(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Response {
    let path = format!("/devices/{id}");
    let context = match context::load(&state, &headers, &path).await {
        Ok(Some(value)) => value,
        Ok(None) => return redirect("/login"),
        Err(error) => return internal(error),
    };
    let Some(device) = context
        .devices
        .iter()
        .find(|device| device["id"] == id)
        .cloned()
    else {
        return not_found(&context);
    };
    let processes = if device["role"] == "viewer" {
        Vec::new()
    } else {
        match processes_for_device(&state, &context.user.id, &id) {
            Ok(value) => value,
            Err(error) => return internal(error),
        }
    };
    Html(crate::page_html::device(
        &context,
        &device,
        &processes,
        state.execution.persists_process_metadata(),
    ))
    .into_response()
}

async fn process(
    State(state): State<AppState>,
    Path((device_id, process_id)): Path<(String, String)>,
    headers: HeaderMap,
) -> Response {
    let path = format!("/devices/{device_id}/processes/{process_id}");
    let context = match context::load(&state, &headers, &path).await {
        Ok(Some(value)) => value,
        Ok(None) => return redirect("/login"),
        Err(error) => return internal(error),
    };
    let Some(device) = context
        .devices
        .iter()
        .find(|device| device["id"] == device_id)
        .cloned()
    else {
        return not_found(&context);
    };
    let process = match process_json(&state, &context.user.id, &process_id) {
        Ok(value) => value,
        Err(error) => return internal(error),
    };
    let Some(process) = process else {
        return not_found(&context);
    };
    if process["device_id"] != device_id {
        return not_found(&context);
    }
    Html(crate::page_html::process(&context, &device, &process)).into_response()
}

async fn account(State(state): State<AppState>, headers: HeaderMap) -> Response {
    let context = match context::load(&state, &headers, "/account").await {
        Ok(Some(value)) => value,
        Ok(None) => return redirect("/login"),
        Err(error) => return internal(error),
    };
    let passkeys = match data::passkeys(&state, &context.user.id) {
        Ok(value) => value,
        Err(error) => return internal(error),
    };
    Html(crate::page_html::account(&context, &passkeys)).into_response()
}

async fn api_keys(State(state): State<AppState>, headers: HeaderMap) -> Response {
    let context = match context::load(&state, &headers, "/api").await {
        Ok(Some(value)) => value,
        Ok(None) => return redirect("/login"),
        Err(error) => return internal(error),
    };
    let keys = match data::api_keys(&state, &context.user.id) {
        Ok(value) => value,
        Err(error) => return internal(error),
    };
    Html(crate::page_html::api_keys(&context, &keys)).into_response()
}

async fn cli_login(
    State(state): State<AppState>,
    headers: HeaderMap,
    Query(query): Query<std::collections::HashMap<String, String>>,
) -> Response {
    let code = query.get("code").cloned().unwrap_or_default();
    let preview = match cli_authorization_preview(&state, &code) {
        Ok(value) => value,
        Err(error) => return internal(error),
    };
    let Some((client, key, lifetime)) = preview else {
        return (StatusCode::GONE, "CLI authorization expired").into_response();
    };
    let context = match context::load(&state, &headers, &format!("/cli/login?code={code}")).await {
        Ok(Some(value)) => value,
        Ok(None) => {
            return redirect(&format!(
                "/?next={}",
                url::form_urlencoded::byte_serialize(format!("/cli/login?code={code}").as_bytes())
                    .collect::<String>()
            ));
        }
        Err(error) => return internal(error),
    };
    Html(crate::page_html::cli_login(
        &context, &code, &client, &key, &lifetime,
    ))
    .into_response()
}

async fn mcp_integrations(State(state): State<AppState>, headers: HeaderMap) -> Response {
    let context = match context::load(&state, &headers, "/integrations/mcp").await {
        Ok(Some(value)) => value,
        Ok(None) => return redirect("/login"),
        Err(error) => return internal(error),
    };
    let grants = match data::mcp_grants(&state, &context.user.id) {
        Ok(value) => value,
        Err(error) => return internal(error),
    };
    Html(crate::page_html::mcp_page(
        &context,
        &mcp_resource(&state),
        &grants,
    ))
    .into_response()
}

async fn workspace_access(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Response {
    let path = format!("/workspaces/{id}/access");
    let context = match context::load(&state, &headers, &path).await {
        Ok(Some(value)) => value,
        Ok(None) => return redirect("/login"),
        Err(error) => return internal(error),
    };
    let role = match workspace_role(&state, &context.user.id, &id) {
        Ok(value) => value,
        Err(error) => return internal(error.into()),
    };
    if role.as_deref() != Some("owner") {
        return not_found(&context);
    }
    let workspace = match workspace_json(&state, &context.user.id, &id) {
        Ok(value) => value,
        Err(error) => return internal(error),
    };
    let Some(workspace) = workspace else {
        return not_found(&context);
    };
    let (members, invites) = match data::workspace_access(&state, &id) {
        Ok(value) => value,
        Err(error) => return internal(error),
    };
    Html(crate::page_html::workspace_access(
        &context, &workspace, &members, &invites,
    ))
    .into_response()
}

async fn workspace_activity(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Response {
    let path = format!("/workspaces/{id}/activity");
    let context = match context::load(&state, &headers, &path).await {
        Ok(Some(value)) => value,
        Ok(None) => return redirect("/login"),
        Err(error) => return internal(error),
    };
    let role = match workspace_role(&state, &context.user.id, &id) {
        Ok(value) => value,
        Err(error) => return internal(error.into()),
    };
    if role.is_none() {
        return not_found(&context);
    }
    let workspace = match workspace_json(&state, &context.user.id, &id) {
        Ok(value) => value,
        Err(error) => return internal(error),
    };
    let Some(workspace) = workspace else {
        return not_found(&context);
    };
    let events = match data::activity(&state, &id) {
        Ok(value) => value,
        Err(error) => return internal(error),
    };
    Html(crate::page_html::activity(&context, &workspace, &events)).into_response()
}
