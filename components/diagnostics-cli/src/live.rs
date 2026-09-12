use crate::ohrats::rc_local_service::host;
use std::io::{IsTerminal as _, Write as _};

const SERVICE: &str = "rc.service";

pub fn logs(args: &[String]) -> Result<u32, String> {
    let follow = args.iter().any(|arg| arg == "--follow" || arg == "-f");
    let mut limit = 20;
    let mut numeric = false;
    for arg in args {
        if matches!(arg.as_str(), "--follow" | "-f" | "--service") {
            continue;
        }
        if numeric {
            return Err("usage: rc logs [--service] [--follow] [1..100]".into());
        }
        limit = arg
            .parse::<u32>()
            .map_err(|_| "usage: rc logs [--service] [--follow] [1..100]")?;
        numeric = true;
    }
    if !(1..=100).contains(&limit) {
        return Err("log limit must be between 1 and 100".into());
    }
    let mut cursor = String::new();
    loop {
        let events = host::logs(SERVICE, &cursor, limit)?;
        let full = events.len() == limit as usize;
        for event in events {
            cursor = event.cursor;
            println!(
                "{} {}",
                timestamp(&event.timestamp),
                escaped(&event.message)
            );
        }
        std::io::stdout().flush().map_err(|_| "log output closed")?;
        if !follow {
            return Ok(0);
        }
        if !full {
            host::wait(1000);
        }
    }
}

pub fn processes(args: &[String]) -> Result<u32, String> {
    if args
        .iter()
        .any(|arg| !matches!(arg.as_str(), "--watch" | "--commands"))
    {
        return Err("usage: rc ps [--watch] [--commands]".into());
    }
    let watch = args.iter().any(|arg| arg == "--watch");
    let commands = args.iter().any(|arg| arg == "--commands");
    loop {
        let processes = host::processes(SERVICE, commands)?;
        if watch && std::io::stdout().is_terminal() {
            print!("\x1b[H\x1b[2J");
        }
        println!(
            "PID\tPROCESS{}",
            if commands { "\tARGV (local, live)" } else { "" }
        );
        for process in &processes {
            print!("{}\t{}", process.pid, escaped(&process.name));
            if commands {
                // Debug string quoting preserves boundaries and escapes terminal controls.
                print!(
                    "\t{:?}{}",
                    process.argv,
                    if process.truncated {
                        " [truncated]"
                    } else {
                        ""
                    }
                );
            }
            println!();
        }
        println!(
            "{} processes in the RC user service.{}",
            processes.len(),
            if watch { " Ctrl-C stops watching." } else { "" }
        );
        std::io::stdout()
            .flush()
            .map_err(|_| "process output closed")?;
        if !watch {
            return Ok(0);
        }
        host::wait(2000);
    }
}

fn escaped(value: &str) -> String {
    value
        .chars()
        .flat_map(|c| {
            if c.is_control() {
                c.escape_default().collect::<Vec<_>>()
            } else {
                vec![c]
            }
        })
        .collect()
}

fn timestamp(value: &str) -> String {
    let Some(seconds) = value.parse::<u64>().ok().map(|v| v / 1_000_000) else {
        return "[unknown time]".into();
    };
    let days = seconds / 86400 + 719468;
    let era = days / 146097;
    let day = days % 146097;
    let year_in_era = (day - day / 1460 + day / 36524 - day / 146096) / 365;
    let day_in_year = day - (365 * year_in_era + year_in_era / 4 - year_in_era / 100);
    let month_from_march = (5 * day_in_year + 2) / 153;
    let date = day_in_year - (153 * month_from_march + 2) / 5 + 1;
    let month = if month_from_march < 10 {
        month_from_march + 3
    } else {
        month_from_march - 9
    };
    let year = year_in_era + era * 400 + u64::from(month <= 2);
    format!(
        "{year:04}-{month:02}-{date:02} {:02}:{:02}:{:02}Z",
        seconds / 3600 % 24,
        seconds / 60 % 60,
        seconds % 60
    )
}

#[cfg(test)]
mod tests {
    use super::{escaped, timestamp};
    #[test]
    fn journal_times_show_calendar_dates_in_utc() {
        assert_eq!(timestamp("0"), "1970-01-01 00:00:00Z");
        assert_eq!(timestamp("951782400000000"), "2000-02-29 00:00:00Z");
    }
    #[test]
    fn process_and_journal_text_cannot_inject_terminal_sequences() {
        assert_eq!(escaped("hello\x1b[2J\nworld"), "hello\\u{1b}[2J\\nworld");
    }
}
