use super::{ComponentExecutionRuntime, run_and_collect};
use rc_node::{ProcessExecutionMode, ProcessSpec};

pub(super) fn check(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    check_exact_argv(runtime.clone())?;
    check_external_pipeline(runtime.clone())?;
    check_external_substitution(runtime.clone())?;
    check_relative_cwd(runtime)
}

fn check_exact_argv(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let values = [
        "",
        "hello world",
        "single'quote",
        "double\"quote",
        "back\\slash",
        "line\nbreak",
        "雪🐀",
        "-leading",
    ];
    let executable = std::env::current_exe()?.to_string_lossy().into_owned();
    let mut words = Vec::with_capacity(values.len() + 2);
    words.push(shell_quote(&executable));
    words.push("argv-fixture".into());
    words.extend(values.iter().map(|value| shell_quote(value)));
    let script = words.join(" ");
    let mut spec = ProcessSpec::command("manager-shell-argv-check", &script);
    spec.mode = ProcessExecutionMode::RcShell { script };
    let actual = run_and_collect(runtime, spec)?;
    let mut expected = Vec::new();
    for value in values {
        expected.extend_from_slice(&(value.len() as u64).to_le_bytes());
        expected.extend_from_slice(value.as_bytes());
    }
    anyhow::ensure!(actual == expected, "portable shell changed argv boundaries");
    Ok(())
}

fn check_external_pipeline(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let executable = std::env::current_exe()?.to_string_lossy().into_owned();
    let script = format!("{} argv-fixture piped | cat", shell_quote(&executable));
    let mut spec = ProcessSpec::command("manager-shell-external-pipeline-check", &script);
    spec.mode = ProcessExecutionMode::RcShell { script };
    let actual = run_and_collect(runtime, spec)?;
    let mut expected = (5_u64).to_le_bytes().to_vec();
    expected.extend_from_slice(b"piped");
    anyhow::ensure!(
        actual == expected,
        "portable shell external pipeline changed binary output"
    );
    Ok(())
}

fn check_external_substitution(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let executable = std::env::current_exe()?.to_string_lossy().into_owned();
    let script = format!(
        "echo $({} text-fixture external-substitution)",
        shell_quote(&executable)
    );
    let mut spec = ProcessSpec::command("manager-shell-external-substitution-check", &script);
    spec.mode = ProcessExecutionMode::RcShell { script };
    anyhow::ensure!(
        run_and_collect(runtime, spec)? == b"external-substitution\n",
        "portable shell external command substitution failed"
    );
    Ok(())
}

fn check_relative_cwd(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let root = std::env::temp_dir().join(format!("rc-shell-cwd-{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&root);
    std::fs::create_dir_all(&root)?;
    let script = format!(
        "cd {} ; mkdir child ; cd child ; touch marker",
        shell_quote(&root.to_string_lossy())
    );
    let mut spec = ProcessSpec::command("manager-shell-cwd-check", &script);
    spec.mode = ProcessExecutionMode::RcShell { script };
    let result = run_and_collect(runtime, spec);
    let exists = root.join("child/marker").is_file();
    let _ = std::fs::remove_dir_all(&root);
    result?;
    anyhow::ensure!(
        exists,
        "portable shell resolved relative cd against the wrong cwd"
    );
    Ok(())
}

fn shell_quote(value: &str) -> String {
    format!("'{}'", value.replace('\'', "'\\''"))
}
