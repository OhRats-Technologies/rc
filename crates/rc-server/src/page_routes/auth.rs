use crate::{AppState, active_user_count, browser_user, hash, now_ms};
use axum::{
    extract::{Path, Query, State},
    http::{HeaderMap, HeaderValue, StatusCode},
    response::{Html, IntoResponse, Response},
};
use rusqlite::OptionalExtension;

pub(super) async fn home(
    State(state): State<AppState>,
    headers: HeaderMap,
    Query(query): Query<std::collections::HashMap<String, String>>,
) -> Response {
    let users = match active_user_count(&state) {
        Ok(value) => value,
        Err(error) => return super::internal(error.into()),
    };
    if users == 0 {
        return Html(crate::page_html::auth(crate::page_html::AuthPage::Setup {
            authorized: setup_authorized(&state, &headers),
        }))
        .into_response();
    }
    match browser_user(&state, &headers) {
        Ok(Some(_)) => {
            if let Some(invite) = query.get("invite") {
                return Html(crate::page_html::auth(crate::page_html::AuthPage::Join {
                    invite,
                }))
                .into_response();
            }
            return super::redirect("/devices");
        }
        Ok(None) => {}
        Err(error) => return super::internal(error.into()),
    }
    if let Some(invite) = query.get("invite") {
        let valid = match valid_invite(&state, invite) {
            Ok(value) => value,
            Err(error) => return super::internal(error.into()),
        };
        if valid {
            return Html(crate::page_html::auth(
                crate::page_html::AuthPage::Register { invite },
            ))
            .into_response();
        }
        return (
            StatusCode::UNAUTHORIZED,
            Html(crate::page_html::auth(
                crate::page_html::AuthPage::InvalidInvite,
            )),
        )
            .into_response();
    }
    let next = query
        .get("next")
        .filter(|value| safe_next(value))
        .map(String::as_str)
        .unwrap_or("/devices");
    Html(crate::page_html::auth(crate::page_html::AuthPage::Login {
        next,
        signup: public_signup_configured(&state, users),
    }))
    .into_response()
}

pub(super) async fn login(
    State(state): State<AppState>,
    headers: HeaderMap,
    Query(query): Query<std::collections::HashMap<String, String>>,
) -> Response {
    match browser_user(&state, &headers) {
        Ok(Some(_)) => return super::redirect("/devices"),
        Ok(None) => {}
        Err(error) => return super::internal(error.into()),
    }
    let users = match active_user_count(&state) {
        Ok(value) => value,
        Err(error) => return super::internal(error.into()),
    };
    let next = query
        .get("next")
        .filter(|value| safe_next(value))
        .map(String::as_str)
        .unwrap_or("/devices");
    Html(crate::page_html::auth(crate::page_html::AuthPage::Login {
        next,
        signup: public_signup_configured(&state, users),
    }))
    .into_response()
}

pub(super) async fn signup(State(state): State<AppState>, headers: HeaderMap) -> Response {
    let users = match active_user_count(&state) {
        Ok(value) => value,
        Err(error) => return super::internal(error.into()),
    };
    if users == 0
        || !state.config.public_signup
        || state.config.turnstile_site_key.is_none()
        || state.config.turnstile_secret_key.is_none()
    {
        return super::redirect("/login");
    }
    match browser_user(&state, &headers) {
        Ok(Some(_)) => return super::redirect("/devices"),
        Ok(None) => {}
        Err(error) => return super::internal(error.into()),
    }
    let site_key = state
        .config
        .turnstile_site_key
        .as_deref()
        .unwrap_or_default();
    Html(crate::page_html::auth(crate::page_html::AuthPage::Signup {
        site_key,
    }))
    .into_response()
}

pub(super) async fn setup_link(
    State(state): State<AppState>,
    Path(token): Path<String>,
) -> Response {
    let users = match active_user_count(&state) {
        Ok(value) => value,
        Err(error) => return super::internal(error.into()),
    };
    if users > 0 {
        return super::redirect("/");
    }
    let Some(expected) = state.config.setup_token.as_deref() else {
        return super::redirect("/");
    };
    if hash(&token) != hash(expected) {
        return (StatusCode::FORBIDDEN, "invalid setup link").into_response();
    }
    let secure = if state.config.public_url.starts_with("https://") {
        "; Secure"
    } else {
        ""
    };
    let setup_cookie = hash(&token);
    let cookie =
        format!("rc_setup={setup_cookie}; Path=/; HttpOnly; SameSite=Strict; Max-Age=900{secure}");
    let mut response = super::redirect("/");
    let value = match HeaderValue::from_str(&cookie) {
        Ok(value) => value,
        Err(error) => return super::internal(error.into()),
    };
    response.headers_mut().insert("set-cookie", value);
    response
}

fn valid_invite(state: &AppState, token: &str) -> rusqlite::Result<bool> {
    state.db.with_connection(|db| {
        db.query_row(
            "SELECT 1 FROM workspace_invites WHERE token_hash=? AND used_at IS NULL AND expires_at>?",
            rusqlite::params![hash(token),now_ms()], |_| Ok(()),
        ).optional().map(|value| value.is_some())
    })
}

fn setup_authorized(state: &AppState, headers: &HeaderMap) -> bool {
    let Some(expected) = state.config.setup_token.as_deref() else {
        return true;
    };
    headers
        .get("cookie")
        .and_then(|value| value.to_str().ok())
        .is_some_and(|cookies| {
            cookies
                .split(';')
                .filter_map(|part| part.trim().split_once('='))
                .any(|(key, value)| key == "rc_setup" && value == hash(expected))
        })
}

fn safe_next(value: &str) -> bool {
    value.starts_with('/') && !value.starts_with("//")
}

fn public_signup_configured(state: &AppState, active_users: i64) -> bool {
    active_users > 0
        && state.config.public_signup
        && state.config.turnstile_site_key.is_some()
        && state.config.turnstile_secret_key.is_some()
}
