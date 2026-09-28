//! Native fixture: assert no console, emit both streams, and own a descendant.
use std::{
    os::windows::process::CommandExt,
    process::{Command, Stdio},
    time::Duration,
};
#[link(name = "kernel32")]
unsafe extern "system" {
    fn GetConsoleWindow() -> *mut std::ffi::c_void;
}
fn main() {
    let args: Vec<_> = std::env::args().collect();
    if args[1] == "child" {
        loop {
            std::thread::sleep(Duration::from_secs(1));
        }
    }
    let console = unsafe { GetConsoleWindow() } as usize;
    let mut child = Command::new(std::env::current_exe().unwrap())
        .arg("child")
        .creation_flags(0x08000000)
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    println!("fixture stdout ✓");
    eprintln!("fixture stderr ✓");
    std::fs::write(
        &args[2],
        format!("{} {console} {}", std::process::id(), child.id()),
    )
    .unwrap();
    if args[1] == "exit" {
        std::process::exit(17);
    }
    let _ = child.wait();
}
