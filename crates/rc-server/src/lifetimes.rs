pub const WEB_DEFAULT_LIFETIME: &str = "30d";
pub const CONTROL_DEFAULT_LIFETIME: &str = "30d";
pub const CLI_DEFAULT_LIFETIME: &str = "never";
pub const API_DEFAULT_LIFETIME: &str = "never";
pub const MCP_DEFAULT_LIFETIME: &str = "never";
pub const MAX_FINITE_AUTH_LIFETIME_MS: i64 = 366 * 24 * 60 * 60 * 1000;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Lifetime {
    pub expires_at: i64,
    pub max_age: Option<i64>,
}

pub fn auth_lifetime(
    value: Option<&str>,
    fallback: &str,
    allow_never: bool,
    now: i64,
) -> Result<Lifetime, &'static str> {
    let selected = value
        .filter(|value| !value.trim().is_empty())
        .unwrap_or(fallback);
    if selected == "never" {
        if !allow_never {
            return Err("invalid authorization lifetime");
        }
        return Ok(Lifetime {
            expires_at: 0,
            max_age: None,
        });
    }
    let seconds = match selected {
        "1h" => 60 * 60,
        "1d" => 24 * 60 * 60,
        "7d" => 7 * 24 * 60 * 60,
        "30d" => 30 * 24 * 60 * 60,
        "90d" => 90 * 24 * 60 * 60,
        "180d" => 180 * 24 * 60 * 60,
        "1y" => 365 * 24 * 60 * 60,
        _ => return Err("invalid authorization lifetime"),
    };
    Ok(Lifetime {
        expires_at: now + seconds * 1000,
        max_age: Some(seconds),
    })
}
