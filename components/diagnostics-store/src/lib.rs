wit_bindgen::generate!({
    path: "../../wit",
    world: "diagnostics-store",
    generate_all,
});

use exports::ohrats::rc_diagnostics::{
    query::Guest as QueryGuest, reporting::Guest as ReportingGuest,
};
use ohrats::{
    rc_diagnostics::types::{Event, Field, Health, Level, Report},
    rc_plugin::types::Service,
};
use std::{
    cell::{Cell, RefCell},
    collections::VecDeque,
    time::{SystemTime, UNIX_EPOCH},
};

const MAX_EVENTS: usize = 256;
const MAX_FIELDS: usize = 16;

thread_local! {
    static EVENTS: RefCell<VecDeque<Event>> = const { RefCell::new(VecDeque::new()) };
    static SEQUENCE: Cell<u64> = const { Cell::new(0) };
}

struct DiagnosticsStore;

impl Guest for DiagnosticsStore {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:diagnostics-store".into(),
            version: "0.1.0".into(),
            provides: vec![
                Service {
                    name: "ohrats:rc-diagnostics/query".into(),
                    version: "0.1.0".into(),
                    priority: 100,
                    keys: Vec::new(),
                },
                Service {
                    name: "ohrats:rc-diagnostics/reporting".into(),
                    version: "0.1.0".into(),
                    priority: 100,
                    keys: Vec::new(),
                },
            ],
            requires: Vec::new(),
            commands: Vec::new(),
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        Err(format!("unsupported command {command:?}"))
    }
}

impl ReportingGuest for DiagnosticsStore {
    fn submit(value: Report) -> Result<u64, String> {
        validate_report(&value)?;
        let sequence = SEQUENCE.with(|current| {
            let next = current.get().saturating_add(1);
            current.set(next);
            next
        });
        let event = Event {
            sequence,
            timestamp_ms: timestamp_ms(),
            level: value.level,
            source: value.source,
            code: value.code,
            message: value.message,
            fields: value.fields,
        };
        EVENTS.with(|events| {
            let mut events = events.borrow_mut();
            events.push_back(event);
            while events.len() > MAX_EVENTS {
                events.pop_front();
            }
        });
        Ok(sequence)
    }
}

impl QueryGuest for DiagnosticsStore {
    fn recent(limit: u32) -> Result<Vec<Event>, String> {
        if limit > 100 {
            return Err("diagnostic query limit exceeds 100".into());
        }
        Ok(EVENTS.with(|events| {
            events
                .borrow()
                .iter()
                .rev()
                .take(limit as usize)
                .cloned()
                .collect()
        }))
    }

    fn status() -> Health {
        EVENTS.with(|events| {
            let events = events.borrow();
            Health {
                retained: events.len() as u32,
                newest_sequence: events.back().map_or(0, |event| event.sequence),
                errors: events
                    .iter()
                    .filter(|event| event.level == Level::Error)
                    .count() as u32,
                warnings: events
                    .iter()
                    .filter(|event| event.level == Level::Warn)
                    .count() as u32,
            }
        })
    }
}

fn validate_report(value: &Report) -> Result<(), String> {
    validate_token(&value.source, 96, "diagnostic source")?;
    validate_token(&value.code, 96, "diagnostic code")?;
    if value.message.is_empty() || value.message.len() > 512 {
        return Err("diagnostic message must contain 1 to 512 bytes".into());
    }
    if value.fields.len() > MAX_FIELDS {
        return Err("diagnostic report has too many fields".into());
    }
    for field in &value.fields {
        validate_field(field)?;
    }
    Ok(())
}

fn validate_field(field: &Field) -> Result<(), String> {
    validate_token(&field.name, 64, "diagnostic field")?;
    if field.value.len() > 256 {
        return Err("diagnostic field value exceeds 256 bytes".into());
    }
    let name = field.name.to_ascii_lowercase();
    if [
        "command",
        "credential",
        "input",
        "key",
        "output",
        "secret",
        "token",
        "transcript",
    ]
    .iter()
    .any(|word| name.contains(word))
    {
        return Err(format!(
            "diagnostic field {:?} may contain plaintext",
            field.name
        ));
    }
    Ok(())
}

fn validate_token(value: &str, maximum: usize, label: &str) -> Result<(), String> {
    if !value.is_empty()
        && value.len() <= maximum
        && value.bytes().all(|byte| {
            byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'-' | b'_' | b':' | b'/')
        })
    {
        Ok(())
    } else {
        Err(format!("invalid {label} {value:?}"))
    }
}

fn timestamp_ms() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |duration| duration.as_millis() as u64)
}

#[cfg(test)]
mod tests {
    use super::{Field, Level, Report, validate_report};

    #[test]
    fn rejects_fields_that_may_contain_control_plaintext() {
        let value = Report {
            level: Level::Info,
            source: "rc.test".into(),
            code: "test.event".into(),
            message: "bounded metadata".into(),
            fields: vec![Field {
                name: "command_text".into(),
                value: "whoami".into(),
            }],
        };
        assert!(validate_report(&value).is_err());
    }

    #[test]
    fn accepts_bounded_operational_metadata() {
        let value = Report {
            level: Level::Warn,
            source: "rc.transport".into(),
            code: "connection.closed".into(),
            message: "peer connection ended".into(),
            fields: vec![Field {
                name: "peer".into(),
                value: "device-1".into(),
            }],
        };
        assert!(validate_report(&value).is_ok());
    }
}

export!(DiagnosticsStore);
