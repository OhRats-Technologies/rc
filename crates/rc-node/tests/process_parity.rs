use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use nix::{errno::Errno, sys::signal::kill, unistd::Pid};
use rc_node::{ProcessEvent, ProcessManager, ProcessSpec};
use rc_protocol::TerminalSpec;
use std::{
    path::PathBuf,
    sync::mpsc::{self, Receiver},
    time::{Duration, Instant},
};

fn manager() -> (ProcessManager, Receiver<ProcessEvent>) {
    let (tx, rx) = mpsc::channel();
    let runner = PathBuf::from(env!("CARGO_BIN_EXE_rc-process-runner"));
    (
        ProcessManager::new(runner, move |event| {
            let _ = tx.send(event);
        }),
        rx,
    )
}

fn collect(id: &str, rx: &Receiver<ProcessEvent>) -> (Vec<u8>, Vec<u8>, i32) {
    let deadline = Instant::now() + Duration::from_secs(6);
    let mut stdout = Vec::new();
    let mut stderr = Vec::new();
    loop {
        let wait = deadline.saturating_duration_since(Instant::now());
        assert!(!wait.is_zero(), "timed out waiting for {id}");
        let event = rx.recv_timeout(wait).expect("process event");
        if event.id() != id {
            continue;
        }
        match event {
            ProcessEvent::Stdout { data, .. } => {
                stdout.extend(URL_SAFE_NO_PAD.decode(data).unwrap())
            }
            ProcessEvent::Stderr { data, .. } => {
                stderr.extend(URL_SAFE_NO_PAD.decode(data).unwrap())
            }
            ProcessEvent::Exit { exit_code, .. } => return (stdout, stderr, exit_code),
            ProcessEvent::Started { .. } => {}
        }
    }
}

#[test]
fn preserves_binary_stdout_and_separate_stderr() {
    let (manager, rx) = manager();
    manager
        .start(ProcessSpec::command(
            "binary",
            "printf '\\000\\377A'; printf 'err' >&2",
        ))
        .unwrap();
    let (stdout, stderr, code) = collect("binary", &rx);
    assert_eq!(code, 0);
    assert_eq!(stdout, [0, 0xff, b'A']);
    assert_eq!(stderr, b"err");
}

#[test]
fn closing_stdin_delivers_eof() {
    let (manager, rx) = manager();
    manager.start(ProcessSpec::command("stdin", "cat")).unwrap();
    manager.input("stdin", b"hello").unwrap();
    manager.close_input("stdin");
    let (stdout, stderr, code) = collect("stdin", &rx);
    assert_eq!(code, 0);
    assert_eq!(stdout, b"hello");
    assert!(stderr.is_empty());
}

#[test]
fn pty_merges_stdout_and_stderr() {
    let (manager, rx) = manager();
    let mut spec = ProcessSpec::command("pty", "printf out; printf err >&2");
    spec.terminal = Some(TerminalSpec {
        cols: 80,
        rows: 24,
        term: "xterm-256color".into(),
    });
    manager.start(spec).unwrap();
    let (stdout, stderr, code) = collect("pty", &rx);
    assert_eq!(code, 0);
    assert_eq!(stdout, b"outerr");
    assert!(stderr.is_empty());
}

#[test]
fn lifeline_kills_background_descendants() {
    let (manager, rx) = manager();
    manager
        .start(ProcessSpec::command("tree", "sleep 30 & echo $!; wait"))
        .unwrap();
    let deadline = Instant::now() + Duration::from_secs(3);
    let mut child_pid = None;
    while Instant::now() < deadline {
        if let Ok(ProcessEvent::Stdout { id, data }) = rx.recv_timeout(Duration::from_millis(200))
            && id == "tree"
        {
            let text = String::from_utf8(URL_SAFE_NO_PAD.decode(data).unwrap()).unwrap();
            child_pid = text.trim().parse::<i32>().ok();
            if child_pid.is_some() {
                break;
            }
        }
    }
    let pid = child_pid.expect("background pid");
    manager.signal("tree", "KILL").unwrap();
    let (_, _, code) = collect("tree", &rx);
    assert_eq!(code, 137);
    let deadline = Instant::now() + Duration::from_secs(2);
    loop {
        match kill(Pid::from_raw(pid), None) {
            Err(Errno::ESRCH) => break,
            _ if Instant::now() >= deadline => {
                panic!("background process {pid} survived lifeline teardown")
            }
            _ => std::thread::sleep(Duration::from_millis(25)),
        }
    }
}
