use crate::ohrats::{
    rc_diagnostics::{
        reporting,
        types::{Field, Level, Report},
    },
    rc_process::environment_host,
};

pub(crate) fn activate() -> Result<(), String> {
    let windows = matches!(
        environment_host::host_platform(),
        environment_host::PlatformKind::Windows
    );
    reporting::submit(&Report {
        level: Level::Info,
        source: "rc.runtime".into(),
        code: "runtime.active".into(),
        message: "portable execution runtime activated".into(),
        fields: vec![
            Field {
                name: "platform".into(),
                value: if windows { "windows" } else { "unix" }.into(),
            },
            Field {
                name: "process_backend".into(),
                value: if windows {
                    "job-object"
                } else {
                    "process-group"
                }
                .into(),
            },
            Field {
                name: "terminal_backend".into(),
                value: if windows { "conpty" } else { "pty" }.into(),
            },
        ],
    })
    .map(|_| ())
}

pub(crate) fn counts(counts: [usize; 3]) {
    let _ = reporting::submit(&Report {
        level: Level::Info,
        source: "rc.runtime".into(),
        code: "runtime.executions".into(),
        message: "execution counts changed".into(),
        fields: [
            ("attached", counts[0]),
            ("managed", counts[1]),
            ("scheduled", counts[2]),
        ]
        .into_iter()
        .map(|(name, value)| Field {
            name: name.into(),
            value: value.to_string(),
        })
        .collect(),
    });
}
