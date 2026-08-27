use rc_server::{AppState, Config, active_user_count, app, opaque, ssh_internal_app};
use std::time::Duration;
use tracing_subscriber::EnvFilter;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    match std::env::args().nth(1).as_deref() {
        Some("--version" | "version") => {
            println!("rc-server {}", env!("CARGO_PKG_VERSION"));
            return Ok(());
        }
        Some("--healthcheck") => return healthcheck().await,
        Some("--help" | "-h") => {
            println!("Usage: rc-server [--version|--healthcheck]");
            return Ok(());
        }
        Some(argument) => anyhow::bail!("unknown rc-server argument {argument:?}"),
        None => {}
    }
    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("rc_server=info")),
        )
        .init();
    let mut config = Config::from_env()?;
    let generated_setup_token = if config.setup_token.is_none() {
        let token = opaque(24);
        config.setup_token = Some(token.clone());
        Some(token)
    } else {
        None
    };
    let listen = config.listen;
    let state = AppState::new(config)?;
    let users = active_user_count(&state)?;
    if users == 0 {
        if let Some(token) = generated_setup_token {
            tracing::warn!(
                setup_url = %format!("{}/setup/{token}", state.config.public_url.trim_end_matches('/')),
                "RC setup is required; this one-time bootstrap link changes if the server restarts"
            );
        } else {
            tracing::warn!(
                "RC setup is required; open /setup/<RC_SETUP_TOKEN> on the configured PUBLIC_URL"
            );
        }
    }
    let ssh_state = state.clone();
    let ssh_internal_port = state.config.ssh_internal_port;
    tokio::spawn(async move {
        match tokio::net::TcpListener::bind(("127.0.0.1", ssh_internal_port)).await {
            Ok(listener) => {
                tracing::info!(port = ssh_internal_port, "RC SSH internal bridge listening");
                if let Err(error) = axum::serve(listener, ssh_internal_app(ssh_state)).await {
                    tracing::error!(%error, "RC SSH internal bridge stopped");
                }
            }
            Err(error) => tracing::error!(%error, "failed to bind RC SSH internal bridge"),
        }
    });
    let listener = tokio::net::TcpListener::bind(listen).await?;
    tracing::info!(%listen, "RC server listening");
    axum::serve(
        listener,
        app(state).into_make_service_with_connect_info::<std::net::SocketAddr>(),
    )
    .await?;
    Ok(())
}

async fn healthcheck() -> anyhow::Result<()> {
    let port = std::env::var("PORT")
        .unwrap_or_else(|_| "3000".into())
        .parse::<u16>()?;
    let response = reqwest::Client::builder()
        .timeout(Duration::from_secs(3))
        .build()?
        .get(format!("http://127.0.0.1:{port}/healthz"))
        .send()
        .await?;
    anyhow::ensure!(
        response.status() == reqwest::StatusCode::OK,
        "health endpoint returned {}",
        response.status()
    );
    anyhow::ensure!(
        response.text().await?.trim() == "ok",
        "unexpected health body"
    );
    Ok(())
}
