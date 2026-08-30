mod crypto;
mod execution;
mod process;
mod scheduler;
mod transport;
mod values;

use self::{process::ComponentProcessPolicy, transport::ComponentTransportPolicy};
use crate::{runtime::Runtime, watch};
use anyhow::Context as _;
use rc_node::{
    DEFAULT_SERVER, NodeRuntime, ProcessChannel, ProcessEnvironment, ProcessExecutionMode,
    ProcessLifetime, ProcessPolicy, ProcessPrincipal, ProcessStartRequest, ScheduleManager,
    TransportAnswerRequest, TransportPolicy, acquire_run_lock, load_config, load_state,
    resolve_state_dir,
};
use rc_protocol::{ControlIceMode, IceServer};
use std::{path::PathBuf, sync::Arc, thread, time::Duration};

pub struct Options {
    pub state_dir: Option<PathBuf>,
    pub server: Option<String>,
    pub agent_version: Option<String>,
}

pub fn run(mut runtime: Runtime, options: Options) -> anyhow::Result<()> {
    let registry = runtime.service_registry();
    let process_policy = ComponentProcessPolicy::new(registry.clone())?;
    let execution_runtime = execution::ComponentExecutionRuntime::new(registry.clone())?;
    let scheduler = scheduler::ComponentScheduler::new(registry.clone())?;
    let transport_policy = ComponentTransportPolicy::new(registry)?;
    anyhow::ensure!(
        process_policy.available()?,
        "required process-policy component is unavailable"
    );
    anyhow::ensure!(
        execution_runtime.available()?,
        "required execution-runtime component is unavailable"
    );
    anyhow::ensure!(
        scheduler.available()?,
        "required scheduler component is unavailable"
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
    let version = options
        .agent_version
        .or_else(|| nonempty_env("RC_AGENT_VERSION"))
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
        state_dir,
        server,
        state,
        version,
        Arc::new(process_policy),
        Arc::new(transport_policy),
        execution_runtime,
        scheduler,
    ))
}

pub fn check(runtime: &Runtime) -> anyhow::Result<()> {
    let registry = runtime.service_registry();
    let process = ComponentProcessPolicy::new(registry.clone())?;
    let transport = ComponentTransportPolicy::new(registry)?;
    let execution = execution::ComponentExecutionRuntime::new(runtime.service_registry())?;
    let scheduler = scheduler::ComponentScheduler::new(runtime.service_registry())?;
    anyhow::ensure!(process.available()?, "process policy is unavailable");
    anyhow::ensure!(
        transport.available("webrtc")?,
        "WebRTC policy is unavailable"
    );
    anyhow::ensure!(execution.available()?, "execution runtime is unavailable");
    anyhow::ensure!(scheduler.available()?, "scheduler is unavailable");
    scheduler.list().map_err(anyhow::Error::msg)?;
    let start = process
        .authorize_start(ProcessStartRequest {
            execution_id: "policy-check".into(),
            mode: ProcessExecutionMode::Argv {
                program: "printf".into(),
                args: vec!["ok".into()],
            },
            cwd: None,
            environment: ProcessEnvironment::default(),
            terminal: None,
            channel: ProcessChannel::Control,
            lifetime: ProcessLifetime::Attached,
            principal: ProcessPrincipal {
                user_id: "test".into(),
                role: "operator".into(),
                can_execute: true,
                can_manage_devices: false,
            },
            max_runtime_ms: None,
        })
        .map_err(anyhow::Error::msg)?;
    anyhow::ensure!(
        matches!(start.mode, ProcessExecutionMode::Argv { .. }),
        "process policy returned wrong plan"
    );
    execution::check_exact_argv(&execution).context("execution runtime exact-argv check")?;
    execution::check_manager(execution.clone()).context("execution manager policy check")?;
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

pub fn crypto_check(runtime: &Runtime) -> anyhow::Result<()> {
    crypto::check(runtime)
}

pub fn crypto_probe(runtime: Runtime) -> anyhow::Result<()> {
    crypto::probe(runtime)
}

pub fn probe(mut runtime: Runtime) -> anyhow::Result<()> {
    use std::io::BufRead as _;
    use std::io::Write as _;

    let registry = runtime.service_registry();
    let process = ComponentProcessPolicy::new(registry.clone())?;
    let transport = ComponentTransportPolicy::new(registry)?;
    thread::spawn(move || {
        loop {
            if let Err(error) = runtime.reconcile() {
                eprintln!("component probe reconcile failed: {error:#}");
            }
            thread::sleep(Duration::from_millis(50));
        }
    });
    for line in std::io::stdin().lock().lines() {
        if line? == "process" {
            let plan = process
                .authorize_start(ProcessStartRequest {
                    execution_id: "probe".into(),
                    mode: ProcessExecutionMode::Argv {
                        program: "true".into(),
                        args: Vec::new(),
                    },
                    cwd: None,
                    environment: ProcessEnvironment::default(),
                    terminal: None,
                    channel: ProcessChannel::Control,
                    lifetime: ProcessLifetime::Attached,
                    principal: ProcessPrincipal {
                        user_id: "probe".into(),
                        role: "owner".into(),
                        can_execute: true,
                        can_manage_devices: true,
                    },
                    max_runtime_ms: None,
                })
                .map_err(anyhow::Error::msg)?;
            println!("Process {}", plan.stdin_chunk_bytes);
            std::io::stdout().flush()?;
            continue;
        }
        let attempts = transport
            .attempts(vec![IceServer {
                urls: vec!["stun:example.test".into(), "turn:example.test".into()],
                username: String::new(),
                credential: String::new(),
            }])
            .map_err(anyhow::Error::msg)?;
        let first = attempts
            .first()
            .ok_or_else(|| anyhow::anyhow!("empty transport attempt plan"))?;
        println!("{:?} {}", first.mode, first.connect_timeout_ms);
        std::io::stdout().flush()?;
    }
    Ok(())
}

#[allow(clippy::too_many_arguments)]
async fn run_node(
    state_dir: PathBuf,
    server: String,
    state: rc_node::NodeState,
    version: String,
    process_policy: Arc<dyn rc_node::ProcessPolicy>,
    transport_policy: Arc<dyn rc_node::TransportPolicy>,
    execution_runtime: execution::ComponentExecutionRuntime,
    scheduler: scheduler::ComponentScheduler,
) -> anyhow::Result<()> {
    let scheduler_state_dir = state_dir.clone();
    let mut runtime = NodeRuntime::with_execution_manager(
        state_dir,
        process_policy,
        transport_policy,
        move |sink| {
            Arc::new(execution::ComponentExecutionManager::new(
                execution_runtime,
                sink,
            ))
        },
    );
    let scheduler_manager = runtime.manager_arc();
    runtime.set_schedule_manager(Arc::new(scheduler.clone()));
    let scheduler_device = state.device_id.clone();
    let scheduler_task = tokio::spawn(scheduler::run(
        scheduler,
        scheduler_manager,
        scheduler_state_dir,
        scheduler_device,
    ));
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
    scheduler_task.abort();
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

fn nonempty_env(name: &str) -> Option<String> {
    std::env::var(name)
        .ok()
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
}
