use super::{ComponentExecutionRuntime, ProcessExecutionMode, ProcessSpec, run_and_collect};

pub(super) fn check(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let directory = std::env::temp_dir().join(format!(
        "rc-shell-redirect-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)?
            .as_nanos()
    ));
    std::fs::create_dir(&directory)?;
    let path = directory.join("output.txt");
    let mut spec = shell_spec(
        "manager-shell-redirect-check",
        "echo redirected > output.txt",
    );
    spec.cwd = directory.to_string_lossy().into_owned();
    anyhow::ensure!(run_and_collect(runtime.clone(), spec)?.is_empty());
    let mut append = shell_spec("manager-shell-append-check", "echo appended >> output.txt");
    append.cwd = directory.to_string_lossy().into_owned();
    anyhow::ensure!(run_and_collect(runtime.clone(), append)?.is_empty());
    let contents = std::fs::read(&path)?;
    let mut pipeline = shell_spec(
        "manager-shell-pipeline-redirect-check",
        "echo piped | cat > output.txt",
    );
    pipeline.cwd = directory.to_string_lossy().into_owned();
    anyhow::ensure!(run_and_collect(runtime.clone(), pipeline)?.is_empty());
    let pipeline_contents = std::fs::read(&path)?;
    let mut intermediate = shell_spec(
        "manager-shell-intermediate-redirect-check",
        "echo diverted > output.txt | cat",
    );
    intermediate.cwd = directory.to_string_lossy().into_owned();
    anyhow::ensure!(run_and_collect(runtime, intermediate)?.is_empty());
    let intermediate_contents = std::fs::read(&path)?;
    let _ = std::fs::remove_dir_all(directory);
    anyhow::ensure!(contents == b"redirected\nappended\n");
    anyhow::ensure!(pipeline_contents == b"piped\n");
    anyhow::ensure!(intermediate_contents == b"diverted\n");
    Ok(())
}

fn shell_spec(id: &str, script: &str) -> ProcessSpec {
    let mut spec = ProcessSpec::command(id, script);
    spec.mode = ProcessExecutionMode::RcShell {
        script: script.into(),
    };
    spec
}
