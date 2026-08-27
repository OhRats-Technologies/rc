#[path = "passkeys_http/support.rs"]
mod support;

use axum::http::{StatusCode, header};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::SigningKey;
use rc_server::app;
use support::{delete_json, get_json, json_request, json_request_headers, temp_root, test_state};
use webauthn_authenticator_rs::{
    prelude::{CreationChallengeResponse, RequestChallengeResponse, Url, WebauthnAuthenticator},
    softpasskey::SoftPasskey,
};

#[tokio::test]
async fn setup_login_and_step_up_work_with_a_real_webauthn_authenticator() -> anyhow::Result<()> {
    let _ = tracing_subscriber::fmt::try_init();
    let root = temp_root()?;
    let state = test_state(&root)?;
    let application = app(state.clone());
    let origin = Url::parse("http://localhost")?;
    let mut authenticator = WebauthnAuthenticator::new(SoftPasskey::new(true));

    let setup = json_request(
        &application,
        "/api/v1/auth/setup/options",
        serde_json::json!({"name":"Test User"}),
        None,
    )
    .await?;
    assert_eq!(setup.status, StatusCode::OK);
    let creation: CreationChallengeResponse = serde_json::from_value(serde_json::json!({
        "publicKey": setup.body["options"].clone()
    }))?;
    let credential = authenticator.do_registration(origin.clone(), creation)?;
    let verified = json_request(
        &application,
        "/api/v1/auth/setup/verify",
        serde_json::json!({
            "ceremonyId": setup.body["ceremonyId"],
            "response": credential,
        }),
        None,
    )
    .await?;
    assert_eq!(verified.status, StatusCode::CREATED);
    let first_cookie = verified.cookie()?;

    let me = get_json(&application, "/api/v1/me", Some(&first_cookie)).await?;
    assert_eq!(me.status, StatusCode::OK);
    assert_eq!(me.body["user"]["name"], "Test User");
    assert_eq!(me.body["workspaces"].as_array().map(Vec::len), Some(1));
    let user_id = me.body["user"]["id"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("missing setup user id"))?;

    let login = json_request(
        &application,
        "/api/v1/auth/login/options",
        serde_json::json!({}),
        None,
    )
    .await?;
    let request: RequestChallengeResponse = serde_json::from_value(serde_json::json!({
        "publicKey": login.body["options"].clone()
    }))?;
    let assertion = authenticator.do_authentication(origin.clone(), request)?;
    let assertion = serde_json::to_value(assertion)?;
    let logged_in = json_request(
        &application,
        "/api/v1/auth/login/verify",
        serde_json::json!({
            "ceremonyId": login.body["ceremonyId"],
            "response": assertion.clone(),
            "lifetime": "1d",
        }),
        None,
    )
    .await?;
    assert_eq!(logged_in.status, StatusCode::OK);
    let cookie = logged_in.cookie()?;

    let collision_client = "collision-control-client";
    let collision_public = URL_SAFE_NO_PAD.encode([29_u8; 32]);
    rusqlite::Connection::open(root.join("rc.sqlite3"))?.execute(
        "INSERT INTO clients(id,user_id,kind,name,public_key,scopes,created_at,expires_at) VALUES(?,?,'api','Existing API',?,'[\"read\"]',?,0)",
        rusqlite::params![collision_client,user_id,collision_public,rc_server::now_ms()],
    )?;
    let collision_signing = SigningKey::from_bytes(&[10_u8; 32]);
    let collision_start = json_request(
        &application,
        "/api/v1/control/authorize/options",
        serde_json::json!({
            "clientId": collision_client,
            "signingPublicKey": URL_SAFE_NO_PAD.encode(collision_signing.verifying_key().as_bytes()),
            "lifetime": "30d",
        }),
        Some(&cookie),
    )
    .await?;
    assert_eq!(collision_start.status, StatusCode::OK);
    let request: RequestChallengeResponse = serde_json::from_value(serde_json::json!({
        "publicKey": collision_start.body["options"].clone()
    }))?;
    let assertion = authenticator.do_authentication(origin.clone(), request)?;
    let collision_verified = json_request(
        &application,
        "/api/v1/control/authorize/verify",
        serde_json::json!({
            "authorizationId": collision_start.body["authorizationId"],
            "response": assertion,
        }),
        Some(&cookie),
    )
    .await?;
    assert_eq!(collision_verified.status, StatusCode::UNAUTHORIZED);
    let collision_row: (String, String) = rusqlite::Connection::open(root.join("rc.sqlite3"))?
        .query_row(
            "SELECT kind,public_key FROM clients WHERE id=?",
            [collision_client],
            |row| Ok((row.get(0)?, row.get(1)?)),
        )?;
    assert_eq!(collision_row, ("api".into(), collision_public));

    let control_signing = SigningKey::from_bytes(&[11_u8; 32]);
    let control_client = "browser-control-test";
    let control_start = json_request(
        &application,
        "/api/v1/control/authorize/options",
        serde_json::json!({
            "clientId": control_client,
            "signingPublicKey": URL_SAFE_NO_PAD.encode(control_signing.verifying_key().as_bytes()),
            "lifetime": "30d",
        }),
        Some(&cookie),
    )
    .await?;
    assert_eq!(control_start.status, StatusCode::OK);
    let request: RequestChallengeResponse = serde_json::from_value(serde_json::json!({
        "publicKey": control_start.body["options"].clone()
    }))?;
    let assertion = authenticator.do_authentication(origin.clone(), request)?;
    let control_verified = json_request(
        &application,
        "/api/v1/control/authorize/verify",
        serde_json::json!({
            "authorizationId": control_start.body["authorizationId"],
            "response": assertion,
        }),
        Some(&cookie),
    )
    .await?;
    assert_eq!(control_verified.status, StatusCode::CREATED);
    let control_status = get_json(
        &application,
        &format!("/api/v1/control/clients/{control_client}"),
        Some(&cookie),
    )
    .await?;
    assert_eq!(control_status.status, StatusCode::OK);
    assert_eq!(control_status.body["authorized"], true);

    let replay = json_request(
        &application,
        "/api/v1/auth/login/verify",
        serde_json::json!({
            "ceremonyId": login.body["ceremonyId"],
            "response": assertion,
            "lifetime": "1d",
        }),
        None,
    )
    .await?;
    assert_eq!(replay.status, StatusCode::UNAUTHORIZED);

    let step = json_request(
        &application,
        "/api/v1/auth/step-up/options",
        serde_json::json!({}),
        Some(&cookie),
    )
    .await?;
    let request: RequestChallengeResponse = serde_json::from_value(serde_json::json!({
        "publicKey": step.body["options"].clone()
    }))?;
    let assertion = authenticator.do_authentication(origin.clone(), request)?;
    let stepped = json_request(
        &application,
        "/api/v1/auth/step-up/verify",
        serde_json::json!({
            "authorizationId": step.body["authorizationId"],
            "response": assertion,
        }),
        Some(&cookie),
    )
    .await?;
    assert_eq!(stepped.status, StatusCode::OK);
    assert!(
        stepped.body["token"]
            .as_str()
            .is_some_and(|value| !value.is_empty())
    );
    let step_token = stepped.body["token"].as_str().unwrap_or_default();
    let api_signing = SigningKey::from_bytes(&[17_u8; 32]);
    let created = json_request_headers(
        &application,
        "/api/v1/tokens",
        serde_json::json!({
            "name": "Test API",
            "scopes": ["read", "execute"],
            "publicKey": URL_SAFE_NO_PAD.encode(api_signing.verifying_key().as_bytes()),
            "lifetime": "30d",
        }),
        Some(&cookie),
        &[("x-rc-step-up", step_token)],
    )
    .await?;
    assert_eq!(created.status, StatusCode::OK);
    assert_eq!(
        created.body["scopes"],
        serde_json::json!(["read", "execute"])
    );
    let replayed_step = json_request_headers(
        &application,
        "/api/v1/tokens",
        serde_json::json!({
            "name": "Should fail",
            "publicKey": URL_SAFE_NO_PAD.encode(api_signing.verifying_key().as_bytes()),
        }),
        Some(&cookie),
        &[("x-rc-step-up", step_token)],
    )
    .await?;
    assert_eq!(replayed_step.status, StatusCode::UNAUTHORIZED);

    let delete_step = json_request(
        &application,
        "/api/v1/auth/step-up/options",
        serde_json::json!({}),
        Some(&cookie),
    )
    .await?;
    let request: RequestChallengeResponse = serde_json::from_value(serde_json::json!({
        "publicKey": delete_step.body["options"].clone()
    }))?;
    let assertion = authenticator.do_authentication(origin, request)?;
    let delete_token = json_request(
        &application,
        "/api/v1/auth/step-up/verify",
        serde_json::json!({
            "authorizationId": delete_step.body["authorizationId"],
            "response": assertion,
        }),
        Some(&cookie),
    )
    .await?;
    let deleted = delete_json(
        &application,
        "/api/v1/account",
        Some(&cookie),
        &[(
            "x-rc-step-up",
            delete_token.body["token"].as_str().unwrap_or_default(),
        )],
    )
    .await?;
    assert_eq!(deleted.status, StatusCode::OK);
    assert_eq!(deleted.body["ok"], true);
    assert!(
        deleted
            .headers
            .get(header::SET_COOKIE)
            .and_then(|value| value.to_str().ok())
            .is_some_and(|value| value.contains("Max-Age=0"))
    );
    let status = get_json(&application, "/api/v1/status", None).await?;
    assert_eq!(status.status, StatusCode::OK);
    assert_eq!(status.body["setupRequired"], true);
    assert_eq!(status.body["publicSignup"], false);
    Ok(())
}
