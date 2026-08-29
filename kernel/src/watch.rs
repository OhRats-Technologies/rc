use crate::runtime::Runtime;
use notify::{Config, RecommendedWatcher, RecursiveMode, Watcher};
use std::{
    path::Path,
    sync::mpsc::{self, RecvTimeoutError},
    time::Duration,
};

const DEBOUNCE: Duration = Duration::from_millis(150);
const SAFETY_RESCAN: Duration = Duration::from_secs(1);

pub fn run(runtime: &mut Runtime) -> anyhow::Result<()> {
    let (sender, receiver) = mpsc::channel();
    let mut watcher = RecommendedWatcher::new(sender, Config::default())?;
    watcher.watch(runtime.directory(), RecursiveMode::NonRecursive)?;
    runtime.reconcile()?;
    print_status(runtime);
    loop {
        match receiver.recv_timeout(SAFETY_RESCAN) {
            Ok(event) => {
                event?;
                while receiver.recv_timeout(DEBOUNCE).is_ok() {}
            }
            Err(RecvTimeoutError::Timeout) => {}
            Err(RecvTimeoutError::Disconnected) => anyhow::bail!("component watcher disconnected"),
        }
        if runtime.reconcile()? {
            print_status(runtime);
        }
    }
}

fn print_status(runtime: &Runtime) {
    eprintln!("RC component graph ({})", runtime.directory().display());
    for status in runtime.statuses() {
        eprintln!(
            "  {:<24} {:<12} {:?} {}{}",
            status.id,
            status.version,
            status.state,
            short_path(status.path),
            status
                .error
                .map(|error| format!(" — {error}"))
                .unwrap_or_default()
        );
    }
}

fn short_path(path: &Path) -> String {
    path.file_name()
        .map(|name| name.to_string_lossy().into_owned())
        .unwrap_or_else(|| path.to_string_lossy().into_owned())
}
