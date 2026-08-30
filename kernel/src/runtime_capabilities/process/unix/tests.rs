use super::*;
use crate::bindings::ohrats::rc_process::{
    process_host::SpawnRequest,
    types::{Environment, EnvironmentBase},
};
use std::{io::Read, thread, time::Duration};

fn environment(base: EnvironmentBase) -> Environment {
    Environment {
        base,
        changes: Vec::new(),
    }
}

fn request(program: &str, args: &[&str]) -> SpawnRequest {
    SpawnRequest {
        program: program.into(),
        args: args.iter().map(|value| (*value).into()).collect(),
        cwd: None,
        environment: environment(EnvironmentBase::Inherit),
        terminal: None,
    }
}

fn output(value: &mut StreamValue) -> Vec<u8> {
    let mut result = Vec::new();
    for _ in 0..400 {
        let read = match value {
            StreamValue::Reader(reader) => {
                let mut bytes = [0_u8; 4096];
                match reader.read(&mut bytes) {
                    Ok(0) => break,
                    Ok(length) => {
                        result.extend_from_slice(&bytes[..length]);
                        true
                    }
                    Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => false,
                    Err(error) => panic!("read failed: {error}"),
                }
            }
            _ => panic!("expected reader"),
        };
        if !read {
            thread::sleep(Duration::from_millis(5));
        }
    }
    result
}

#[test]
fn exact_argv_reaches_the_child_without_quoting() {
    let values = [
        "",
        "hello world",
        "'",
        "\"",
        "\\",
        "line\nbreak",
        "snowman ☃",
        "emoji 🐀",
        "-leading",
        r"C:\Program Files\RC",
        "/tmp/with space",
    ];
    let mut args = vec!["-c", "printf '%s\\0' \"$@\"", "argv-fixture"];
    args.extend(values);
    let mut group = Group::default();
    let mut spawned = spawn(&mut group, request("/bin/sh", &args)).unwrap();
    let actual = output(&mut spawned.stdout);
    let expected = values
        .iter()
        .flat_map(|value| value.as_bytes().iter().copied().chain([0]))
        .collect::<Vec<_>>();
    assert_eq!(actual, expected);
    while group.poll(spawned.native_child).unwrap().is_none() {
        thread::sleep(Duration::from_millis(2));
    }
}

#[test]
fn clean_environment_applies_set_and_unset() {
    let mut request = request("/usr/bin/env", &[]);
    request.environment = Environment {
        base: EnvironmentBase::Clean,
        changes: vec![
            crate::bindings::ohrats::rc_process::types::EnvironmentChange {
                name: "RC_VALUE".into(),
                value: Some("hello=world".into()),
            },
            crate::bindings::ohrats::rc_process::types::EnvironmentChange {
                name: "PATH".into(),
                value: None,
            },
        ],
    };
    let mut group = Group::default();
    let mut spawned = spawn(&mut group, request).unwrap();
    assert_eq!(output(&mut spawned.stdout), b"RC_VALUE=hello=world\n");
}

#[test]
fn killing_a_group_kills_parent_and_grandchild() {
    let mut group = Group::default();
    let mut spawned = spawn(
        &mut group,
        request("/bin/sh", &["-c", "sleep 30 & printf '%s\\n' \"$!\"; wait"]),
    )
    .unwrap();
    let pid = loop {
        let bytes = output(&mut spawned.stdout);
        if let Ok(value) = String::from_utf8(bytes)
            && let Ok(pid) = value.trim().parse::<i32>()
        {
            break pid;
        }
    };
    group.signal(Signal::Kill).unwrap();
    group.close();
    for _ in 0..100 {
        let result = unsafe { libc::kill(pid, 0) };
        if result < 0 && std::io::Error::last_os_error().raw_os_error() == Some(libc::ESRCH) {
            return;
        }
        thread::sleep(Duration::from_millis(5));
    }
    panic!("grandchild {pid} survived execution-group close");
}
