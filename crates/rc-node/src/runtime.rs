use crate::{
    ControlManager, NODE_CAPABILITIES, NativeProcessPolicy, NativeTransportPolicy, NodeState,
    ProcessEvent, ProcessManager, ProcessPolicy, ServerTransport, TransportPolicy, lock_metadata,
};
use rc_protocol::{NodeHello, NodeToServer};
use std::{collections::VecDeque, io, path::PathBuf, sync::Arc};
use tokio::sync::mpsc;

const LIFECYCLE_QUEUE_LIMIT: usize = 4096;

pub struct NodeRuntime {
    manager: Arc<ProcessManager>,
    state_dir: PathBuf,
    outbound: mpsc::UnboundedSender<NodeToServer>,
    lifecycle: mpsc::UnboundedReceiver<NodeToServer>,
    pending: VecDeque<NodeToServer>,
    services: rc_context::Context,
    mesh: rc_mesh::RouteBroker,
    _service_leases: Vec<rc_context::ServiceLease>,
    process_policy: Arc<dyn ProcessPolicy>,
    transport_policy: Arc<dyn TransportPolicy>,
}

impl NodeRuntime {
    pub fn new(runner: PathBuf, state_dir: PathBuf) -> Self {
        Self::new_with_policies(
            runner,
            state_dir,
            Arc::new(NativeProcessPolicy),
            Arc::new(NativeTransportPolicy),
        )
    }

    pub fn new_with_policies(
        runner: PathBuf,
        state_dir: PathBuf,
        process_policy: Arc<dyn ProcessPolicy>,
        transport_policy: Arc<dyn TransportPolicy>,
    ) -> Self {
        let (tx, lifecycle) = mpsc::unbounded_channel();
        let event_tx = tx.clone();
        let manager = Arc::new(ProcessManager::new(runner, move |event| {
            let message = match event {
                ProcessEvent::Started { id } => NodeToServer::ProcessStarted { id },
                ProcessEvent::Exit {
                    id,
                    exit_code,
                    signal,
                } => NodeToServer::ProcessExit {
                    id,
                    exit_code,
                    signal,
                },
                ProcessEvent::Stdout { .. } | ProcessEvent::Stderr { .. } => return,
            };
            let _ = event_tx.send(message);
        }));
        let relay_tx = tx.clone();
        manager.set_relay_sink(move |relay_id, event| {
            let message = if let Some(session_id) = relay_id.strip_prefix("ssh:") {
                match event {
                    ProcessEvent::Started { .. } => return true,
                    ProcessEvent::Stdout { data, .. } => NodeToServer::SshStdout {
                        session_id: session_id.to_owned(),
                        data,
                    },
                    ProcessEvent::Stderr { data, .. } => NodeToServer::SshStderr {
                        session_id: session_id.to_owned(),
                        data,
                    },
                    ProcessEvent::Exit {
                        exit_code, signal, ..
                    } => NodeToServer::SshExit {
                        session_id: session_id.to_owned(),
                        exit_code,
                        signal,
                    },
                }
            } else if let Some(process_id) = relay_id.strip_prefix("mcp:") {
                match event {
                    ProcessEvent::Started { .. } => return true,
                    ProcessEvent::Stdout { data, .. } => NodeToServer::McpStdout {
                        process_id: process_id.to_owned(),
                        data,
                    },
                    ProcessEvent::Stderr { data, .. } => NodeToServer::McpStderr {
                        process_id: process_id.to_owned(),
                        data,
                    },
                    ProcessEvent::Exit {
                        exit_code, signal, ..
                    } => NodeToServer::McpExit {
                        process_id: process_id.to_owned(),
                        exit_code,
                        signal,
                    },
                }
            } else {
                return false;
            };
            relay_tx.send(message).is_ok()
        });
        let services = rc_context::Context::root("rc-node");
        let mesh = rc_mesh::RouteBroker::default();
        let mesh_policy = rc_mesh::MeshPolicy::default();
        let service_leases = vec![
            services
                .provide(manager.clone())
                .expect("fresh Node context accepts process manager"),
            services
                .provide(Arc::new(mesh.clone()))
                .expect("fresh Node context accepts route broker"),
            services
                .provide(Arc::new(mesh_policy))
                .expect("fresh Node context accepts mesh policy"),
        ];
        Self {
            manager,
            state_dir,
            outbound: tx,
            lifecycle,
            pending: VecDeque::new(),
            services,
            mesh,
            _service_leases: service_leases,
            process_policy,
            transport_policy,
        }
    }

    pub fn manager(&self) -> &ProcessManager {
        &self.manager
    }

    pub fn context(&self) -> &rc_context::Context {
        &self.services
    }

    pub fn mesh(&self) -> &rc_mesh::RouteBroker {
        &self.mesh
    }

    pub fn shutdown(&self) {
        self.manager.clear_relay_sink();
        self.manager.shutdown();
    }

    pub async fn connect_once(
        &mut self,
        server: &str,
        state: &NodeState,
        version: &str,
    ) -> anyhow::Result<()> {
        let transport = ServerTransport::connect(server, state).await?;
        let control = ControlManager::new_with_policies(
            state.clone(),
            self.state_dir.clone(),
            self.manager.clone(),
            self.outbound.clone(),
            version,
            self.process_policy.clone(),
            self.transport_policy.clone(),
        );
        let secure_control = control.clone();
        self.manager.set_secure_sink(move |session_id, event| {
            secure_control.send_process_event(session_id, event)
        });
        let mut effects = rc_context::EffectScope::new();
        let manager = self.manager.clone();
        effects.defer(move || manager.clear_secure_sink());
        let cleanup_control = control.clone();
        effects.defer_async(move || async move { cleanup_control.shutdown().await });
        let result = self
            .run_connected(server, state, version, transport, &control)
            .await;
        effects.revert().await;
        result
    }

    async fn run_connected(
        &mut self,
        server: &str,
        state: &NodeState,
        version: &str,
        mut transport: ServerTransport,
        control: &ControlManager,
    ) -> anyhow::Result<()> {
        let mut closed = transport.closed();
        transport
            .send(&NodeToServer::Hello {
                hello: node_hello(state, version, &self.state_dir)?,
            })
            .await?;

        let active = self.manager.active_ids();
        transport
            .send(&NodeToServer::ProcessSync {
                ids: active.clone(),
            })
            .await?;
        for id in active {
            transport.send(&NodeToServer::ProcessStarted { id }).await?;
        }

        while let Ok(message) = self.lifecycle.try_recv() {
            self.queue(message);
        }
        self.flush(&transport).await?;

        loop {
            tokio::select! {
                message = self.lifecycle.recv() => {
                    let Some(message) = message else {
                        return Ok(());
                    };
                    self.queue(message);
                    self.flush(&transport).await?;
                }
                message = transport.recv() => {
                    let Some(message) = message else {
                        return Ok(());
                    };
                    control.handle(server, message).await;
                    while let Ok(message) = self.lifecycle.try_recv() {
                        self.queue(message);
                    }
                    self.flush(&transport).await?;
                }
                changed = closed.changed() => {
                    if changed.is_err() || *closed.borrow() {
                        return Ok(());
                    }
                }
            }
        }
    }

    fn queue(&mut self, message: NodeToServer) {
        if self.pending.len() == LIFECYCLE_QUEUE_LIMIT {
            self.pending.pop_front();
        }
        self.pending.push_back(message);
    }

    async fn flush(&mut self, transport: &ServerTransport) -> anyhow::Result<()> {
        while let Some(message) = self.pending.front() {
            transport.send(message).await?;
            self.pending.pop_front();
        }
        Ok(())
    }
}

pub fn node_hello(
    state: &NodeState,
    version: &str,
    state_dir: &std::path::Path,
) -> io::Result<NodeHello> {
    let (lock_hash, lock_generation) = lock_metadata(state_dir);
    Ok(NodeHello {
        version: version.to_owned(),
        hostname: hostname(),
        platform: platform().into(),
        arch: arch().into(),
        capabilities: NODE_CAPABILITIES
            .iter()
            .map(|value| (*value).into())
            .collect(),
        transport_public_key: state.transport_public_key()?,
        lock_hash,
        lock_generation,
    })
}

fn hostname() -> String {
    std::process::Command::new("hostname")
        .output()
        .ok()
        .and_then(|out| String::from_utf8(out.stdout).ok())
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
        .unwrap_or_else(|| "localhost".into())
}

fn platform() -> &'static str {
    match std::env::consts::OS {
        "macos" => "darwin",
        value => value,
    }
}

fn arch() -> &'static str {
    match std::env::consts::ARCH {
        "aarch64" => "arm64",
        "x86_64" => "amd64",
        value => value,
    }
}
