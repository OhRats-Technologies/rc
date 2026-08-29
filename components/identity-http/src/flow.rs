use crate::{
    AUTH_SCRIPT_PATH, config,
    ohrats::{
        rc_http::types::{Request, Response},
        rc_identity::{credentials, users},
        rc_session::{lookup, management},
    },
    pages, request, response, time, webauthn,
};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use serde::Deserialize;
use serde_json::Value;

const AUTH_SCRIPT: &[u8] = include_bytes!("../assets/auth.js");

pub fn handle(request_value: Request) -> Result<Option<Response>, String> {
    let method = request_value.method.as_str();
    let path = request_value.path.as_str();
    if path == AUTH_SCRIPT_PATH && matches!(method, "GET" | "HEAD") {
        return Ok(Some(head(
            request_value.method == "HEAD",
            response::javascript(AUTH_SCRIPT),
        )));
    }
    let result = match (method, path) {
        ("GET" | "HEAD", "/") => home(&request_value)?,
        ("GET" | "HEAD", "/login") => Some(login_page(&request_value)?),
        ("GET" | "HEAD", path) if path.starts_with("/setup/") => {
            Some(setup_link(&request_value, &path[7..])?)
        }
        ("GET", "/api/v1/status") => Some(status(&request_value)?),
        ("POST", "/api/v1/auth/setup/options") => Some(setup_options(&request_value)?),
        ("POST", "/api/v1/auth/setup/verify") => Some(setup_verify(&request_value)?),
        ("POST", "/api/v1/auth/login/options") => Some(login_options(&request_value)?),
        ("POST", "/api/v1/auth/login/verify") => Some(login_verify(&request_value)?),
        ("POST", "/account/logout") => Some(logout(&request_value)?),
        _ => None,
    };
    Ok(result.map(|value| head(request_value.method == "HEAD", value)))
}

fn home(request_value: &Request) -> Result<Option<Response>, String> {
    if users::count()? == 0 {
        let authorized = config::setup_authorized(request::cookie_header(request_value))?;
        return Ok(Some(response::html(200, pages::setup(authorized))));
    }
    if lookup::find(request::cookie_header(request_value))?.is_some() {
        return Ok(Some(response::redirect("/devices")));
    }
    Ok(None)
}

fn login_page(request_value: &Request) -> Result<Response, String> {
    if users::count()? == 0 {
        return Ok(response::redirect("/"));
    }
    if lookup::find(request::cookie_header(request_value))?.is_some() {
        return Ok(response::redirect("/devices"));
    }
    let query = request::query(request_value);
    let next = query
        .get("next")
        .filter(|value| request::safe_next(value))
        .map(String::as_str)
        .unwrap_or("/devices");
    Ok(response::html(200, pages::login(next)))
}

fn setup_link(request_value: &Request, token: &str) -> Result<Response, String> {
    if users::count()? > 0 {
        return Ok(response::redirect("/"));
    }
    let rp = config::relying_party(request_value)?;
    match config::setup_cookie(token, rp.secure) {
        Ok(Some(cookie)) => Ok(response::with_cookie(response::redirect("/"), cookie)),
        Ok(None) => Ok(response::redirect("/")),
        Err(_) => response::error(403, "invalid setup link"),
    }
}

fn status(request_value: &Request) -> Result<Response, String> {
    let count = users::count()?;
    let required = count == 0;
    response::json(
        200,
        serde_json::json!({
            "setupRequired": required,
            "setupAuthorized": required && config::setup_authorized(request::cookie_header(request_value))?,
            "publicSignup": false,
            "version": env!("CARGO_PKG_VERSION"),
        }),
    )
}

#[derive(Deserialize)]
struct NameInput {
    name: String,
}

fn setup_options(request_value: &Request) -> Result<Response, String> {
    if users::count()? > 0 {
        return response::error(409, "setup already completed");
    }
    if !config::setup_authorized(request::cookie_header(request_value))? {
        return response::error(403, "Open the RC setup link first.");
    }
    let input: NameInput = request::json(request_value)?;
    let name = clean_name(&input.name)?;
    let user_id = random_id()?;
    let rp = config::relying_party(request_value)?;
    let (ceremony_id, options) = webauthn::begin_registration("setup", &user_id, &name, &rp)?;
    response::json(
        200,
        serde_json::json!({ "ceremonyId": ceremony_id, "options": options }),
    )
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct VerifyInput {
    ceremony_id: String,
    response: Value,
}

fn setup_verify(request_value: &Request) -> Result<Response, String> {
    if users::count()? > 0 {
        return response::error(409, "setup already completed");
    }
    let input: VerifyInput = request::json(request_value)?;
    let registration =
        match webauthn::finish_registration("setup", &input.ceremony_id, input.response) {
            Ok(value) => value,
            Err(error) => {
                log_auth_failure("registration", &error);
                return response::error(401, "passkey verification failed");
            }
        };
    let user = credentials::create_user(
        &registration.user_id,
        &registration.display_name,
        "Passkey",
        &registration.credential,
    )?;
    issue_session(request_value, &user.id, "30d", 201)
}

fn login_options(request_value: &Request) -> Result<Response, String> {
    let rp = config::relying_party(request_value)?;
    match webauthn::begin_login(&rp) {
        Ok((ceremony_id, options)) => response::json(
            200,
            serde_json::json!({ "ceremonyId": ceremony_id, "options": options }),
        ),
        Err(error) => response::error(401, &error),
    }
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct LoginVerify {
    ceremony_id: String,
    response: Value,
    lifetime: Option<String>,
}

fn login_verify(request_value: &Request) -> Result<Response, String> {
    let input: LoginVerify = request::json(request_value)?;
    let user_id = match webauthn::finish_login(&input.ceremony_id, input.response) {
        Ok(value) => value,
        Err(error) => {
            log_auth_failure("authentication", &error);
            return response::error(401, "passkey verification failed");
        }
    };
    issue_session(
        request_value,
        &user_id,
        input.lifetime.as_deref().unwrap_or("30d"),
        200,
    )
}

fn logout(request_value: &Request) -> Result<Response, String> {
    management::revoke(request::cookie_header(request_value))?;
    let secure = config::relying_party(request_value)?.secure;
    Ok(response::with_cookie(
        response::redirect("/"),
        response::clear_session_cookie(secure),
    ))
}

fn issue_session(
    request_value: &Request,
    user_id: &str,
    lifetime: &str,
    status: u16,
) -> Result<Response, String> {
    let seconds = lifetime_seconds(lifetime)?;
    let expires_at_ms = time::now_ms().saturating_add(seconds.saturating_mul(1000));
    let issued = management::issue(user_id, expires_at_ms)?;
    let secure = config::relying_party(request_value)?.secure;
    let value = response::json(
        status,
        serde_json::json!({ "ok": true, "expiresAt": expires_at_ms }),
    )?;
    Ok(response::with_cookie(
        value,
        response::session_cookie(&issued.token, seconds, secure),
    ))
}

fn clean_name(value: &str) -> Result<String, String> {
    let value = value.trim().chars().take(120).collect::<String>();
    if value.is_empty() || value.chars().any(char::is_control) {
        Err("name required".into())
    } else {
        Ok(value)
    }
}

fn lifetime_seconds(value: &str) -> Result<u64, String> {
    match value {
        "1h" => Ok(60 * 60),
        "1d" => Ok(24 * 60 * 60),
        "7d" => Ok(7 * 24 * 60 * 60),
        "30d" => Ok(30 * 24 * 60 * 60),
        "90d" => Ok(90 * 24 * 60 * 60),
        "180d" => Ok(180 * 24 * 60 * 60),
        "1y" => Ok(365 * 24 * 60 * 60),
        _ => Err("invalid authorization lifetime".into()),
    }
}

fn random_id() -> Result<String, String> {
    let mut bytes = [0; 16];
    getrandom::fill(&mut bytes).map_err(display)?;
    Ok(URL_SAFE_NO_PAD.encode(bytes))
}

fn head(value: bool, mut response: Response) -> Response {
    if value {
        response.body.clear();
    }
    response
}

fn log_auth_failure(kind: &str, error: &str) {
    use crate::ohrats::rc_plugin::{host, types::LogLevel};
    let bounded = error.chars().take(240).collect::<String>();
    host::log(
        LogLevel::Warn,
        &format!("WebAuthn {kind} failed: {bounded}"),
    );
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}

#[cfg(test)]
mod tests {
    use super::{clean_name, lifetime_seconds};

    #[test]
    fn validates_names_and_browser_lifetimes() {
        assert_eq!(clean_name("  Fern  ").as_deref(), Ok("Fern"));
        assert!(clean_name("\n").is_err());
        assert_eq!(lifetime_seconds("30d"), Ok(2_592_000));
        assert!(lifetime_seconds("never").is_err());
    }
}
