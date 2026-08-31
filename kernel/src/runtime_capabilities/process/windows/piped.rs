use super::{Group, NativeChild, Spawned, guard};
use crate::{
    bindings::ohrats::rc_process::process_host::SpawnRequest,
    runtime_capabilities::process::StreamValue,
};
use std::{
    os::windows::{io::AsRawHandle as _, process::CommandExt as _},
    process::{Command, Stdio},
};
use windows::Win32::{Foundation::HANDLE, System::Threading::CREATE_NEW_PROCESS_GROUP};

use super::support::{apply_std_environment, assign, display};

pub(super) fn spawn(group: &mut Group, request: SpawnRequest) -> Result<Spawned, String> {
    let gate = guard::LaunchGate::new()?;
    let mut command = Command::new(guard::executable()?);
    command
        .arg(guard::MARKER)
        .arg(&gate.name)
        .arg(&gate.ready_name)
        .arg(request.program)
        .args(request.args)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .creation_flags(CREATE_NEW_PROCESS_GROUP.0);
    apply_std_environment(&mut command, request.environment);
    if let Some(cwd) = request.cwd {
        command.current_dir(cwd);
    }
    let mut child = command.spawn().map_err(display)?;
    if let Err(error) = assign(group.job, HANDLE(child.as_raw_handle())) {
        let _ = child.kill();
        let _ = child.wait();
        return Err(error);
    }
    if let Err(error) = gate.wait_until_open() {
        let _ = child.kill();
        let _ = child.wait();
        return Err(error);
    }
    if let Err(error) = gate.release() {
        let _ = child.kill();
        let _ = child.wait();
        return Err(error);
    }
    let native_child = child.id();
    let stdin = child
        .stdin
        .take()
        .map(|value| StreamValue::Writer(Box::new(value)));
    let stdout = child
        .stdout
        .take()
        .ok_or_else(|| "stdout was not piped".to_owned())?;
    let stderr = child
        .stderr
        .take()
        .ok_or_else(|| "stderr was not piped".to_owned())?;
    group.process_groups.push(native_child);
    group
        .children
        .insert(native_child, NativeChild::Piped(child));
    Ok(Spawned {
        native_child,
        stdin,
        stdout: StreamValue::Reader(Box::new(stdout)),
        stderr: Some(StreamValue::Reader(Box::new(stderr))),
    })
}
