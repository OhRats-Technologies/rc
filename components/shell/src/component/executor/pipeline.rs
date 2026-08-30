use super::{ExitResult, NativeJob, Output, PollResult, Stage, State, Target};
use crate::component::ohrats::rc_process::{filesystem_host, process_host, types::StreamKind};

pub(super) fn poll(job: &mut NativeJob, max: u32) -> Result<PollResult, String> {
    let mut output = Vec::new();
    pump_input(job)?;
    pump_links(job, max)?;
    if let Some(last) = job.stages.last_mut() {
        match read_stage(last, max)? {
            process_host::ReadResult::Data(bytes) => {
                emit(last, StreamKind::Stdout, bytes, &mut output)?
            }
            process_host::ReadResult::Eof => last.stdout_eof = true,
            process_host::ReadResult::WouldBlock => {}
        }
    }
    for stage in &mut job.stages {
        if stage.stderr.is_some() {
            let read = stage.stderr.as_ref().expect("checked stderr").read(max)?;
            match read {
                process_host::ReadResult::Data(bytes) => {
                    emit(stage, StreamKind::Stderr, bytes, &mut output)?
                }
                process_host::ReadResult::Eof => stage.stderr_eof = true,
                process_host::ReadResult::WouldBlock => {}
            }
        } else {
            stage.stderr_eof = true;
        }
        if stage.exit.is_none()
            && let Some(child) = &stage.child
            && let Some(exit) = child.poll_exit()?
        {
            stage.exit = Some(ExitResult {
                code: exit.code,
                signal: exit.signal,
            });
        } else if stage.exit.is_none() && stage.virtual_io.is_some() && stage.stdout_eof {
            stage.exit = Some(ExitResult {
                code: stage.virtual_io.as_ref().map(|value| value.code),
                signal: None,
            });
        }
    }
    let exit = job.stages.last().and_then(|stage| stage.exit);
    let drained = job
        .stages
        .iter()
        .all(|stage| stage.exit.is_some() && stage.stdout_eof && stage.stderr_eof)
        && job
            .links
            .iter()
            .all(|link| link.upstream_eof && link.offset == link.pending.len());
    Ok(PollResult {
        state: if exit.is_some() && drained {
            State::Exited
        } else {
            State::Running
        },
        output,
        exit,
    })
}

pub(super) fn pump_input(job: &mut NativeJob) -> Result<(), String> {
    let Some((bytes, offset)) = &mut job.input else {
        return Ok(());
    };
    let first = job.stages.first_mut().ok_or("pipeline has no stages")?;
    if *offset < bytes.len()
        && let process_host::WriteResult::Accepted(count) = write_stage(first, &bytes[*offset..])?
    {
        *offset += count as usize;
    }
    if *offset == bytes.len() {
        close_stage_input(first)?;
        job.input = None;
    }
    Ok(())
}

pub(super) fn input(job: &mut NativeJob, bytes: &[u8]) -> Result<u32, String> {
    if job.redirected_input {
        return Err("shell stdin is redirected".into());
    }
    let first = job.stages.first_mut().ok_or("pipeline has no stages")?;
    match write_stage(first, bytes)? {
        process_host::WriteResult::Accepted(value) => Ok(value),
        process_host::WriteResult::WouldBlock => Ok(0),
    }
}

pub(super) fn close_input(job: &mut NativeJob) -> Result<(), String> {
    let first = job.stages.first_mut().ok_or("pipeline has no stages")?;
    close_stage_input(first)
}

pub(super) fn pump_links(job: &mut NativeJob, max: u32) -> Result<(), String> {
    for index in 0..job.links.len() {
        let (left, right) = job.stages.split_at_mut(index + 1);
        let upstream = &mut left[index];
        let downstream = &mut right[0];
        let link = &mut job.links[index];
        if link.offset == link.pending.len() && !link.upstream_eof {
            link.pending.clear();
            link.offset = 0;
            match read_stage(upstream, max)? {
                process_host::ReadResult::Data(bytes) => {
                    if upstream.stdout_target.is_some() {
                        emit(upstream, StreamKind::Stdout, bytes, &mut Vec::new())?;
                    } else {
                        link.pending = bytes;
                    }
                }
                process_host::ReadResult::Eof => {
                    link.upstream_eof = true;
                    upstream.stdout_eof = true;
                }
                process_host::ReadResult::WouldBlock => {}
            }
        }
        if link.offset < link.pending.len()
            && let process_host::WriteResult::Accepted(count) =
                write_stage(downstream, &link.pending[link.offset..])?
        {
            link.offset += count as usize;
        }
        if link.upstream_eof && link.offset == link.pending.len() {
            close_stage_input(downstream)?;
        }
    }
    Ok(())
}

fn emit(
    stage: &mut Stage,
    kind: StreamKind,
    bytes: Vec<u8>,
    output: &mut Vec<Output>,
) -> Result<(), String> {
    let sibling = match kind {
        StreamKind::Stdout => stage.stderr_target.as_ref(),
        StreamKind::Stderr => stage.stdout_target.as_ref(),
    }
    .map(|other| (other.path.clone(), other.written));
    let target = match kind {
        StreamKind::Stdout => &mut stage.stdout_target,
        StreamKind::Stderr => &mut stage.stderr_target,
    };
    let Some(target) = target else {
        output.push(Output { kind, bytes });
        return Ok(());
    };
    let sibling_written = sibling.is_some_and(|(path, written)| path == target.path && written);
    filesystem_host::write(
        &target.path,
        &bytes,
        target.append || target.written || sibling_written,
    )?;
    target.written = true;
    Ok(())
}

pub(super) fn target((path, append): (String, bool)) -> Target {
    Target {
        path,
        append,
        written: false,
    }
}

fn read_stage(stage: &mut Stage, max: u32) -> Result<process_host::ReadResult, String> {
    if let Some(stream) = &stage.stdout {
        return stream.read(max);
    }
    let value = stage
        .virtual_io
        .as_mut()
        .ok_or("pipeline stage has no stdout")?;
    if value.pending.is_empty()
        && let Some(producer) = &value.producer
    {
        while value.pending.len() < max as usize {
            value.pending.extend_from_slice(producer);
        }
    }
    if value.offset < value.pending.len() {
        let end = value
            .pending
            .len()
            .min(value.offset.saturating_add(max as usize));
        let bytes = value.pending[value.offset..end].to_vec();
        value.offset = end;
        if value.offset == value.pending.len() {
            value.pending.clear();
            value.offset = 0;
        }
        Ok(process_host::ReadResult::Data(bytes))
    } else if value.input_closed {
        Ok(process_host::ReadResult::Eof)
    } else {
        Ok(process_host::ReadResult::WouldBlock)
    }
}

fn write_stage(stage: &mut Stage, bytes: &[u8]) -> Result<process_host::WriteResult, String> {
    if let Some(stdin) = &stage.stdin {
        return stdin.write(bytes);
    }
    let value = stage
        .virtual_io
        .as_mut()
        .ok_or("pipeline stdin closed early")?;
    if !value.passthrough || value.input_closed {
        return Err("pipeline stdin closed early".into());
    }
    value.pending.extend_from_slice(bytes);
    Ok(process_host::WriteResult::Accepted(bytes.len() as u32))
}

fn close_stage_input(stage: &mut Stage) -> Result<(), String> {
    if let Some(stdin) = stage.stdin.take() {
        return stdin.close_write();
    }
    let value = stage
        .virtual_io
        .as_mut()
        .ok_or("pipeline stdin closed early")?;
    value.input_closed = true;
    Ok(())
}
