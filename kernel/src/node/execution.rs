use super::{process::start_request, values};
use crate::service::{PinnedProvider, ServiceRegistry};
use rc_node::{
    ExecutionRead, JournalChunk, ProcessChannel, ProcessEnvironment, ProcessExecutionMode,
    ProcessLifetime, ProcessPrincipal, ProcessSignal, ProcessStartRequest, StreamKind,
};
use semver::VersionReq;
use std::time::{Duration, Instant};
use wasmtime::component::{ResourceAny, Val};

mod manager;
mod manager_check;
pub use manager::ComponentExecutionManager;
pub use manager_check::check_manager;

const SERVICE: &str = "ohrats:rc-process/runtime";

#[derive(Clone)]
pub struct ComponentExecutionRuntime {
    registry: ServiceRegistry,
    requirement: VersionReq,
}

pub struct Execution {
    provider: PinnedProvider,
    resource: ResourceAny,
}

impl ComponentExecutionRuntime {
    pub fn new(registry: ServiceRegistry) -> anyhow::Result<Self> {
        Ok(Self {
            registry,
            requirement: VersionReq::parse("^0.3")?,
        })
    }

    pub fn available(&self) -> anyhow::Result<bool> {
        Ok(self
            .registry
            .has_provider(SERVICE, &self.requirement, None)?)
    }

    pub fn start(&self, request: ProcessStartRequest) -> Result<Execution, String> {
        let provider = self
            .registry
            .pinned(SERVICE, &self.requirement)
            .map_err(display)?
            .into_iter()
            .next()
            .ok_or_else(|| "execution runtime is unavailable".to_owned())?;
        let values = provider
            .call(SERVICE, "start", &[start_request(request)])
            .map_err(display)?;
        let resource = match values::result_value(values, "execution start")? {
            Val::Resource(resource) if resource.owned() => resource,
            _ => return Err("execution runtime returned an invalid resource".into()),
        };
        Ok(Execution { provider, resource })
    }
}

impl Execution {
    fn call(&self, function: &str, extra: &[Val]) -> Result<Vec<Val>, String> {
        let mut params = Vec::with_capacity(extra.len() + 1);
        params.push(Val::Resource(self.resource));
        params.extend_from_slice(extra);
        self.provider
            .call(SERVICE, function, &params)
            .map_err(display)
    }

    pub fn state(&self) -> Result<String, String> {
        match self.call("[method]execution.state", &[])?.as_slice() {
            [Val::Enum(value)] => Ok(value.clone()),
            _ => Err("execution runtime returned an invalid state".into()),
        }
    }

    pub fn read(&self, cursor: u64, max_bytes: u32) -> Result<ExecutionRead, String> {
        let values = self.call(
            "[method]execution.read",
            &[Val::U64(cursor), Val::U32(max_bytes)],
        )?;
        let fields = values::record(
            values::result_value(values, "execution read")?,
            "read result",
        )?;
        let chunks = values::list_field(&fields, "chunks")?;
        let mut output = Vec::with_capacity(chunks.len());
        for chunk in chunks {
            let fields = values::record(chunk, "output chunk")?;
            let stream = match values::enum_field(&fields, "kind")?.as_str() {
                "stdout" => StreamKind::Stdout,
                "stderr" => StreamKind::Stderr,
                _ => return Err("execution runtime returned an invalid stream".into()),
            };
            let cursor = u64_field(&fields, "cursor")?;
            let mut bytes = Vec::new();
            for byte in values::list(values::field(&fields, "bytes")?.clone(), "output bytes")? {
                match byte {
                    Val::U8(value) => bytes.push(value),
                    _ => return Err("non-byte output".into()),
                }
            }
            output.push(JournalChunk {
                stream,
                cursor,
                bytes,
            });
        }
        let exit = option_record(&fields, "exit")?;
        Ok(ExecutionRead {
            status: match values::enum_field(&fields, "state")?.as_str() {
                "exited" => "exited",
                "lost" => "lost",
                _ => "running",
            },
            chunks: output,
            next_cursor: u64_field(&fields, "next-cursor")?,
            truncated_before: u64_field(&fields, "truncated-before")?,
            more: bool_field(&fields, "more")?,
            exit_code: exit
                .as_ref()
                .and_then(|value| option_u32(value, "code"))
                .and_then(|value| i32::try_from(value).ok()),
            signal: exit
                .as_ref()
                .and_then(|value| option_enum(value, "signal"))
                .unwrap_or_default(),
        })
    }

    pub fn input(&self, bytes: &[u8]) -> Result<u32, String> {
        let values = self.call(
            "[method]execution.input",
            &[Val::List(bytes.iter().copied().map(Val::U8).collect())],
        )?;
        match values::result_value(values, "execution input")? {
            Val::U32(value) => Ok(value),
            _ => Err("execution runtime returned an invalid input result".into()),
        }
    }

    pub fn close_input(&self) -> Result<(), String> {
        values::unit_result(
            self.call("[method]execution.close-input", &[])?,
            "execution close-input",
        )
    }

    pub fn attach(&self, controller: &str) -> Result<(), String> {
        values::unit_result(
            self.call(
                "[method]execution.attach",
                &[Val::String(controller.into())],
            )?,
            "execution attach",
        )
    }

    pub fn detach(&self, controller: &str) -> Result<(), String> {
        values::unit_result(
            self.call(
                "[method]execution.detach",
                &[Val::String(controller.into())],
            )?,
            "execution detach",
        )
    }

    pub fn resize(&self, cols: u16, rows: u16) -> Result<(), String> {
        let size = Val::Record(vec![
            ("cols".into(), Val::U16(cols)),
            ("rows".into(), Val::U16(rows)),
        ]);
        values::unit_result(
            self.call("[method]execution.resize", &[size])?,
            "execution resize",
        )
    }

    pub fn signal(&self, signal: ProcessSignal) -> Result<(), String> {
        let name = match signal {
            ProcessSignal::Interrupt => "interrupt",
            ProcessSignal::Terminate => "terminate",
            ProcessSignal::Kill => "kill",
        };
        values::unit_result(
            self.call("[method]execution.signal", &[Val::Enum(name.into())])?,
            "execution signal",
        )
    }
}

impl Drop for Execution {
    fn drop(&mut self) {
        if let Err(error) = self.provider.drop_resource(self.resource) {
            eprintln!("execution resource drop failed: {error:#}");
        }
    }
}

pub fn check_exact_argv(runtime: &ComponentExecutionRuntime) -> anyhow::Result<()> {
    let executable = std::env::current_exe()?.to_string_lossy().into_owned();
    let execution = runtime
        .start(ProcessStartRequest {
            execution_id: format!("runtime-check-{}", std::process::id()),
            mode: ProcessExecutionMode::Argv {
                program: executable,
                args: vec!["--version".into()],
            },
            cwd: None,
            environment: ProcessEnvironment::default(),
            terminal: None,
            channel: ProcessChannel::Control,
            lifetime: ProcessLifetime::Managed,
            principal: ProcessPrincipal {
                user_id: "runtime-check".into(),
                role: "owner".into(),
                can_execute: true,
                can_manage_devices: true,
            },
            max_runtime_ms: Some(5_000),
        })
        .map_err(anyhow::Error::msg)?;
    let deadline = Instant::now() + Duration::from_secs(5);
    let mut output = Vec::new();
    while Instant::now() < deadline {
        let read = execution
            .read(output.len() as u64, 4096)
            .map_err(anyhow::Error::msg)?;
        for chunk in read.chunks {
            output.extend(chunk.bytes);
        }
        if execution.state().map_err(anyhow::Error::msg)? == "exited" {
            break;
        }
        std::thread::sleep(Duration::from_millis(10));
    }
    anyhow::ensure!(
        String::from_utf8_lossy(&output).starts_with("RC kernel "),
        "execution runtime exact-argv probe failed"
    );
    Ok(())
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}

fn u64_field(fields: &[(String, Val)], name: &str) -> Result<u64, String> {
    match values::field(fields, name)? {
        Val::U64(value) => Ok(*value),
        _ => Err(format!("field {name:?} is not u64")),
    }
}

fn bool_field(fields: &[(String, Val)], name: &str) -> Result<bool, String> {
    match values::field(fields, name)? {
        Val::Bool(value) => Ok(*value),
        _ => Err(format!("field {name:?} is not bool")),
    }
}

fn option_record(
    fields: &[(String, Val)],
    name: &str,
) -> Result<Option<Vec<(String, Val)>>, String> {
    values::option_record_field(fields, name)
}

fn option_u32(fields: &[(String, Val)], name: &str) -> Option<u32> {
    match values::field(fields, name).ok()? {
        Val::Option(Some(value)) => match value.as_ref() {
            Val::U32(value) => Some(*value),
            _ => None,
        },
        _ => None,
    }
}

fn option_enum(fields: &[(String, Val)], name: &str) -> Option<String> {
    match values::field(fields, name).ok()? {
        Val::Option(Some(value)) => match value.as_ref() {
            Val::Enum(value) => Some(value.to_ascii_uppercase()),
            _ => None,
        },
        _ => None,
    }
}
