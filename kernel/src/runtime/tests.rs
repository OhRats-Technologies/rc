use super::*;

#[test]
fn vanished_path_is_retried_without_an_invalid_status() -> anyhow::Result<()> {
    let directory = tempfile::tempdir()?;
    let path = directory.path().join("provider.wasm");
    fs::write(&path, b"present during the desired scan")?;
    let mut runtime = Runtime::new(directory.path().to_path_buf())?;
    runtime.after_scan = Some(Box::new({
        let path = path.clone();
        move || fs::remove_file(path).expect("delete between scan and load")
    }));

    assert!(runtime.failed_paths.is_empty());
    assert!(!runtime.reconcile()?);
    assert!(runtime.statuses().is_empty());
    Ok(())
}

#[test]
fn invalid_existing_path_is_not_a_removal() -> anyhow::Result<()> {
    let directory = tempfile::tempdir()?;
    let path = directory.path().join("provider.wasm");
    fs::write(&path, b"not a WebAssembly component")?;
    let mut runtime = Runtime::new(directory.path().to_path_buf())?;

    let outcome = runtime.load_path(path.clone());
    assert!(!outcome.vanished);
    assert!(outcome.changed);
    assert!(runtime.failed_paths.contains_key(&path));
    Ok(())
}


#[test]
fn starts_with_a_writable_temporary_root() {
    let root = tempfile::tempdir().unwrap();
    let components = root.path().join("components");
    let runtime = Runtime::new(components.clone()).unwrap();

    assert_eq!(runtime.directory(), components);
    assert!(root.path().join("cache/wasmtime").is_dir());
    assert!(root.path().join("kernel.sqlite3").is_file());

    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt as _;
        let mode = std::fs::metadata(root.path().join("cache/wasmtime"))
            .unwrap()
            .permissions()
            .mode();
        assert_eq!(mode & 0o777, 0o700);
    }
}
