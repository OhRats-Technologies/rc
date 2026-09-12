use crate::{
    bindings::ohrats::rc_local_service::host::{Host, LogEntry, Process},
    host::HostState,
};

#[cfg(target_os = "linux")]
mod linux;

impl Host for HostState {
    fn logs(
        &mut self,
        service: String,
        cursor: String,
        limit: u32,
    ) -> Result<Vec<LogEntry>, String> {
        authorized(self, &service)?;
        if !(1..=100).contains(&limit) || cursor.len() > 4096 || cursor.contains('\0') {
            return Err("invalid local service log query".into());
        }
        #[cfg(target_os = "linux")]
        return linux::logs(&service, &cursor, limit);
        #[cfg(not(target_os = "linux"))]
        Err("live service inspection currently requires Linux with systemd".into())
    }

    fn processes(&mut self, service: String, commands: bool) -> Result<Vec<Process>, String> {
        authorized(self, &service)?;
        #[cfg(target_os = "linux")]
        return linux::processes(&service, commands);
        #[cfg(not(target_os = "linux"))]
        {
            let _ = commands;
            Err("live service inspection currently requires Linux with systemd".into())
        }
    }

    fn wait(&mut self, milliseconds: u32) {
        std::thread::sleep(std::time::Duration::from_millis(u64::from(
            milliseconds.min(5000),
        )));
    }
}

fn authorized(host: &HostState, service: &str) -> Result<(), String> {
    if host.plugin_id() != "ohrats:diagnostics-cli" {
        return Err("component is not granted local service inspection".into());
    }
    if service.starts_with('-')
        || service.len() > 256
        || !service.ends_with(".service")
        || !service
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b"-_.@".contains(&b))
    {
        return Err("invalid service name".into());
    }
    Ok(())
}
