use crate::{
    Config, ControlHub, Database, EventHub, ExecutionPolicy, McpHub, NodeHub, SshHub, TurnProvider,
    middleware,
};
use std::{net::IpAddr, sync::Arc};
use webauthn_rs::prelude::{Url, Webauthn, WebauthnBuilder};

#[derive(Clone)]
pub struct AppState {
    pub config: Arc<Config>,
    pub db: Database,
    pub turn: TurnProvider,
    pub nodes: NodeHub,
    pub control: ControlHub,
    pub webauthn: Arc<Webauthn>,
    pub events: EventHub,
    pub execution: ExecutionPolicy,
    pub ssh: SshHub,
    pub mcp: McpHub,
    pub services: rc_context::Context,
    pub mesh: rc_mesh::RouteBroker,
    pub rate_limits: middleware::RateLimiter,
    _service_leases: Arc<Vec<rc_context::ServiceLease>>,
}

impl AppState {
    pub fn new(config: Config) -> anyhow::Result<Self> {
        let webauthn = build_webauthn(&config.public_url)?;
        let db = Database::open(&config.db_path)?;
        let execution =
            ExecutionPolicy::new(config.execution_history, config.execution_history_ttl_hours);
        execution.cleanup_startup(&db)?;
        EventHub::cleanup_transient(&db)?;
        let turn = TurnProvider::new(config.turn_token_id.clone(), config.turn_api_token.clone());
        let nodes = NodeHub::default();
        let control = ControlHub::new(nodes.clone(), turn.clone());
        let events = EventHub::new(execution.clone());
        let ssh = SshHub::default();
        let mcp = McpHub::default();
        let services = rc_context::Context::root("rc-server");
        let mesh = rc_mesh::RouteBroker::default();
        let coordinator = rc_mesh::CoordinatorPolicy::tier0();
        let service_leases = Arc::new(vec![
            services.provide(Arc::new(db.clone()))?,
            services.provide(Arc::new(nodes.clone()))?,
            services.provide(Arc::new(execution.clone()))?,
            services.provide(Arc::new(mesh.clone()))?,
            services.provide(Arc::new(coordinator))?,
        ]);
        Ok(Self {
            config: Arc::new(config),
            db,
            turn,
            nodes,
            control,
            webauthn: Arc::new(webauthn),
            events,
            execution,
            ssh,
            mcp,
            services,
            mesh,
            rate_limits: middleware::RateLimiter::default(),
            _service_leases: service_leases,
        })
    }

    pub fn workspace_context(&self, workspace_id: &str) -> rc_context::Context {
        self.services.child(format!("workspace:{workspace_id}"))
    }

    pub fn release_device_sessions(&self, device_id: &str) {
        self.control.release_device(device_id);
        let mut processes = self.ssh.release_device(device_id);
        processes.extend(self.mcp.release_device(device_id));
        for process_id in processes {
            self.lose_hosted_process(device_id, &process_id, "RC Node disconnected");
        }
    }

    pub fn complete_hosted_process(
        &self,
        device_id: &str,
        process_id: &str,
        exit_code: i32,
        signal: &str,
    ) {
        let lifecycle = match self
            .db
            .mark_process_exit(device_id, process_id, exit_code, signal)
        {
            Ok(value) => value,
            Err(error) => {
                tracing::warn!(%device_id, %process_id, %error, "failed to complete hosted process");
                return;
            }
        };
        let Some(lifecycle) = lifecycle else { return };
        self.emit_process(
            "process.exited",
            device_id,
            process_id,
            &lifecycle,
            serde_json::json!({"exitCode":exit_code,"signal":signal}),
        );
    }

    pub fn lose_hosted_process(&self, device_id: &str, process_id: &str, reason: &str) {
        let lifecycle = match self.db.mark_process_lost(device_id, process_id, reason) {
            Ok(value) => value,
            Err(error) => {
                tracing::warn!(%device_id, %process_id, %error, "failed to mark hosted process lost");
                return;
            }
        };
        let Some(lifecycle) = lifecycle else { return };
        self.emit_process(
            "process.lost",
            device_id,
            process_id,
            &lifecycle,
            serde_json::json!({"error":reason}),
        );
    }

    pub async fn disconnect_device(&self, device_id: &str) {
        self.nodes.remove_if(device_id, "").await;
        self.release_device_sessions(device_id);
    }

    pub fn emit_device_presence(&self, device_id: &str, online: bool) {
        let workspace = match self.db.device_workspace(device_id) {
            Ok(Some(value)) => value,
            Ok(None) => return,
            Err(error) => {
                tracing::warn!(%device_id, %error, "failed to resolve device workspace");
                return;
            }
        };
        let kind = if online {
            "device.online"
        } else {
            "device.offline"
        };
        if let Err(error) = self.events.emit(
            &self.db,
            kind,
            Some(&workspace),
            None,
            Some(device_id),
            serde_json::json!({}),
        ) {
            tracing::warn!(%device_id, %error, "failed to emit device presence");
        }
    }

    fn emit_process(
        &self,
        kind: &str,
        device_id: &str,
        process_id: &str,
        lifecycle: &crate::ProcessLifecycle,
        mut detail: serde_json::Value,
    ) {
        detail["processId"] = process_id.into();
        if let Err(error) = self.events.emit(
            &self.db,
            kind,
            Some(&lifecycle.workspace_id),
            Some(&lifecycle.user_id),
            Some(device_id),
            detail,
        ) {
            tracing::warn!(%device_id, %process_id, %error, "failed to emit hosted process event");
        }
        if let Err(error) = self.execution.finalize(&self.db, process_id) {
            tracing::warn!(%process_id, %error, "failed to finalize hosted process");
        }
    }
}

pub(crate) fn build_webauthn(public_url: &str) -> anyhow::Result<Webauthn> {
    let public_url = public_url.trim_end_matches('/');
    let origin = Url::parse(public_url)
        .map_err(|error| anyhow::anyhow!("PUBLIC_URL must be an absolute URL: {error}"))?;
    if !matches!(origin.scheme(), "http" | "https") {
        anyhow::bail!("PUBLIC_URL must use http or https");
    }
    let rp_id = origin
        .host_str()
        .ok_or_else(|| anyhow::anyhow!("PUBLIC_URL must contain a host"))?;
    if rp_id.parse::<IpAddr>().is_ok() {
        anyhow::bail!(
            "PUBLIC_URL must use a DNS hostname for passkeys; use http://localhost for local development instead of an IP address"
        );
    }
    WebauthnBuilder::new(rp_id, &origin)
        .map_err(|error| {
            anyhow::anyhow!(
                "PUBLIC_URL is not a valid passkey origin; use HTTPS with a DNS hostname, or http://localhost for local development: {error}"
            )
        })?
        .rp_name("RC")
        .build()
        .map_err(|error| anyhow::anyhow!("failed to configure passkeys: {error}"))
}
