mod builder;
mod builtins;
mod expansion;
mod pipeline;
mod sequence;
mod substitution;

use super::Shell;
use super::exports::ohrats::rc_shell::executor::{
    ExitResult, Guest as ExecutorGuest, Output, PollResult, StartRequest, State,
};
use super::ohrats::rc_process::{
    process_host::{ByteStream, Child, ExecutionGroup},
    types::{Signal, StreamKind},
};
use std::{cell::RefCell, collections::HashMap};

thread_local! {
    static JOBS: RefCell<HashMap<String, ShellJob>> = RefCell::new(HashMap::new());
}

struct ShellJob {
    group: Option<ExecutionGroup>,
    state: JobState,
}

enum JobState {
    Native(NativeJob),
    Virtual(VirtualJob),
    Sequence(sequence::SequenceJob),
    Preparing(substitution::PreparationJob),
    Cancelled(Signal),
}

struct NativeJob {
    stages: Vec<Stage>,
    links: Vec<Link>,
    input: Option<(Vec<u8>, usize)>,
    redirected_input: bool,
}

struct Target {
    path: String,
    append: bool,
    written: bool,
}

struct Stage {
    child: Option<Child>,
    stdin: Option<ByteStream>,
    stdout: Option<ByteStream>,
    stderr: Option<ByteStream>,
    stdout_target: Option<Target>,
    stderr_target: Option<Target>,
    virtual_io: Option<VirtualIo>,
    exit: Option<ExitResult>,
    stdout_eof: bool,
    stderr_eof: bool,
}

struct VirtualIo {
    pending: Vec<u8>,
    producer: Option<Vec<u8>>,
    offset: usize,
    input_closed: bool,
    passthrough: bool,
    code: u32,
}

#[derive(Default)]
struct Link {
    pending: Vec<u8>,
    offset: usize,
    upstream_eof: bool,
}

struct VirtualJob {
    output: Option<Vec<u8>>,
    code: u32,
    shell_exit: bool,
    stdout_target: Option<Target>,
}

impl ExecutorGuest for Shell {
    fn start(id: String, request: StartRequest) -> Result<(), String> {
        if id.is_empty() {
            return Err("shell job id is empty".into());
        }
        let job = builder::build_job(&id, request)?;
        JOBS.with(|jobs| {
            let mut jobs = jobs.borrow_mut();
            if jobs.contains_key(&id) {
                return Err("shell job already exists".into());
            }
            jobs.insert(id, job);
            Ok(())
        })
    }

    fn poll(id: String, max_bytes: u32) -> Result<PollResult, String> {
        with_job(&id, |job| {
            let group = job.group.as_ref().ok_or("shell job is closed")?;
            poll_state(&mut job.state, group, max_bytes)
        })
    }

    fn input(id: String, bytes: Vec<u8>) -> Result<u32, String> {
        with_job(&id, |job| {
            let JobState::Native(job) = sequence::active_mut(&mut job.state) else {
                return Err("builtin stdin is closed".into());
            };
            pipeline::input(job, &bytes)
        })
    }

    fn close_input(id: String) -> Result<(), String> {
        with_job(&id, |job| {
            if let JobState::Native(job) = sequence::active_mut(&mut job.state) {
                pipeline::close_input(job)?;
            }
            Ok(())
        })
    }

    fn resize(id: String, cols: u16, rows: u16) -> Result<(), String> {
        with_group(&id, |group| group.resize(cols, rows))
    }

    fn request_signal(id: String, signal: Signal) -> Result<(), String> {
        with_job(&id, |job| {
            let group = job.group.as_ref().ok_or("shell job is closed")?;
            group.signal(signal)?;
            if matches!(job.state, JobState::Preparing(_)) || matches!(signal, Signal::Kill) {
                job.state = JobState::Cancelled(signal);
            }
            Ok(())
        })
    }

    fn close(id: String) {
        JOBS.with(|jobs| {
            if let Some(mut job) = jobs.borrow_mut().remove(&id)
                && let Some(group) = job.group.take()
            {
                group.close();
            }
        });
    }
}

fn poll_state(
    state: &mut JobState,
    group: &ExecutionGroup,
    max: u32,
) -> Result<PollResult, String> {
    loop {
        let ready = match state {
            JobState::Native(job) => pipeline::poll(job, max),
            JobState::Virtual(job) => Ok(PollResult {
                state: State::Exited,
                output: builtins::output(job)?,
                exit: Some(ExitResult {
                    code: Some(job.code),
                    signal: None,
                }),
            }),
            JobState::Sequence(job) => sequence::poll(job, group, max),
            JobState::Preparing(job) => match substitution::poll(job, group, max)? {
                substitution::Outcome::Polling(result) => return Ok(result),
                substitution::Outcome::Ready(ready) => {
                    *state = ready;
                    continue;
                }
            },
            JobState::Cancelled(signal) => Ok(PollResult {
                state: State::Exited,
                output: Vec::new(),
                exit: Some(ExitResult {
                    code: None,
                    signal: Some(*signal),
                }),
            }),
        };
        return ready;
    }
}

fn with_job<T>(
    id: &str,
    call: impl FnOnce(&mut ShellJob) -> Result<T, String>,
) -> Result<T, String> {
    JOBS.with(|jobs| {
        let mut jobs = jobs.borrow_mut();
        call(jobs.get_mut(id).ok_or("shell job is unknown")?)
    })
}

fn with_group(
    id: &str,
    call: impl FnOnce(&ExecutionGroup) -> Result<(), String>,
) -> Result<(), String> {
    with_job(id, |job| match job {
        ShellJob {
            group: Some(group), ..
        } => call(group),
        _ => Err("shell job is closed".into()),
    })
}
