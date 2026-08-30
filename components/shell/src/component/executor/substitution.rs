use super::{JobState, PollResult, State, builder::build_script, poll_state, sequence::Context};
use crate::component::ohrats::rc_process::{process_host::ExecutionGroup, types::StreamKind};
use crate::{Script, Word, WordPart};

const CAPTURE_LIMIT: usize = 16 * 1024 * 1024;

pub(super) struct PreparationJob {
    script: Option<Script>,
    context: Context,
    nested: Option<Box<JobState>>,
    slot: Option<Slot>,
    captured: Vec<u8>,
}

pub(super) enum Outcome {
    Polling(PollResult),
    Ready(JobState),
}

#[derive(Clone, Copy)]
enum WordKind {
    Assignment(usize),
    Argument(usize),
    Redirect(usize),
}

struct Slot {
    chain: usize,
    command: usize,
    word: WordKind,
    part: usize,
}

impl PreparationJob {
    pub fn new(script: Script, context: Context) -> Self {
        Self {
            script: Some(script),
            context,
            nested: None,
            slot: None,
            captured: Vec::new(),
        }
    }
}

pub(super) fn poll(
    job: &mut PreparationJob,
    group: &ExecutionGroup,
    max: u32,
) -> Result<Outcome, String> {
    let mut visible = Vec::new();
    loop {
        if let Some(nested) = &mut job.nested {
            let result = poll_state(nested, group, max)?;
            for chunk in result.output {
                if matches!(chunk.kind, StreamKind::Stdout) {
                    if job.captured.len().saturating_add(chunk.bytes.len()) > CAPTURE_LIMIT {
                        return Err("command substitution output exceeds capacity".into());
                    }
                    job.captured.extend(chunk.bytes);
                } else {
                    visible.push(chunk);
                }
            }
            if !matches!(result.state, State::Exited) {
                return Ok(Outcome::Polling(running(visible)));
            }
            let value = substitution_text(&job.captured)?;
            replace(
                job.script.as_mut().unwrap(),
                job.slot.take().unwrap(),
                value,
            );
            job.nested = None;
            job.captured.clear();
            continue;
        }
        let Some((slot, source)) = take_next(job.script.as_mut().unwrap()) else {
            let script = job.script.take().unwrap();
            return Ok(Outcome::Ready(build_script(
                script,
                job.context.clone(),
                group,
            )?));
        };
        let script = crate::parse(&source).map_err(|error| error.to_string())?;
        let mut context = job.context.clone();
        context.terminal = None;
        job.slot = Some(slot);
        job.nested = Some(Box::new(JobState::Preparing(PreparationJob::new(
            script, context,
        ))));
        if !visible.is_empty() {
            return Ok(Outcome::Polling(running(visible)));
        }
    }
}

fn running(output: Vec<super::Output>) -> PollResult {
    PollResult {
        state: State::Running,
        output,
        exit: None,
    }
}

fn substitution_text(bytes: &[u8]) -> Result<String, String> {
    let value = std::str::from_utf8(bytes)
        .map_err(|_| "command substitution output is not UTF-8")?
        .trim_end_matches('\n');
    Ok(value.into())
}

fn take_next(script: &mut Script) -> Option<(Slot, String)> {
    for (chain, chain_value) in script.chains.iter_mut().enumerate() {
        for (command, command_value) in chain_value.pipeline.commands.iter_mut().enumerate() {
            for (index, value) in command_value.assignments.iter_mut().enumerate() {
                if let Some((part, source)) = take_word(&mut value.value) {
                    return Some((
                        Slot {
                            chain,
                            command,
                            word: WordKind::Assignment(index),
                            part,
                        },
                        source,
                    ));
                }
            }
            for (index, value) in command_value.words.iter_mut().enumerate() {
                if let Some((part, source)) = take_word(value) {
                    return Some((
                        Slot {
                            chain,
                            command,
                            word: WordKind::Argument(index),
                            part,
                        },
                        source,
                    ));
                }
            }
            for (index, value) in command_value.redirects.iter_mut().enumerate() {
                if let Some((part, source)) = take_word(&mut value.target) {
                    return Some((
                        Slot {
                            chain,
                            command,
                            word: WordKind::Redirect(index),
                            part,
                        },
                        source,
                    ));
                }
            }
        }
    }
    None
}

fn take_word(word: &mut Word) -> Option<(usize, String)> {
    word.parts.iter_mut().enumerate().find_map(|(index, part)| {
        let WordPart::CommandSubstitution(source) = part else {
            return None;
        };
        let source = std::mem::take(source);
        *part = WordPart::Literal(String::new());
        Some((index, source))
    })
}

fn replace(script: &mut Script, slot: Slot, value: String) {
    let command = &mut script.chains[slot.chain].pipeline.commands[slot.command];
    let word = match slot.word {
        WordKind::Assignment(index) => &mut command.assignments[index].value,
        WordKind::Argument(index) => &mut command.words[index],
        WordKind::Redirect(index) => &mut command.redirects[index].target,
    };
    word.parts[slot.part] = WordPart::Literal(value);
}
