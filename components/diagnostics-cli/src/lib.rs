wit_bindgen::generate!({
    path: "../../wit",
    world: "diagnostics-cli",
    generate_all,
});

use ohrats::{
    rc_diagnostics::query,
    rc_plugin::types::{Command, Requirement, Selection},
};

struct DiagnosticsCli;
mod live;

impl Guest for DiagnosticsCli {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:diagnostics-cli".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: vec![Requirement {
                name: "ohrats:rc-diagnostics/query".into(),
                version: "^0.1".into(),
                selection: Selection::Single,
            }],
            commands: vec![
                Command {
                    name: "doctor".into(),
                    summary: "Show bounded local component diagnostics".into(),
                    usage: "rc doctor".into(),
                },
                Command {
                    name: "logs".into(),
                    summary: "Show the local service journal; use --follow for live updates".into(),
                    usage: "rc logs [--follow] [limit] | rc logs --components [limit]".into(),
                },
                Command {
                    name: "ps".into(),
                    summary: "Inspect live processes in the local Linux RC service".into(),
                    usage: "rc ps [--watch] [--commands]".into(),
                },
            ],
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        match command.as_str() {
            "doctor" => doctor(&args),
            "logs" => logs(&args),
            "ps" => live::processes(&args),
            _ => Err(format!("unsupported command {command:?}")),
        }
    }
}

fn doctor(args: &[String]) -> Result<u32, String> {
    if !args.is_empty() {
        return Err("usage: rc doctor".into());
    }
    let value = query::status();
    println!("RC diagnostics");
    println!("retained {}", value.retained);
    println!("warnings {}", value.warnings);
    println!("errors {}", value.errors);
    println!("newest sequence {}", value.newest_sequence);
    Ok(u32::from(value.errors > 0))
}

fn logs(args: &[String]) -> Result<u32, String> {
    if args.first().map(String::as_str) != Some("--components") {
        return live::logs(args);
    }
    let args = &args[1..];
    println!("Component diagnostics from this CLI invocation (not the background Node).");
    let limit = match args {
        [] => 20,
        [value] => value
            .parse::<u32>()
            .map_err(|_| "log limit must be an integer".to_owned())?,
        _ => return Err("usage: rc logs --components [limit]".into()),
    };
    for event in query::recent(limit)? {
        println!(
            "{} {:?} {} {}: {}",
            event.sequence, event.level, event.source, event.code, event.message
        );
        for field in event.fields {
            println!("  {}={}", field.name, field.value);
        }
    }
    Ok(0)
}

export!(DiagnosticsCli);
