use super::*;
use ohrats::rc_process::types::{Action, Environment, EnvironmentBase, Principal};

fn principal(role: &str, execute: bool) -> Principal {
    Principal {
        user_id: "user-1".into(),
        role: role.into(),
        can_execute: execute,
        can_manage_devices: role == "owner",
    }
}

#[test]
fn owner_can_access_another_users_process() {
    let request = AccessRequest {
        execution_id: "process-1".into(),
        owner_user_id: "user-2".into(),
        action: Action::Signal,
        principal: principal("owner", true),
    };
    assert!(ProcessPolicy::authorize_access(request).is_ok());
}

#[test]
fn operator_cannot_access_another_users_process() {
    let request = AccessRequest {
        execution_id: "process-1".into(),
        owner_user_id: "user-2".into(),
        action: Action::Attach,
        principal: principal("operator", true),
    };
    assert_eq!(
        ProcessPolicy::authorize_access(request),
        Err("process access denied".into())
    );
}

#[test]
fn start_normalizes_terminal_name() {
    let request = StartRequest {
        execution_id: "process-1".into(),
        mode: ExecutionMode::Argv(("printf".into(), vec!["ok".into()])),
        cwd: None,
        environment: Environment {
            base: EnvironmentBase::Inherit,
            changes: Vec::new(),
        },
        terminal: Some(Terminal {
            cols: 80,
            rows: 24,
            term: " ".into(),
        }),
        channel: Channel::Control,
        lifetime: Lifetime::Attached,
        principal: principal("operator", true),
        max_runtime_ms: None,
    };
    let plan = ProcessPolicy::authorize_start(request).unwrap();
    assert!(matches!(plan.mode, ExecutionMode::Argv(_)));
    assert_eq!(plan.terminal.unwrap().term, "xterm-256color");
}

#[test]
fn exact_argv_preserves_empty_and_quoted_arguments() {
    let mode = ExecutionMode::Argv((
        "fixture".into(),
        vec!["".into(), "hello world".into(), "'".into(), "\n".into()],
    ));
    assert!(validate_mode(&mode).is_ok());
    let ExecutionMode::Argv((_, args)) = mode else {
        unreachable!()
    };
    assert_eq!(args, ["", "hello world", "'", "\n"]);
}

#[test]
fn portable_policy_preserves_case_distinct_environment_names() {
    let environment = Environment {
        base: EnvironmentBase::Inherit,
        changes: vec![
            ohrats::rc_process::types::EnvironmentChange {
                name: "Path".into(),
                value: Some("one".into()),
            },
            ohrats::rc_process::types::EnvironmentChange {
                name: "PATH".into(),
                value: Some("two".into()),
            },
        ],
    };
    assert!(validate_environment(&environment).is_ok());
}

#[test]
fn rejects_channel_lifetime_and_terminal_confusion() {
    let base = StartRequest {
        execution_id: "process-1".into(),
        mode: ExecutionMode::Argv(("fixture".into(), Vec::new())),
        cwd: None,
        environment: Environment {
            base: EnvironmentBase::Inherit,
            changes: Vec::new(),
        },
        terminal: None,
        channel: Channel::Mcp,
        lifetime: Lifetime::Attached,
        principal: principal("owner", true),
        max_runtime_ms: None,
    };
    assert!(ProcessPolicy::authorize_start(base.clone()).is_err());
    let mut terminal = base;
    terminal.lifetime = Lifetime::Managed;
    terminal.terminal = Some(Terminal {
        cols: 80,
        rows: 24,
        term: "xterm".into(),
    });
    assert!(ProcessPolicy::authorize_start(terminal).is_err());
}
