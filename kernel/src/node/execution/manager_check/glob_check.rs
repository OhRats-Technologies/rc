use super::{ComponentExecutionRuntime, ProcessExecutionMode, ProcessSpec, run_and_collect};

pub(super) fn check(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let directory = std::env::temp_dir().join(format!(
        "rc-shell-glob-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)?
            .as_nanos()
    ));
    std::fs::create_dir(&directory)?;
    std::fs::write(directory.join("b.rcglob"), b"")?;
    std::fs::write(directory.join("a.rcglob"), b"")?;
    std::fs::write(directory.join("ignored.txt"), b"")?;
    let mut spec = ProcessSpec::command("manager-shell-glob-check", "echo *.rcglob");
    spec.mode = ProcessExecutionMode::RcShell {
        script: "echo *.rcglob".into(),
    };
    spec.cwd = directory.to_string_lossy().into_owned();
    let output = run_and_collect(runtime, spec);
    let _ = std::fs::remove_dir_all(directory);
    anyhow::ensure!(
        output? == b"a.rcglob b.rcglob\n",
        "portable shell relative glob expansion failed"
    );
    Ok(())
}
