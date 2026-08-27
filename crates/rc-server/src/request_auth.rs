use crate::{AppState, AuthPrincipal, authenticate};
use axum::http::{HeaderMap, Method, StatusCode};

#[derive(Debug, thiserror::Error)]
pub enum RequestAuthError {
    #[error("authentication required")]
    Unauthorized,
    #[error("browser session required")]
    BrowserRequired,
    #[error("API key requires {0} scope")]
    Scope(&'static str),
    #[error("database error")]
    Internal,
}

impl RequestAuthError {
    pub fn status(&self) -> StatusCode {
        match self {
            Self::Unauthorized | Self::BrowserRequired => StatusCode::UNAUTHORIZED,
            Self::Scope(_) => StatusCode::FORBIDDEN,
            Self::Internal => StatusCode::INTERNAL_SERVER_ERROR,
        }
    }
}

pub fn require_principal(
    state: &AppState,
    headers: &HeaderMap,
    method: &Method,
    path: &str,
    body: &[u8],
    api_scope: Option<&'static str>,
) -> Result<AuthPrincipal, RequestAuthError> {
    let principal = authenticate(state, headers, method, path, body)
        .map_err(|_| RequestAuthError::Unauthorized)?
        .ok_or(RequestAuthError::Unauthorized)?;
    if let (Some(client), Some(scope)) = (&principal.client, api_scope)
        && client.kind == "api"
        && !client.has_scope(scope)
    {
        return Err(RequestAuthError::Scope(scope));
    }
    Ok(principal)
}

pub fn require_browser(
    state: &AppState,
    headers: &HeaderMap,
) -> Result<AuthPrincipal, RequestAuthError> {
    let principal = authenticate(state, headers, &Method::GET, "/", &[])
        .map_err(|_| RequestAuthError::Unauthorized)?
        .ok_or(RequestAuthError::Unauthorized)?;
    if !principal.browser {
        return Err(RequestAuthError::BrowserRequired);
    }
    Ok(principal)
}
