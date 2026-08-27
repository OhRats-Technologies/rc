use crate::{AppState, browser_user, devices_json, workspaces_json};
use axum::http::HeaderMap;

pub(super) async fn load(
    state: &AppState,
    headers: &HeaderMap,
    path: &str,
) -> anyhow::Result<Option<crate::page_html::PageContext>> {
    let Some(user) = browser_user(state, headers)? else {
        return Ok(None);
    };
    let workspaces = workspaces_json(state, &user.id)?;
    let devices = devices_json(state, &user.id).await?;
    Ok(Some(crate::page_html::PageContext {
        user,
        workspaces,
        devices,
        path: path.to_owned(),
        sidebar: cookie(headers, "rc_sidebar")
            .filter(|value| matches!(value.as_str(), "open" | "closed"))
            .unwrap_or_else(|| "open".into()),
    }))
}

fn cookie(headers: &HeaderMap, name: &str) -> Option<String> {
    headers
        .get("cookie")?
        .to_str()
        .ok()?
        .split(';')
        .filter_map(|part| part.trim().split_once('='))
        .find(|(key, _)| *key == name)
        .map(|(_, value)| value.to_owned())
}
