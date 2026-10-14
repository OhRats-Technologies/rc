use super::*;

const REDIRECT: &str = "https://oauth-redirect.googleusercontent.com/r/user_bound_custom-mcp-test";

fn fixture() -> anyhow::Result<(AppState, rusqlite::Connection)> {
    let root = temp_root()?;
    let path = root.join("rc.sqlite3");
    let state = AppState::new(test_config(&root, &path))?;
    seed(
        &path,
        "client",
        REDIRECT,
        "http://localhost/mcp",
        &"a".repeat(43),
        "code",
    )?;
    let db = rusqlite::Connection::open(path)?;
    db.execute(
        "INSERT INTO sessions(token_hash,user_id,kind,created_at,expires_at) VALUES(?,'mcp-user','browser',?,?)",
        rusqlite::params![hash("browser-session"), now_ms(), now_ms() + 60_000],
    )?;
    Ok((state, db))
}

fn query(state: &str) -> String {
    url::form_urlencoded::Serializer::new(String::new())
        .extend_pairs([
            ("response_type", "code"),
            ("client_id", "client"),
            ("redirect_uri", REDIRECT),
            ("scope", "mcp:observe mcp:terminal"),
            ("resource", "http://localhost/mcp"),
            ("code_challenge", &"a".repeat(43)),
            ("code_challenge_method", "S256"),
            ("state", state),
        ])
        .finish()
}

#[tokio::test]
async fn long_opaque_state_survives_login_consent_switch_and_denial() -> anyhow::Result<()> {
    let (state, db) = fixture()?;
    let application = app(state);
    for opaque in [
        "AOtJLdGvd0_-".repeat(128),
        format!("{} +/%&=?é", "x".repeat(4096)),
    ] {
        let raw = query(&opaque);
        let response = application
            .clone()
            .oneshot(Request::get(format!("/oauth/authorize?{raw}")).body(Body::empty())?)
            .await?;
        assert_eq!(response.status(), StatusCode::SEE_OTHER);
        let login = url::Url::parse(&format!(
            "http://localhost{}",
            response.headers()[header::LOCATION].to_str()?
        ))?;
        let next = login
            .query_pairs()
            .find(|(key, _)| key == "next")
            .unwrap()
            .1
            .into_owned();
        assert_eq!(next, format!("/oauth/authorize?{raw}"));

        for action in ["switch-account", "cancel"] {
            let response = application
                .clone()
                .oneshot(
                    Request::get(&next)
                        .header(header::COOKIE, "rc_session=browser-session")
                        .body(Body::empty())?,
                )
                .await?;
            assert_eq!(response.status(), StatusCode::OK);
            let (id, stored): (String, String) =
                db.query_row("SELECT id,state FROM mcp_requests", [], |row| {
                    Ok((row.get(0)?, row.get(1)?))
                })?;
            assert_eq!(stored, opaque);
            let response = application
                .clone()
                .oneshot(
                    Request::post(format!("/oauth/authorize/{action}"))
                        .header(header::COOKIE, "rc_session=browser-session")
                        .header(header::CONTENT_TYPE, "application/json")
                        .body(Body::from(serde_json::json!({"requestId":id}).to_string()))?,
                )
                .await?;
            assert_eq!(response.status(), StatusCode::OK);
            let body = response_json(response).await?;
            let target = body["redirect"].as_str().unwrap();
            let redirect = if action == "switch-account" {
                let login = url::Url::parse(&format!("http://localhost{target}"))?;
                let next = login
                    .query_pairs()
                    .find(|(key, _)| key == "next")
                    .unwrap()
                    .1
                    .into_owned();
                db.execute("INSERT INTO sessions(token_hash,user_id,kind,created_at,expires_at) VALUES(?,'mcp-user','browser',?,?)",
                    rusqlite::params![hash("browser-session"),now_ms(),now_ms()+60_000])?;
                url::Url::parse(&format!("http://localhost{next}"))?
            } else {
                let redirect = url::Url::parse(target)?;
                assert!(
                    redirect
                        .query_pairs()
                        .any(|(key, value)| key == "error" && value == "access_denied")
                );
                redirect
            };
            assert_eq!(
                redirect
                    .query_pairs()
                    .find(|(key, _)| key == "state")
                    .unwrap()
                    .1,
                opaque
            );
        }
    }
    Ok(())
}

#[tokio::test]
async fn oversized_authorization_queries_are_rejected_before_login_or_storage() -> anyhow::Result<()>
{
    let (state, db) = fixture()?;
    let raw = format!("{}&extra={}", query("small"), "x".repeat(8192));
    let user = rc_server::UserIdentity {
        id: "mcp-user".into(),
        name: "User".into(),
    };
    assert!(rc_server::create_oauth_request(&state, &user, &raw).is_err());
    let application = app(state);
    for cookie in ["", "rc_session=browser-session"] {
        let response = application
            .clone()
            .oneshot(
                Request::get(format!("/oauth/authorize?{raw}"))
                    .header(header::COOKIE, cookie)
                    .body(Body::empty())?,
            )
            .await?;
        assert_eq!(response.status(), StatusCode::URI_TOO_LONG);
        assert!(response.headers().get(header::LOCATION).is_none());
    }
    assert_eq!(
        db.query_row("SELECT count(*) FROM mcp_requests", [], |row| row
            .get::<_, i64>(0))?,
        0
    );
    Ok(())
}
