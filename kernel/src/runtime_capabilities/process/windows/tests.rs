use super::*;
use crate::bindings::ohrats::rc_process::types::{Environment, EnvironmentBase, EnvironmentChange};
use std::io::{BufRead, BufReader, Read};
use windows::Win32::{
    Foundation::{CloseHandle, WAIT_OBJECT_0},
    System::Threading::{OpenProcess, PROCESS_SYNCHRONIZE, WaitForSingleObject},
};

fn request(args: &[&str], terminal: Option<Terminal>) -> SpawnRequest {
    SpawnRequest {
        program: "cmd.exe".into(),
        args: args.iter().map(|value| (*value).to_owned()).collect(),
        cwd: None,
        environment: Environment {
            base: EnvironmentBase::Inherit,
            changes: Vec::new(),
        },
        terminal,
    }
}

fn wait(group: &mut Group, child: u32) -> NativeExit {
    for _ in 0..500 {
        if let Some(exit) = group.poll(child).unwrap() {
            return exit;
        }
        std::thread::sleep(std::time::Duration::from_millis(10));
    }
    panic!("Windows child did not exit")
}

fn wait_terminal(group: &mut Group, child: u32, stdout: StreamValue) -> (NativeExit, String) {
    let StreamValue::Reader(mut stdout) = stdout else {
        panic!("terminal output is not readable")
    };
    let output = std::sync::Arc::new(std::sync::Mutex::new(String::new()));
    let captured = output.clone();
    std::thread::spawn(move || {
        let mut buffer = [0_u8; 256];
        while let Ok(count) = stdout.read(&mut buffer) {
            if count == 0 {
                break;
            }
            captured
                .lock()
                .unwrap()
                .push_str(&String::from_utf8_lossy(&buffer[..count]));
        }
    });
    for _ in 0..500 {
        if let Some(exit) = group.poll(child).unwrap() {
            group.close();
            std::thread::sleep(std::time::Duration::from_millis(50));
            let output = output.lock().unwrap().clone();
            return (exit, output);
        }
        std::thread::sleep(std::time::Duration::from_millis(10));
    }
    group.close();
    std::thread::sleep(std::time::Duration::from_millis(50));
    let output = output.lock().unwrap().clone();
    panic!("Windows terminal child did not exit; output={output:?}")
}

#[test]
fn piped_process_receives_environment_and_separate_streams() {
    let mut group = Group::new().unwrap();
    let mut request = request(
        &["/D", "/S", "/C", "echo %RC_PROCESS_TEST%&echo error 1>&2"],
        None,
    );
    request.environment.changes.push(EnvironmentChange {
        name: "RC_PROCESS_TEST".into(),
        value: Some("hello world".into()),
    });
    let spawned = spawn(&mut group, request).unwrap();
    let StreamValue::Reader(mut stdout) = spawned.stdout else {
        panic!("stdout is not readable")
    };
    let StreamValue::Reader(mut stderr) = spawned.stderr.unwrap() else {
        panic!("stderr is not readable")
    };
    let mut out = String::new();
    let mut err = String::new();
    stdout.read_to_string(&mut out).unwrap();
    stderr.read_to_string(&mut err).unwrap();
    let exit = wait(&mut group, spawned.native_child);
    assert_eq!(exit.code, Some(0), "stdout={out:?} stderr={err:?}");
    assert!(out.contains("hello world"));
    assert!(err.contains("error"));
}

#[test]
fn unicode_exact_argv_boundaries_reach_the_child() {
    let expected = vec![
        "".to_owned(),
        "hello world".to_owned(),
        "single'quote".to_owned(),
        "double\"quote".to_owned(),
        r"C:\path with space\tail\".to_owned(),
        "line\nbreak".to_owned(),
        "雪🐀".to_owned(),
        "-leading".to_owned(),
    ];
    let mut args = vec!["--echo-argv".to_owned()];
    args.extend(expected.clone());
    let mut group = Group::new().unwrap();
    let spawned = spawn(
        &mut group,
        SpawnRequest {
            program: guard::executable().unwrap().to_string_lossy().into_owned(),
            args,
            cwd: None,
            environment: Environment {
                base: EnvironmentBase::Inherit,
                changes: Vec::new(),
            },
            terminal: None,
        },
    )
    .unwrap();
    assert_eq!(wait(&mut group, spawned.native_child).code, Some(0));
    let StreamValue::Reader(mut stdout) = spawned.stdout else {
        panic!("stdout is not readable")
    };
    let mut output = String::new();
    stdout.read_to_string(&mut output).unwrap();
    let actual: Vec<String> = serde_json::from_str(output.trim_start_matches('\u{feff}')).unwrap();
    assert_eq!(actual, expected);
}

#[test]
fn conflicting_case_insensitive_environment_keys_are_rejected() {
    let mut request = request(&["/C", "exit 0"], None);
    request.environment.changes = vec![
        EnvironmentChange {
            name: "PATH".into(),
            value: Some("one".into()),
        },
        EnvironmentChange {
            name: "Path".into(),
            value: Some("two".into()),
        },
    ];
    assert_eq!(
        validate_request(&request),
        Err("invalid or conflicting Windows environment change".into())
    );
}

#[test]
fn unicode_cwd_and_clean_set_unset_environment_reach_the_child() {
    let root = std::env::temp_dir().join(format!("rc-windows-cwd-{}-雪🐀", std::process::id()));
    std::fs::create_dir_all(&root).unwrap();
    let mut group = Group::new().unwrap();
    let spawned = spawn(
        &mut group,
        SpawnRequest {
            program: std::env::var("ComSpec").unwrap(),
            args: [
                "/D",
                "/S",
                "/C",
                "echo %CD%&echo %RC_SET%&if defined PATH exit /b 7",
            ]
            .into_iter()
            .map(str::to_owned)
            .collect(),
            cwd: Some(root.to_string_lossy().into_owned()),
            environment: Environment {
                base: EnvironmentBase::Clean,
                changes: vec![
                    EnvironmentChange {
                        name: "RC_SET".into(),
                        value: Some("value=雪".into()),
                    },
                    EnvironmentChange {
                        name: "PATH".into(),
                        value: None,
                    },
                ],
            },
            terminal: None,
        },
    )
    .unwrap();
    assert_eq!(wait(&mut group, spawned.native_child).code, Some(0));
    let StreamValue::Reader(mut stdout) = spawned.stdout else {
        panic!("stdout is not readable")
    };
    let mut output = String::new();
    stdout.read_to_string(&mut output).unwrap();
    assert!(
        output
            .to_lowercase()
            .contains(&root.to_string_lossy().to_lowercase())
    );
    assert!(output.contains("value=雪"));
    std::fs::remove_dir_all(root).unwrap();
}

#[test]
fn conpty_merges_output_resizes_and_stops_with_job() {
    let mut group = Group::new().unwrap();
    let spawned = spawn(
        &mut group,
        request(
            &["/D", "/C", "echo terminal"],
            Some(Terminal {
                cols: 80,
                rows: 24,
                term: "xterm-256color".into(),
            }),
        ),
    )
    .unwrap();
    group.resize(100, 40).unwrap();
    assert!(spawned.stderr.is_none());
    let (exit, output) = wait_terminal(&mut group, spawned.native_child, spawned.stdout);
    assert_eq!(exit.code, Some(0), "output={output:?}");
    assert!(output.contains("terminal"));
}

#[test]
fn job_kill_terminates_parent_and_grandchild() {
    let mut group = Group::new().unwrap();
    let script = concat!(
        "$child=Start-Process -FilePath ping.exe -ArgumentList @('-n','30','127.0.0.1') ",
        "-WindowStyle Hidden -PassThru;",
        "[Console]::Out.WriteLine($child.Id);[Console]::Out.Flush();",
        "Wait-Process -Id $child.Id"
    );
    let spawned = spawn(
        &mut group,
        SpawnRequest {
            program: "powershell.exe".into(),
            args: [
                "-NoLogo",
                "-NoProfile",
                "-NonInteractive",
                "-Command",
                script,
            ]
            .into_iter()
            .map(str::to_owned)
            .collect(),
            cwd: None,
            environment: Environment {
                base: EnvironmentBase::Inherit,
                changes: Vec::new(),
            },
            terminal: None,
        },
    )
    .unwrap();
    let StreamValue::Reader(stdout) = spawned.stdout else {
        panic!("stdout is not readable")
    };
    let mut line = String::new();
    BufReader::new(stdout).read_line(&mut line).unwrap();
    let grandchild = line.trim().parse::<u32>().unwrap();
    group.signal(Signal::Kill).unwrap();
    assert!(wait(&mut group, spawned.native_child).code.is_some());
    let handle = unsafe { OpenProcess(PROCESS_SYNCHRONIZE, false, grandchild) };
    let Ok(handle) = handle else { return };
    assert_eq!(unsafe { WaitForSingleObject(handle, 5_000) }, WAIT_OBJECT_0);
    unsafe { CloseHandle(handle) }.unwrap();
}

#[test]
fn one_execution_job_owns_every_pipeline_stage() {
    let mut group = Group::new().unwrap();
    let first = spawn(
        &mut group,
        request(&["/D", "/S", "/C", "ping -n 30 127.0.0.1 >nul"], None),
    )
    .unwrap();
    let second = spawn(
        &mut group,
        request(&["/D", "/S", "/C", "ping -n 30 127.0.0.1 >nul"], None),
    )
    .unwrap();
    assert_eq!(
        group.process_groups,
        [first.native_child, second.native_child]
    );
    group.signal(Signal::Kill).unwrap();
    assert!(wait(&mut group, first.native_child).code.is_some());
    assert!(wait(&mut group, second.native_child).code.is_some());
}

mod ownership;
mod signals;
mod streams;
