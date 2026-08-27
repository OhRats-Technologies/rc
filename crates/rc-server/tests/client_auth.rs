use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::SigningKey;
use rc_crypto::sign_api_seed;
use rc_server::{AppState, ClientHttpError, Config, now_ms, verify_client_request};
use std::{net::SocketAddr, path::PathBuf};
use uuid::Uuid;

#[test]
fn signed_client_auth_binds_request_and_rejects_replay() -> anyhow::Result<()> {
    let root = std::env::temp_dir().join(format!("rc-client-auth-{}", Uuid::new_v4()));
    std::fs::create_dir_all(&root)?;
    let _cleanup = Cleanup(root.clone());
    let db_path = root.join("rc.sqlite3");
    let state = AppState::new(Config {
        listen: SocketAddr::from(([127, 0, 0, 1], 0)),
        data_dir: root,
        db_path: db_path.clone(),
        public_url: "http://localhost".into(),
        static_dir: std::path::PathBuf::from("dist/assets"),
        trust_proxy: false,
        setup_token: None,
        public_signup: false,
        turnstile_site_key: None,
        turnstile_secret_key: None,
        turn_token_id: None,
        turn_api_token: None,
        ssh_daemon_port: 2222,
        ssh_internal_port: 3001,
        mcp_access_ttl_minutes: 15,
    })?;

    let signing = SigningKey::from_bytes(&[7_u8; 32]);
    let signing_seed = URL_SAFE_NO_PAD.encode(signing.to_bytes());
    let client_id = "cli-client";
    let user_id = "user";
    let db = rusqlite::Connection::open(db_path)?;
    db.execute("PRAGMA foreign_keys=ON", [])?;
    db.execute(
        "INSERT INTO users(id,name,created_at) VALUES(?,?,?)",
        rusqlite::params![user_id, "Test User", now_ms()],
    )?;
    db.execute(
        "INSERT INTO clients(id,user_id,kind,name,public_key,scopes,created_at) VALUES(?,?,?,?,?,?,?)",
        rusqlite::params![
            client_id,
            user_id,
            "cli",
            "Test CLI",
            URL_SAFE_NO_PAD.encode(signing.verifying_key().as_bytes()),
            "[]",
            now_ms()
        ],
    )?;

    let body = br#"{"deviceId":"device"}"#;
    let method = axum::http::Method::POST;
    let path = "/api/v1/control/challenge";
    let timestamp = (now_ms() / 1000).to_string();
    let nonce = "abcdefghijklmnopqrstuvwx";
    let signature = sign_api_seed(
        &signing_seed,
        client_id,
        &timestamp,
        nonce,
        method.as_str(),
        path,
        body,
    )?;
    let first_headers = signed_headers(client_id, &timestamp, nonce, &signature)?;
    let identity = verify_client_request(&state, &first_headers, &method, path, body)
        .map_err(|error| anyhow::anyhow!("client auth failed: {}", error.1))?;
    assert_eq!(identity.id, client_id);
    assert_eq!(identity.user_id, user_id);
    assert_eq!(identity.kind, "cli");
    assert!(identity.is_human_client());

    assert!(matches!(
        verify_client_request(&state, &first_headers, &method, path, body),
        Err(ClientHttpError(axum::http::StatusCode::CONFLICT, _))
    ));

    let second_nonce = "zyxwvutsrqponmlkjihgfedc";
    let second_signature = sign_api_seed(
        &signing_seed,
        client_id,
        &timestamp,
        second_nonce,
        method.as_str(),
        path,
        body,
    )?;
    let second = signed_headers(client_id, &timestamp, second_nonce, &second_signature)?;
    assert!(matches!(
        verify_client_request(&state, &second, &method, "/api/v1/control/open", body),
        Err(ClientHttpError(axum::http::StatusCode::UNAUTHORIZED, _))
    ));
    assert!(matches!(
        verify_client_request(&state, &second, &method, path, b"{}"),
        Err(ClientHttpError(axum::http::StatusCode::UNAUTHORIZED, _))
    ));
    Ok(())
}

fn signed_headers(
    client_id: &str,
    timestamp: &str,
    nonce: &str,
    signature: &str,
) -> anyhow::Result<axum::http::HeaderMap> {
    let mut headers = axum::http::HeaderMap::new();
    headers.insert("x-rc-key-id", client_id.parse()?);
    headers.insert("x-rc-timestamp", timestamp.parse()?);
    headers.insert("x-rc-nonce", nonce.parse()?);
    headers.insert("x-rc-signature", signature.parse()?);
    Ok(headers)
}

struct Cleanup(PathBuf);

impl Drop for Cleanup {
    fn drop(&mut self) {
        let _ = std::fs::remove_dir_all(&self.0);
    }
}
