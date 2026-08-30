use super::{JobState, PollResult, State, builder::build_pipeline, poll_state};
use crate::component::ohrats::rc_process::{process_host::ExecutionGroup, types::Terminal};
use crate::{Connector, Script};

#[derive(Clone)]
pub(super) struct Context {
    pub environment: super::expansion::EnvironmentValues,
    pub case_insensitive_environment: bool,
    pub cwd: Option<String>,
    pub terminal: Option<Terminal>,
}

pub(super) struct SequenceJob {
    script: Script,
    index: usize,
    context: Context,
    current: Box<JobState>,
}

impl SequenceJob {
    pub fn new(
        script: Script,
        mut context: Context,
        group: &ExecutionGroup,
    ) -> Result<Self, String> {
        let current = build_pipeline(group, &script.chains[0].pipeline.commands, &mut context)?;
        Ok(Self {
            script,
            index: 0,
            context,
            current: Box::new(current),
        })
    }
}

pub(super) fn poll(
    job: &mut SequenceJob,
    group: &ExecutionGroup,
    max: u32,
) -> Result<PollResult, String> {
    let mut output = Vec::new();
    loop {
        let mut result = poll_state(&mut job.current, group, max)?;
        output.append(&mut result.output);
        if !matches!(result.state, State::Exited) {
            result.output = output;
            return Ok(result);
        }
        let exit = result.exit;
        if matches!(&*job.current, JobState::Virtual(value) if value.shell_exit) {
            return Ok(PollResult {
                state: State::Exited,
                output,
                exit,
            });
        }
        let code = exit.and_then(|value| value.code).unwrap_or(1);
        let Some(next) = job
            .index
            .checked_add(1)
            .filter(|next| *next < job.script.chains.len())
        else {
            return Ok(PollResult {
                state: State::Exited,
                output,
                exit,
            });
        };
        let connector = job.script.chains[job.index]
            .next
            .unwrap_or(Connector::Always);
        job.index = next;
        if should_run(connector, code) {
            *job.current = build_pipeline(
                group,
                &job.script.chains[next].pipeline.commands,
                &mut job.context,
            )?;
        }
    }
}

pub(super) fn active_mut(state: &mut JobState) -> &mut JobState {
    match state {
        JobState::Sequence(job) => active_mut(&mut job.current),
        state @ JobState::Preparing(_) => state,
        state => state,
    }
}

fn should_run(connector: Connector, code: u32) -> bool {
    match connector {
        Connector::Always => true,
        Connector::And => code == 0,
        Connector::Or => code != 0,
    }
}
