use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::SigningKey;
use rand::rngs::OsRng;
use rc_api_client::{ApiClient, ApiError, Credential, public_post, random_url_bytes};
use rc_node::{
    AccountSession, DEFAULT_SERVER, load_account, load_config, resolve_state_dir, save_account,
};
use serde::Deserialize;
#[cfg(any(target_os = "macos", target_os = "linux"))]
use std::process::Command;
use std::{io, time::Duration};

pub fn defaults(url: Option<&str>, token: Option<&str>) -> Result<(String, Credential), ApiError> {
    let dir = resolve_state_dir(None);
    let config = load_config(&dir).unwrap_or_default();
    let account = load_account(&dir).unwrap_or_default();
    let server = url
        .map(str::to_owned)
        .or_else(|| env_nonempty("RC_URL"))
        .or_else(|| (!account.server.is_empty()).then_some(account.server.clone()))
        .or_else(|| (!config.server.is_empty()).then_some(config.server))
        .unwrap_or_else(|| DEFAULT_SERVER.into());
    let explicit = token
        .map(str::to_owned)
        .or_else(|| env_nonempty("RC_API_TOKEN"));
    let credential = if let Some(value) = explicit {
        Credential::parse(&value)?
    } else if account.server.trim_end_matches('/') == server.trim_end_matches('/')
        && !account.client_id.is_empty()
        && !account.signing_seed.is_empty()
    {
        Credential::from_signing_seed(&account.client_id, &account.signing_seed)?
    } else {
        return Err(rc_api_client::AuthError::Missing.into());
    };
    Ok((server, credential))
}

pub fn client(url: Option<&str>, token: Option<&str>) -> Result<ApiClient, ApiError> {
    let (server, credential) = defaults(url, token)?;
    ApiClient::new(&server, credential)
}

pub async fn login(url: Option<String>, expires: String) -> anyhow::Result<()> {
    #[derive(Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct Start {
        request_id: String,
        device_code: String,
        verification_url: String,
        expires_at: i64,
        interval: u64,
    }
    #[derive(Deserialize)]
    struct User {
        name: String,
    }
    #[derive(Deserialize)]
    struct Poll {
        pending: bool,
        user: Option<User>,
    }
    let dir = resolve_state_dir(None);
    let config = load_config(&dir).unwrap_or_default();
    let account = load_account(&dir).unwrap_or_default();
    let server = url
        .or_else(|| env_nonempty("RC_URL"))
        .or_else(|| (!account.server.is_empty()).then_some(account.server))
        .or_else(|| (!config.server.is_empty()).then_some(config.server))
        .unwrap_or_else(|| DEFAULT_SERVER.into());
    let signing = SigningKey::generate(&mut OsRng);
    let client_id = random_url_bytes(18);
    let start: Start = public_post(&server, "/api/v1/auth/cli/start", &serde_json::json!({
        "clientId": client_id, "signingPublicKey": URL_SAFE_NO_PAD.encode(signing.verifying_key().as_bytes()), "lifetime": expires,
    })).await?;
    println!(
        "Open this URL to authorize RC CLI:\n{}",
        start.verification_url
    );
    if open_browser(&start.verification_url).is_ok() {
        println!("Waiting for browser authorization…");
    }
    let interval = Duration::from_secs(start.interval.max(1));
    while unix_millis() < start.expires_at {
        let poll: Poll = public_post(
            &server,
            "/api/v1/auth/cli/poll",
            &serde_json::json!({ "requestId": start.request_id, "deviceCode": start.device_code }),
        )
        .await?;
        if !poll.pending {
            let user = poll.user.map(|value| value.name).unwrap_or_default();
            save_account(
                &dir,
                &AccountSession {
                    server: server.trim_end_matches('/').into(),
                    client_id,
                    signing_seed: URL_SAFE_NO_PAD.encode(signing.to_bytes()),
                    user: user.clone(),
                },
            )?;
            if user.is_empty() {
                println!("RC CLI authorized");
            } else {
                println!("Signed in as {user}");
            }
            return Ok(());
        }
        tokio::time::sleep(interval).await;
    }
    anyhow::bail!("CLI authorization expired")
}

pub async fn logout() -> anyhow::Result<()> {
    let dir = resolve_state_dir(None);
    let account = match load_account(&dir) {
        Ok(value) => value,
        Err(error) if error.kind() == io::ErrorKind::NotFound => {
            println!("RC CLI is not signed in");
            return Ok(());
        }
        Err(error) => return Err(error.into()),
    };
    if let Ok(credential) = Credential::from_signing_seed(&account.client_id, &account.signing_seed)
        && let Ok(client) = ApiClient::new(&account.server, credential)
    {
        let _ = client
            .request_empty(reqwest::Method::DELETE, "/api/v1/auth/cli/session")
            .await;
    }
    match std::fs::remove_file(rc_node::account_path(&dir)) {
        Ok(()) => {}
        Err(error) if error.kind() == io::ErrorKind::NotFound => {}
        Err(error) => return Err(error.into()),
    }
    println!("RC CLI signed out");
    Ok(())
}

fn open_browser(url: &str) -> io::Result<()> {
    if std::env::var_os("RC_NO_BROWSER").is_some() {
        return Err(io::Error::other("browser opening disabled"));
    }
    #[cfg(target_os = "macos")]
    let mut command = {
        let mut c = Command::new("open");
        c.arg(url);
        c
    };
    #[cfg(target_os = "linux")]
    let mut command = {
        let mut c = Command::new("xdg-open");
        c.arg(url);
        c
    };
    #[cfg(windows)]
    return open_browser_windows(url);
    #[cfg(not(any(target_os = "macos", target_os = "linux", windows)))]
    return Err(io::Error::other(format!("open {url} in your browser")));
    #[cfg(any(target_os = "macos", target_os = "linux"))]
    command.spawn().map(|_| ())
}

#[cfg(windows)]
fn open_browser_windows(url: &str) -> io::Result<()> {
    use windows::{
        Win32::{Foundation::HWND, UI::Shell::ShellExecuteW},
        core::PCWSTR,
    };
    let operation = windows_wide("open")?;
    let target = windows_wide(url)?;
    let result = unsafe {
        ShellExecuteW(
            Some(HWND::default()),
            PCWSTR(operation.as_ptr()),
            PCWSTR(target.as_ptr()),
            None,
            None,
            windows::Win32::UI::WindowsAndMessaging::SW_SHOWNORMAL,
        )
    };
    if result.0 as isize > 32 {
        Ok(())
    } else {
        Err(io::Error::other("Windows could not open the browser"))
    }
}

#[cfg(windows)]
fn windows_wide(value: &str) -> io::Result<Vec<u16>> {
    use std::os::windows::ffi::OsStrExt as _;
    let mut encoded: Vec<u16> = std::ffi::OsStr::new(value).encode_wide().collect();
    if encoded.contains(&0) {
        return Err(io::Error::new(
            io::ErrorKind::InvalidInput,
            "browser target contains a NUL",
        ));
    }
    encoded.push(0);
    Ok(encoded)
}

#[cfg(all(test, windows))]
mod windows_browser_tests {
    #[test]
    fn browser_target_is_unicode_and_nul_terminated() {
        let encoded = super::windows_wide("https://rc.example/é/🐀").unwrap();
        assert_eq!(encoded.last(), Some(&0));
        assert!(!encoded[..encoded.len() - 1].contains(&0));
        assert_eq!(
            String::from_utf16(&encoded[..encoded.len() - 1]).unwrap(),
            "https://rc.example/é/🐀"
        );
    }

    #[test]
    fn browser_target_rejects_interior_nul() {
        assert_eq!(
            super::windows_wide("https://rc.example/\0ignored")
                .unwrap_err()
                .kind(),
            std::io::ErrorKind::InvalidInput
        );
    }
}
fn env_nonempty(key: &str) -> Option<String> {
    std::env::var(key)
        .ok()
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
}
fn unix_millis() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}
