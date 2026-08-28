use rc_node::{NodeState, ServerTransport};
use rc_server::{AppState, Config, ControlHub, NewDevice, TurnProvider, app, now_ms};
use std::{net::SocketAddr, path::PathBuf, time::Duration};
use tokio::time::timeout;
use uuid::Uuid;

pub(super) struct Harness {
    root: PathBuf,
    server: tokio::task::JoinHandle<()>,
    pub(super) state: AppState,
    pub(super) seed: rusqlite::Connection,
    pub(super) user_id: String,
    pub(super) workspace_id: String,
    pub(super) device_id: String,
    pub(super) process_id: String,
    pub(super) node: NodeState,
    pub(super) base: String,
}

impl Harness {
    pub(super) async fn start() -> anyhow::Result<Self> {
        let root = std::env::temp_dir().join(format!("rc-node-webrtc-{}", Uuid::new_v4()));
        std::fs::create_dir_all(&root)?;
        let db_path = root.join("rc.sqlite3");
        let config = Config {
            listen: SocketAddr::from(([127, 0, 0, 1], 0)),
            data_dir: root.clone(),
            db_path: db_path.clone(),
            public_url: "http://localhost".into(),
            static_dir: PathBuf::from("dist/assets"),
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
            execution_history: rc_server::ExecutionHistory::None,
            execution_history_ttl_hours: 168,
        };
        let mut state = AppState::new(config)?;
        state.turn = TurnProvider::fixed(Vec::new());
        state.control = ControlHub::new(state.nodes.clone(), state.turn.clone());

        let user_id = Uuid::new_v4().to_string();
        let workspace_id = Uuid::new_v4().to_string();
        let seed = rusqlite::Connection::open(&db_path)?;
        seed.execute("PRAGMA foreign_keys=ON", [])?;
        seed.execute(
            "INSERT INTO users(id,name,created_at) VALUES(?,?,?)",
            rusqlite::params![user_id, "Test User", now_ms()],
        )?;
        seed.execute(
            "INSERT INTO workspaces(id,name,created_by,created_at) VALUES(?,?,?,?)",
            rusqlite::params![workspace_id, "Test Workspace", user_id, now_ms()],
        )?;
        seed.execute(
            "INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,'owner',?)",
            rusqlite::params![workspace_id, user_id, now_ms()],
        )?;

        let device_id = Uuid::new_v4().to_string();
        let node = NodeState::generate(device_id.clone());
        state.db.insert_device(&NewDevice {
            id: device_id.clone(),
            workspace_id: workspace_id.clone(),
            name: "test-node".into(),
            hostname: "test-host".into(),
            platform: "test".into(),
            arch: "test".into(),
            identity_public_key: node.identity_public_key()?,
            transport_public_key: node.transport_public_key()?,
            version: "0.16.0-test".into(),
            capabilities: vec!["process".into(), "webrtc".into()],
        })?;
        let process_id = Uuid::new_v4().to_string();
        seed.execute(
            "INSERT INTO processes(id,device_id,origin,status,terminal,created_by,created_at) VALUES(?,?,'browser','starting',1,?,?)",
            rusqlite::params![process_id, device_id, user_id, now_ms()],
        )?;

        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await?;
        let address = listener.local_addr()?;
        let application = app(state.clone());
        let server = tokio::spawn(async move {
            let _ = axum::serve(listener, application).await;
        });
        Ok(Self {
            root,
            server,
            state,
            seed,
            user_id,
            workspace_id,
            device_id,
            process_id,
            node,
            base: format!("http://{address}"),
        })
    }

    pub(super) async fn connect(&self) -> anyhow::Result<ServerTransport> {
        timeout(
            Duration::from_secs(10),
            ServerTransport::connect(&self.base, &self.node),
        )
        .await?
    }
}

impl Drop for Harness {
    fn drop(&mut self) {
        self.server.abort();
        let _ = std::fs::remove_dir_all(&self.root);
    }
}
