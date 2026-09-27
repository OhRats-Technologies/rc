use super::{ENROLLMENT_TTL_MS, owner, principal};
use crate::auth_public_routes::ApiError;
use crate::{AppState, hash, now_ms, opaque};
use axum::{
    Json,
    extract::{Path, State},
    http::{HeaderMap, Method, StatusCode},
};
use uuid::Uuid;

pub(super) async fn enrollment(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<(StatusCode, Json<serde_json::Value>), ApiError> {
    let path = format!("/api/v1/workspaces/{id}/enrollments");
    let principal = principal(
        &state,
        &headers,
        &Method::POST,
        &path,
        &[],
        Some("manage-devices"),
    )?;
    owner(&state, &principal.user.id, &id)?;
    let count = state.db.with_connection(|db| {
        db.query_row(
            "SELECT count(*) FROM devices WHERE workspace_id=?",
            [&id],
            |row| row.get::<_, i64>(0),
        )
    })?;
    if count >= 25 {
        return Err(ApiError::conflict("device limit reached (25)"));
    }
    let token = format!("enroll_{}", opaque(24));
    let enrollment_id = Uuid::new_v4().to_string();
    let expires = now_ms() + ENROLLMENT_TTL_MS;
    state.db.with_connection(|db| {
        db.execute(
            "INSERT INTO enrollment_tokens(id,workspace_id,token_hash,created_by,created_at,expires_at) VALUES(?,?,?,?,?,?)",
            rusqlite::params![enrollment_id,id,hash(&token),principal.user.id,now_ms(),expires],
        )?;
        Ok(())
    })?;
    Ok((
        StatusCode::CREATED,
        Json(serde_json::json!({
            "token":token,"expiresAt":expires,
            "install":install_command(&state.config.public_url, &token),
            "enroll":enroll_command(&state.config.public_url, &token),
            "installWindows":windows_install_command(&state.config.public_url, &token),
            "enrollWindows":windows_enroll_command(&state.config.public_url, &token)
        })),
    ))
}

fn enroll_command(public_url: &str, token: &str) -> String {
    let server = public_url.trim_end_matches('/');
    format!(
        "rc enroll {} --url {} && rc service install",
        shell_quote(token),
        shell_quote(server)
    )
}

fn install_command(public_url: &str, token: &str) -> String {
    let server = public_url.trim_end_matches('/');
    let script = format!("{server}/install.sh");
    format!(
        "curl -fsSL {} | sh -s -- {} {}",
        shell_quote(&script),
        shell_quote(token),
        shell_quote(server)
    )
}

fn shell_quote(value: &str) -> String {
    format!("'{}'", value.replace('\'', "'\"'\"'"))
}

fn powershell_quote(value: &str) -> String {
    format!("'{}'", value.replace('\'', "''"))
}

fn windows_install_command(public_url: &str, token: &str) -> String {
    let server = public_url.trim_end_matches('/');
    format!(
        "& ([scriptblock]::Create((Invoke-RestMethod -Uri {}))) -Token {} -Server {}",
        powershell_quote(&format!("{server}/install.ps1")),
        powershell_quote(token),
        powershell_quote(server)
    )
}

fn windows_enroll_command(public_url: &str, token: &str) -> String {
    format!(
        r"$rc = Join-Path $env:LOCALAPPDATA 'OhRats\RC\bin\rc.exe'; & $rc enroll {} --url {}; if ($LASTEXITCODE -eq 0) {{ & $rc service install }}",
        powershell_quote(token),
        powershell_quote(public_url.trim_end_matches('/'))
    )
}

#[cfg(test)]
mod tests {
    use super::{enroll_command, install_command};

    #[test]
    fn self_hosted_install_persists_originating_server() {
        assert_eq!(
            install_command("https://rc.example/", "enroll_test"),
            "curl -fsSL 'https://rc.example/install.sh' | sh -s -- 'enroll_test' 'https://rc.example'"
        );
    }

    #[test]
    fn install_command_quotes_shell_metacharacters() {
        assert_eq!(
            install_command("https://rc.example/o'hare", "enroll_$(false)"),
            "curl -fsSL 'https://rc.example/o'\"'\"'hare/install.sh' | sh -s -- 'enroll_$(false)' 'https://rc.example/o'\"'\"'hare'"
        );
    }

    #[test]
    fn installed_node_gets_a_separate_enroll_command() {
        assert_eq!(
            enroll_command("https://rc.example/", "enroll_test"),
            "rc enroll 'enroll_test' --url 'https://rc.example' && rc service install"
        );
    }
}
