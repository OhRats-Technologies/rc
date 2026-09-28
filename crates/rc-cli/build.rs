fn main() {
    for path in [
        "src/service/host.rs",
        "src/service/host_job.rs",
        "src/service/host_retry.rs",
        "../rc-platform/src/service_log.rs",
    ] {
        println!("cargo:rerun-if-changed={path}");
    }
    if std::env::var("CARGO_CFG_TARGET_OS").as_deref() != Ok("windows") {
        return;
    }
    // Embed a native GUI-subsystem launcher so release archives remain single-binary.
    // It uses only std and Win32 and needs no scripting runtime or installed compiler.
    let output =
        std::path::PathBuf::from(std::env::var_os("OUT_DIR").unwrap()).join("rc-service.exe");
    let status = std::process::Command::new(std::env::var_os("RUSTC").unwrap())
        .args(["--edition=2024", "--crate-name", "rc_service", "--target"])
        .arg(std::env::var("TARGET").unwrap())
        .args([
            "-C",
            "opt-level=s",
            "-C",
            "panic=abort",
            "-C",
            "target-feature=+crt-static",
            "src/service/host.rs",
            "-o",
        ])
        .arg(output)
        .status()
        .expect("compile Windows service host");
    assert!(status.success(), "Windows service host compilation failed");
}
