use crate::exports::ohrats::rc_process::runtime::ExitResult;
use crate::ohrats::{
    rc_process::{
        process_host::{self, ByteStream, Child, ExecutionGroup},
        types::{Signal, StartPlan, StreamKind},
    },
    rc_shell::executor,
};
use crate::resolve::spawn_request;

pub(crate) enum Native {
    Process(Process),
    Shell(String),
}

type PollOutput = (Vec<(StreamKind, Vec<u8>)>, Option<ExitResult>);

pub(crate) struct Process {
    group: Option<ExecutionGroup>,
    child: Child,
    stdin: Option<ByteStream>,
    stdout: ByteStream,
    stderr: Option<ByteStream>,
}

impl Native {
    pub(crate) fn start(id: &str, plan: StartPlan) -> Result<Self, String> {
        if let crate::ohrats::rc_process::types::ExecutionMode::RcShell(script) = &plan.mode {
            executor::start(
                id,
                &executor::StartRequest {
                    script: script.clone(),
                    cwd: plan.cwd,
                    environment: plan.environment,
                    terminal: plan.terminal,
                },
            )?;
            return Ok(Self::Shell(id.into()));
        }
        let request = spawn_request(plan)?;
        let group = process_host::create_group(id)?;
        let spawned = group.spawn(&request)?;
        Ok(Self::Process(Process {
            group: Some(group),
            child: spawned.child,
            stdin: spawned.stdin,
            stdout: spawned.stdout,
            stderr: spawned.stderr,
        }))
    }

    pub(crate) fn poll(&mut self, budget: u32) -> Result<PollOutput, String> {
        match self {
            Self::Process(value) => value.poll(budget),
            Self::Shell(id) => {
                let result = executor::poll(id, budget)?;
                Ok((
                    result
                        .output
                        .into_iter()
                        .map(|value| (value.kind, value.bytes))
                        .collect(),
                    result.exit.map(|value| ExitResult {
                        code: value.code,
                        signal: value.signal,
                    }),
                ))
            }
        }
    }

    pub(crate) fn input(&self, bytes: &[u8]) -> Result<u32, String> {
        match self {
            Self::Process(value) => value.input(bytes),
            Self::Shell(id) => executor::input(id, bytes),
        }
    }

    pub(crate) fn close_input(&mut self) -> Result<(), String> {
        match self {
            Self::Process(value) => value.close_input(),
            Self::Shell(id) => executor::close_input(id),
        }
    }

    pub(crate) fn resize(&self, cols: u16, rows: u16) -> Result<(), String> {
        match self {
            Self::Process(value) => value.group()?.resize(cols, rows),
            Self::Shell(id) => executor::resize(id, cols, rows),
        }
    }

    pub(crate) fn signal(&self, signal: Signal) -> Result<(), String> {
        match self {
            Self::Process(value) => value.group()?.signal(signal),
            Self::Shell(id) => executor::request_signal(id, signal),
        }
    }

    pub(crate) fn close(&mut self) {
        match self {
            Self::Process(value) => {
                if let Some(group) = value.group.take() {
                    group.close();
                }
            }
            Self::Shell(id) => executor::close(id),
        }
    }
}

impl Process {
    fn poll(&mut self, budget: u32) -> Result<PollOutput, String> {
        let mut output = Vec::new();
        read(&self.stdout, StreamKind::Stdout, budget, &mut output)?;
        if let Some(stderr) = &self.stderr {
            read(stderr, StreamKind::Stderr, budget, &mut output)?;
        }
        let exit = self.child.poll_exit()?.map(|value| ExitResult {
            code: value.code,
            signal: value.signal,
        });
        Ok((output, exit))
    }

    fn input(&self, bytes: &[u8]) -> Result<u32, String> {
        let stdin = self.stdin.as_ref().ok_or("execution stdin is closed")?;
        match stdin.write(bytes)? {
            process_host::WriteResult::Accepted(value) => Ok(value),
            process_host::WriteResult::WouldBlock => Ok(0),
        }
    }

    fn close_input(&mut self) -> Result<(), String> {
        if let Some(stdin) = self.stdin.take() {
            stdin.close_write()?;
        }
        Ok(())
    }

    fn group(&self) -> Result<&ExecutionGroup, String> {
        self.group
            .as_ref()
            .ok_or_else(|| "execution is closed".into())
    }
}

fn read(
    stream: &ByteStream,
    kind: StreamKind,
    budget: u32,
    output: &mut Vec<(StreamKind, Vec<u8>)>,
) -> Result<(), String> {
    if let process_host::ReadResult::Data(bytes) = stream.read(budget)? {
        output.push((kind, bytes));
    }
    Ok(())
}
