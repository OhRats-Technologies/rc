use rc_node::acquire_run_lock;

#[test]
fn run_lock_is_exclusive_and_reusable() -> std::io::Result<()> {
    let root = std::env::temp_dir().join(format!(
        "rc-run-lock-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_nanos()
    ));
    let first = acquire_run_lock(&root)?;
    let error = acquire_run_lock(&root)
        .err()
        .expect("second lock must fail");
    assert_eq!(error.kind(), std::io::ErrorKind::AlreadyExists);
    drop(first);
    let second = acquire_run_lock(&root)?;
    drop(second);
    let _ = std::fs::remove_dir_all(root);
    Ok(())
}
