use super::{
    JobState, Link, NativeJob, ShellJob, Stage, VirtualIo, VirtualJob, builtins, expansion,
    pipeline, sequence, substitution,
};
use crate::component::{
    exports::ohrats::rc_shell::executor::StartRequest,
    ohrats::rc_process::process_host::{self, ExecutionGroup, SpawnRequest},
};
use expansion::{environment, expand_command, redirects};

pub(super) fn build_job(id: &str, request: StartRequest) -> Result<ShellJob, String> {
    let script = crate::parse(&request.script).map_err(|error| error.to_string())?;
    let case_insensitive_environment = expansion::case_insensitive_environment()?;
    let context = sequence::Context {
        environment: environment(request.environment, case_insensitive_environment)?,
        case_insensitive_environment,
        cwd: request.cwd,
        terminal: request.terminal,
    };
    let group = process_host::create_group(id)?;
    Ok(ShellJob {
        group: Some(group),
        state: JobState::Preparing(substitution::PreparationJob::new(script, context)),
    })
}

pub(super) fn build_script(
    script: crate::Script,
    mut context: sequence::Context,
    group: &ExecutionGroup,
) -> Result<JobState, String> {
    if script.chains.len() == 1 {
        build_pipeline(group, &script.chains[0].pipeline.commands, &mut context)
    } else {
        Ok(JobState::Sequence(sequence::SequenceJob::new(
            script, context, group,
        )?))
    }
}

pub(super) fn build_pipeline(
    group: &ExecutionGroup,
    commands: &[crate::Command],
    context: &mut sequence::Context,
) -> Result<JobState, String> {
    if let Some(job) = single_builtin(commands, context)? {
        return Ok(job);
    }
    if commands.len() > 1 && context.terminal.is_some() {
        return Err("portable shell pipelines do not use a terminal".into());
    }
    let mut job = NativeJob {
        stages: Vec::new(),
        links: (1..commands.len()).map(|_| Link::default()).collect(),
        input: None,
        redirected_input: false,
    };
    for (index, command) in commands.iter().enumerate() {
        add_stage(&mut job, group, command, index, context)?;
    }
    Ok(JobState::Native(job))
}

fn single_builtin(
    commands: &[crate::Command],
    context: &mut sequence::Context,
) -> Result<Option<JobState>, String> {
    let [command] = commands else { return Ok(None) };
    let (argv, changes) = expand_command(
        command,
        &context.environment,
        context.cwd.as_deref(),
        context.case_insensitive_environment,
    )?;
    expansion::apply_changes(
        &mut context.environment,
        changes,
        context.case_insensitive_environment,
    );
    let redirect = redirects(
        command,
        &context.environment,
        context.cwd.as_deref(),
        context.case_insensitive_environment,
    )?;
    if argv.is_empty() && !command.assignments.is_empty() {
        return Ok(Some(JobState::Virtual(VirtualJob {
            output: None,
            code: 0,
            shell_exit: false,
            stdout_target: None,
        })));
    }
    if argv.first().is_some_and(|value| value == "yes") {
        return Ok(None);
    }
    let Some(result) = builtins::run(&argv, context) else {
        return Ok(None);
    };
    let (output, code) = result?;
    Ok(Some(JobState::Virtual(VirtualJob {
        output,
        code,
        shell_exit: argv.first().is_some_and(|value| value == "exit"),
        stdout_target: redirect.stdout.map(pipeline::target),
    })))
}

fn add_stage(
    job: &mut NativeJob,
    group: &ExecutionGroup,
    command: &crate::Command,
    index: usize,
    context: &mut sequence::Context,
) -> Result<(), String> {
    let (argv, changes) = expand_command(
        command,
        &context.environment,
        context.cwd.as_deref(),
        context.case_insensitive_environment,
    )?;
    expansion::apply_changes(
        &mut context.environment,
        changes,
        context.case_insensitive_environment,
    );
    let redirect = redirects(
        command,
        &context.environment,
        context.cwd.as_deref(),
        context.case_insensitive_environment,
    )?;
    if index == 0 {
        job.redirected_input = redirect.stdin.is_some();
        job.input = redirect.stdin.map(|bytes| (bytes, 0));
    } else if redirect.stdin.is_some() {
        return Err("only the first pipeline stage may redirect stdin".into());
    }
    let stdout_target = redirect.stdout.map(pipeline::target);
    let stderr_target = redirect.stderr.map(pipeline::target);
    if let Some(result) = builtins::run(&argv, context) {
        let (output, code) = result?;
        job.stages.push(virtual_stage(
            &argv,
            output,
            code,
            stdout_target,
            stderr_target,
        ));
        return Ok(());
    }
    let (program, args) = argv.split_first().ok_or("shell command is empty")?;
    let spawned = group.spawn(&SpawnRequest {
        program: expansion::resolve_program(program, &context.environment)?,
        args: args.to_vec(),
        cwd: context.cwd.clone(),
        environment: expansion::host_environment(context.environment.clone()),
        terminal: context.terminal.clone(),
    })?;
    job.stages.push(Stage {
        child: Some(spawned.child),
        stdin: spawned.stdin,
        stdout: Some(spawned.stdout),
        stderr: spawned.stderr,
        stdout_target,
        stderr_target,
        virtual_io: None,
        exit: None,
        stdout_eof: false,
        stderr_eof: false,
    });
    Ok(())
}

fn virtual_stage(
    argv: &[String],
    output: Option<Vec<u8>>,
    code: u32,
    stdout_target: Option<super::Target>,
    stderr_target: Option<super::Target>,
) -> Stage {
    let passthrough = argv.first().is_some_and(|value| value == "cat") && argv.len() == 1;
    Stage {
        child: None,
        stdin: None,
        stdout: None,
        stderr: None,
        stdout_target,
        stderr_target,
        virtual_io: Some(VirtualIo {
            pending: output.unwrap_or_default(),
            producer: (argv.first().is_some_and(|value| value == "yes")).then(|| {
                let value = if argv.len() == 1 {
                    "y".into()
                } else {
                    argv[1..].join(" ")
                };
                format!("{value}\n").into_bytes()
            }),
            offset: 0,
            input_closed: !passthrough,
            passthrough,
            code,
        }),
        exit: None,
        stdout_eof: false,
        stderr_eof: true,
    }
}
