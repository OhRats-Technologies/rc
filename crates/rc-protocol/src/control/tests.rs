use super::*;

#[test]
fn exact_argv_round_trips_without_quoting() {
    let values = vec![
        "".into(),
        "hello world".into(),
        "'".into(),
        "\"".into(),
        "\\".into(),
        "line\nbreak".into(),
        "emoji 🐀".into(),
        "-leading".into(),
    ];
    let message = ControlMessage::ProcessStart {
        id: "process-1".into(),
        mode: ExecutionMode::Argv {
            program: "fixture".into(),
            args: values.clone(),
        },
        cwd: Some("/tmp/with space".into()),
        environment: EnvironmentSpec::default(),
        terminal: None,
    };
    let decoded: ControlMessage =
        serde_json::from_slice(&serde_json::to_vec(&message).unwrap()).unwrap();
    let ControlMessage::ProcessStart {
        mode: ExecutionMode::Argv { program, args },
        ..
    } = decoded
    else {
        panic!("wrong control message")
    };
    assert_eq!(program, "fixture");
    assert_eq!(args, values);
}

#[test]
fn login_shell_intent_contains_no_unix_command() {
    let message = ControlMessage::ProcessStart {
        id: "process-1".into(),
        mode: ExecutionMode::SystemLoginShell,
        cwd: None,
        environment: EnvironmentSpec::default(),
        terminal: Some(TerminalSpec {
            cols: 80,
            rows: 24,
            term: "xterm-256color".into(),
        }),
    };
    let encoded = String::from_utf8(serde_json::to_vec(&message).unwrap()).unwrap();
    assert!(encoded.contains("\"kind\":\"systemLoginShell\""));
    assert!(!encoded.contains("SHELL"));
    assert!(!encoded.contains("exec"));
}

#[test]
fn schedule_hash_binds_execution_but_not_mutable_display_state() {
    let mut schedule = ScheduleDefinition {
        id: "schedule-1".into(),
        name: Some("Nightly".into()),
        cron: "0 1 * * *".into(),
        timezone: "America/Toronto".into(),
        mode: ExecutionMode::Argv {
            program: "fixture".into(),
            args: vec!["hello world".into()],
        },
        cwd: None,
        environment: EnvironmentSpec::default(),
        enabled: true,
        misfire: ScheduleMisfirePolicy::Skip,
        max_runtime_ms: Some(60_000),
        permit_hash: String::new(),
        created_by: "owner".into(),
        created_at_ms: 1,
        expires_at_ms: None,
    };
    let initial = schedule_spec_hash(&schedule);
    schedule.name = Some("Renamed".into());
    schedule.enabled = false;
    schedule.created_at_ms = 2;
    schedule.permit_hash = "server-copy".into();
    assert_eq!(schedule_spec_hash(&schedule), initial);
    schedule.cron = "0 2 * * *".into();
    assert_ne!(schedule_spec_hash(&schedule), initial);
}
