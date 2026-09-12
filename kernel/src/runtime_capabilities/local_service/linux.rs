use super::{LogEntry, Process};
use std::{
    fs,
    io::Read,
    path::Path,
    process::{Command, Stdio},
};

const MAX_OUTPUT: u64 = 512 * 1024;

fn output(program: &str, args: &[&str]) -> Result<String, String> {
    let mut child = Command::new(program)
        .args(args)
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .spawn()
        .map_err(|_| {
            format!("could not start {program}; Linux service inspection requires systemd tools")
        })?;
    let mut bytes = Vec::new();
    let result = child
        .stdout
        .take()
        .ok_or("service output pipe unavailable")?
        .take(MAX_OUTPUT + 1)
        .read_to_end(&mut bytes);
    if result.is_err() || bytes.len() as u64 > MAX_OUTPUT {
        let _ = child.kill();
        let _ = child.wait();
        return Err("service inspection output exceeded capacity or could not be read".into());
    }
    let status = child.wait().map_err(|_| "service inspection wait failed")?;
    if !status.success() {
        return Err(format!(
            "{program} failed; check that the user service and journal are accessible"
        ));
    }
    String::from_utf8(bytes).map_err(|_| "service inspection returned invalid UTF-8".into())
}

pub(super) fn logs(service: &str, cursor: &str, limit: u32) -> Result<Vec<LogEntry>, String> {
    let limit = limit.to_string();
    let mut args = vec!["--user", "-u", service, "-o", "json", "--no-pager"];
    if cursor.is_empty() {
        args.extend(["-n", &limit]);
    } else {
        // Forward ordering is essential: -n would skip intervening events after a burst.
        args.extend(["--after-cursor", cursor]);
    }
    let data = output("journalctl", &args)?;
    parse_logs(&data, limit.parse().unwrap_or(20))
}

fn parse_logs(data: &str, limit: u32) -> Result<Vec<LogEntry>, String> {
    data.lines()
        .take(limit as usize)
        .map(|line| {
            let value: serde_json::Value =
                serde_json::from_str(line).map_err(|_| "invalid journal record")?;
            Ok(LogEntry {
                cursor: value["__CURSOR"]
                    .as_str()
                    .ok_or("journal cursor missing")?
                    .into(),
                timestamp: value["__REALTIME_TIMESTAMP"]
                    .as_str()
                    .unwrap_or("unknown")
                    .into(),
                message: value["MESSAGE"]
                    .as_str()
                    .unwrap_or("[binary journal message]")
                    .into(),
            })
        })
        .collect()
}

pub(super) fn processes(service: &str, commands: bool) -> Result<Vec<Process>, String> {
    let group = output(
        "systemctl",
        &[
            "--user",
            "show",
            service,
            "--property=ControlGroup",
            "--value",
        ],
    )?;
    let group = group.trim();
    if group.is_empty() {
        return Err("service is not running".into());
    }
    let relative = group
        .strip_prefix('/')
        .ok_or("invalid service control group")?;
    if Path::new(relative)
        .components()
        .any(|part| !matches!(part, std::path::Component::Normal(_)))
    {
        return Err("invalid service control group".into());
    }
    let root = Path::new("/sys/fs/cgroup").join(relative);
    let mut pids = std::collections::BTreeSet::new();
    collect_pids(&root, &mut pids, 0)?;
    if pids.len() > 256 {
        return Err("service process list exceeds 256 processes".into());
    }
    let mut result = Vec::new();
    for pid in pids {
        let root = format!("/proc/{pid}");
        let Ok(name) = fs::read_to_string(format!("{root}/comm")) else {
            continue;
        };
        let (argv, truncated) = if commands {
            let Ok(file) = fs::File::open(format!("{root}/cmdline")) else {
                continue;
            };
            let mut data = Vec::new();
            if file.take(8193).read_to_end(&mut data).is_err() {
                continue;
            }
            let truncated = data.len() > 8192;
            data.truncate(8192);
            (parse_argv(&data), truncated)
        } else {
            (Vec::new(), false)
        };
        result.push(Process {
            pid,
            name: name.trim_end().into(),
            argv,
            truncated,
        });
    }
    Ok(result)
}

fn parse_argv(data: &[u8]) -> Vec<String> {
    if data.is_empty() {
        return Vec::new();
    }
    data.strip_suffix(&[0])
        .unwrap_or(data)
        .split(|b| *b == 0)
        .map(|s| String::from_utf8_lossy(s).into_owned())
        .collect()
}

fn collect_pids(
    root: &Path,
    pids: &mut std::collections::BTreeSet<u32>,
    depth: u32,
) -> Result<(), String> {
    if depth > 8 || pids.len() > 256 {
        return Err("service process tree exceeds inspection capacity".into());
    }
    let text = fs::read_to_string(root.join("cgroup.procs"))
        .map_err(|_| "service process group is unavailable")?;
    pids.extend(text.lines().filter_map(|line| line.parse::<u32>().ok()));
    for entry in fs::read_dir(root).map_err(|_| "service process group is unavailable")? {
        let entry = entry.map_err(|_| "service process group entry unavailable")?;
        if entry
            .file_type()
            .map_err(|_| "service process group entry unavailable")?
            .is_dir()
        {
            collect_pids(&entry.path(), pids, depth + 1)?;
        }
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn argv_preserves_empty_arguments_and_empty_processes() {
        assert!(parse_argv(b"").is_empty());
        assert_eq!(parse_argv(b"cmd\0\0last\0"), ["cmd", "", "last"]);
    }
    #[test]
    fn cursor_pages_preserve_order_and_do_not_skip_to_newest() {
        let data = "{\"__CURSOR\":\"one\",\"MESSAGE\":\"first\"}\n{\"__CURSOR\":\"two\",\"MESSAGE\":\"second\"}";
        let page = parse_logs(data, 1).unwrap();
        assert_eq!(page[0].cursor, "one");
        assert_eq!(page[0].message, "first");
        assert!(parse_logs("{}", 1).is_err());
    }
}
