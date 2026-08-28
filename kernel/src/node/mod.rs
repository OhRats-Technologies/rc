mod process;
mod transport;
mod values;

use self::{process::ComponentProcessPolicy, transport::ComponentTransportPolicy};
use crate::{runtime::Runtime, watch};
use anyhow::Context as _;
use rc_node::{
    DEFAULT_SERVER, NodeRuntime, ProcessChannel, ProcessPolicy, ProcessPrincipal,
    ProcessStartRequest, TransportAnswerRequest, TransportPolicy, acquire_run_lock, load_config,
    load_state, resolve_state_dir,
};
use rc_protocol::{ControlIceMode, IceServer};
use std::{path::PathBuf, process::Command, sync::Arc, thread, time::Duration};

pub struct Options {
    pub state_dir: Option<PathBuf>,
    pub server: Option<String>,
    pub runner: Option<PathBuf>,
    pub agent_version: Option<String>,
}

pub fn run(mut runtime: Runtime, options: Options) -> anyhow::Result<()> {
    let registry = runtime.service_registry();
    let process_policy = ComponentProcessPolicy::new(registry.clone())?;
    let transport_policy = ComponentTransportPolicy::new(registry)?;
    anyhow::ensure!(
        process_policy.available()?,
        "required process-policy component is unavailable"
    );
    anyhow::ensure!(
        transport_policy.available("webrtc")?,
        "required WebRTC transport component is unavailable"
    );
    let state_dir = options.state_dir.unwrap_or_else(|| resolve_state_dir(None));
    let config = load_config(&state_dir).unwrap_or_default();
    let server = options
        .server
        .or_else(|| nonempty_env("RC_URL"))
        .or_else(|| (!config.server.is_empty()).then_some(config.server))
        .unwrap_or_else(|| DEFAULT_SERVER.into());
    let state = load_state(&state_dir).context("not enrolled; run rc enroll TOKEN")?;
    let _run_lock = acquire_run_lock(&state_dir)?;
    let runner = options.runner.unwrap_or_else(default_runner);
    anyhow::ensure!(runner.is_file(), "RC process runner is unavailable");
    let version = options
        .agent_version
        .or_else(|| nonempty_env("RC_AGENT_VERSION"))
        .or_else(|| runner_version(&runner))
        .unwrap_or_else(|| env!("CARGO_PKG_VERSION").into());
    thread::spawn(move || {
        if let Err(error) = watch::run(&mut runtime) {
            eprintln!("component watcher stopped: {error:#}");
        }
    });
    let tokio = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()?;
    tokio.block_on(run_node(
        runner,
        state_dir,
        server,
        state,
        version,
        Arc::new(process_policy),
        Arc::new(transport_policy),
    ))
}

pub fn check(runtime: &Runtime) -> anyhow::Result<()> {
    let registry = runtime.service_registry();
    let process = ComponentProcessPolicy::new(registry.clone())?;
    let transport = ComponentTransportPolicy::new(registry)?;
    anyhow::ensure!(process.available()?, "process policy is unavailable");
    anyhow::ensure!(
        transport.available("webrtc")?,
        "WebRTC policy is unavailable"
    );
    let start = process
        .authorize_start(ProcessStartRequest {
            process_id: "policy-check".into(),
            command: " printf ok ".into(),
            cwd: String::new(),
            terminal: None,
            channel: ProcessChannel::Control,
            principal: ProcessPrincipal {
                user_id: "test".into(),
                role: "operator".into(),
                can_execute: true,
                can_manage_devices: false,
            },
        })
        .map_err(anyhow::Error::msg)?;
    anyhow::ensure!(
        start.command == "printf ok",
        "process policy returned wrong plan"
    );
    let plan = transport
        .answer_plan(
            "webrtc",
            TransportAnswerRequest {
                mode: ControlIceMode::Stun,
                ice_servers: vec![IceServer {
                    urls: vec!["stun:example.test".into(), "turn:example.test".into()],
                    username: String::new(),
                    credential: String::new(),
                }],
            },
        )
        .map_err(anyhow::Error::msg)?;
    anyhow::ensure!(
        plan.ice_servers.len() == 1 && plan.ice_servers[0].urls == ["stun:example.test"],
        "WebRTC policy returned wrong ICE plan"
    );
    println!("node component policies: ok");
    Ok(())
}

#[allow(clippy::too_many_arguments)]
async fn run_node(
    runner: PathBuf,
    state_dir: PathBuf,
    server: String,
    state: rc_node::NodeState,
    version: String,
    process_policy: Arc<dyn rc_node::ProcessPolicy>,
    transport_policy: Arc<dyn rc_node::TransportPolicy>,
) -> anyhow::Result<()> {
    let mut runtime =
        NodeRuntime::new_with_policies(runner, state_dir, process_policy, transport_policy);
    println!("Connecting to {server} as {}", state.device_id);
    let shutdown = shutdown_signal();
    tokio::pin!(shutdown);
    loop {
        tokio::select! {
            _ = &mut shutdown => break,
            result = runtime.connect_once(&server, &state, &version) => {
                if let Err(error) = result {
                    eprintln!("connection ended: {error}");
                }
            }
        }
        tokio::select! {
            _ = &mut shutdown => break,
            _ = tokio::time::sleep(Duration::from_secs(3)) => {}
        }
    }
    runtime.shutdown();
    Ok(())
}

async fn shutdown_signal() {
    #[cfg(unix)]
    {
        let mut terminate =
            tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate()).ok();
        tokio::select! {
            _ = tokio::signal::ctrl_c() => {}
            _ = async {
                if let Some(signal) = terminate.as_mut() {
                    signal.recv().await;
                } else {
                    std::future::pending::<()>().await;
                }
            } => {}
        }
    }
    #[cfg(not(unix))]
    {
        let _ = tokio::signal::ctrl_c().await;
    }
}

fn default_runner() -> PathBuf {
    std::env::current_exe()
        .ok()
        .and_then(|path| path.parent().map(|parent| parent.join("rc")))
        .unwrap_or_else(|| PathBuf::from("rc"))
}

fn runner_version(runner: &PathBuf) -> Option<String> {
    let output = Command::new(runner).arg("version").output().ok()?;
    if !output.status.success() {
        return None;
    }
    String::from_utf8(output.stdout)
        .ok()?
        .split_whitespace()
        .last()
        .map(str::to_owned)
}

fn nonempty_env(name: &str) -> Option<String> {
    std::env::var(name)
        .ok()
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
}
