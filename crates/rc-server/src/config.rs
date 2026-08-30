use crate::ExecutionHistory;
use std::{env, net::SocketAddr, path::PathBuf};

#[derive(Debug, Clone)]
pub struct Config {
    pub listen: SocketAddr,
    pub data_dir: PathBuf,
    pub db_path: PathBuf,
    pub public_url: String,
    pub static_dir: PathBuf,
    pub trust_proxy: bool,
    pub setup_token: Option<String>,
    pub public_signup: bool,
    pub turnstile_site_key: Option<String>,
    pub turnstile_secret_key: Option<String>,
    pub turn_token_id: Option<String>,
    pub turn_api_token: Option<String>,
    pub ssh_daemon_port: u16,
    pub ssh_internal_port: u16,
    pub mcp_access_ttl_minutes: u64,
    pub execution_history: ExecutionHistory,
    pub execution_history_ttl_hours: u64,
}

impl Config {
    pub fn from_env() -> anyhow::Result<Self> {
        let port = env::var("PORT")
            .unwrap_or_else(|_| "3000".into())
            .parse::<u16>()?;
        let data_dir = PathBuf::from(env::var("DATA_DIR").unwrap_or_else(|_| "./data-v2".into()));
        std::fs::create_dir_all(&data_dir)?;
        secure_directory(&data_dir)?;
        let db_path = env::var_os("RC_DB_PATH")
            .map(PathBuf::from)
            .unwrap_or_else(|| data_dir.join("rc-v2.sqlite3"));
        Ok(Self {
            listen: SocketAddr::from(([0, 0, 0, 0], port)),
            data_dir,
            db_path,
            public_url: env::var("PUBLIC_URL")
                .unwrap_or_else(|_| format!("http://localhost:{port}")),
            static_dir: PathBuf::from(
                env::var("STATIC_DIR").unwrap_or_else(|_| "./dist/assets".into()),
            ),
            trust_proxy: env::var("RC_TRUST_PROXY").ok().as_deref() == Some("1"),
            setup_token: env_nonempty("RC_SETUP_TOKEN"),
            public_signup: env::var("RC_PUBLIC_SIGNUP").ok().as_deref() == Some("1"),
            turnstile_site_key: env_nonempty("RC_TURNSTILE_SITE_KEY"),
            turnstile_secret_key: env_nonempty("RC_TURNSTILE_SECRET_KEY"),
            turn_token_id: env_nonempty("RC_CF_TURN_TOKEN_ID"),
            turn_api_token: env_nonempty("RC_CF_TURN_API_TOKEN"),
            ssh_daemon_port: positive_u16("RC_SSH_DAEMON_PORT", 2222),
            ssh_internal_port: positive_u16("RC_SSH_INTERNAL_PORT", 3001),
            mcp_access_ttl_minutes: positive_u64("RC_MCP_ACCESS_TTL_MINUTES", 15),
            execution_history: ExecutionHistory::parse(
                &env::var("RC_EXECUTION_HISTORY").unwrap_or_else(|_| "none".into()),
            )?,
            execution_history_ttl_hours: positive_u64("RC_EXECUTION_HISTORY_TTL_HOURS", 168),
        })
    }
}

fn secure_directory(path: &std::path::Path) -> std::io::Result<()> {
    rc_platform::protect_private_path(path, true)
}

fn env_nonempty(key: &str) -> Option<String> {
    env::var(key)
        .ok()
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
}

fn positive_u16(key: &str, fallback: u16) -> u16 {
    env::var(key)
        .ok()
        .and_then(|value| value.parse().ok())
        .filter(|value| *value > 0)
        .unwrap_or(fallback)
}

fn positive_u64(key: &str, fallback: u64) -> u64 {
    env::var(key)
        .ok()
        .and_then(|value| value.parse().ok())
        .filter(|value| *value > 0)
        .unwrap_or(fallback)
}
