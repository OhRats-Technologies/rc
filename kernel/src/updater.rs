mod recovery;

use crate::bindings::ohrats::rc_updater::{
    artifact_source::Host as ArtifactSourceHost,
    native_replacement::{CurrentState, Host, Staged},
};
#[cfg(not(windows))]
use std::process::Command;
#[cfg(not(windows))]
use std::collections::BTreeMap;
use std::{fs, path::PathBuf};

const MAX_ARTIFACT: usize = 160 * 1024 * 1024;

#[derive(Default)]
pub(crate) struct NativeReplacement {
    #[cfg(not(windows))]
    prepared: BTreeMap<String, Prepared>,
    #[cfg(not(windows))]
    next_id: u64,
}

#[derive(Default)]
pub(crate) struct ArtifactSource {
    selected: Option<(String, PathBuf)>,
    next_id: u64,
}

#[cfg(not(windows))]
struct Prepared {
    stage: PathBuf,
    target: PathBuf,
    digest: String,
}

impl Host for crate::host::HostState {
    fn current(&mut self) -> Result<CurrentState, String> {
        let target = NativeReplacement::target()?;
        Ok(CurrentState {
            version: recovery::verify_kernel(&target)?,
            digest: recovery::digest_file(&target)?,
        })
    }

    fn prepare(&mut self, artifact: Vec<u8>, expected: String) -> Result<Staged, String> {
        #[cfg(windows)]
        {
            let _ = (artifact, expected);
            Err(windows_upgrade_error())
        }
        #[cfg(not(windows))]
        {
            recovery::validate_digest(&expected)?;
            if artifact.len() > MAX_ARTIFACT || recovery::digest(&artifact) != expected {
                return Err("kernel artifact is oversized or not digest-pinned".into());
            }
            let target = NativeReplacement::target()?;
            let parent = target
                .parent()
                .ok_or_else(|| "kernel has no parent".to_owned())?;
            recovery::recover(parent, &target)?;
            let token = format!("{}-{}", std::process::id(), self.replacement.next_id);
            self.replacement.next_id = self.replacement.next_id.wrapping_add(1);
            let stage = parent.join(format!(".rc-kernel-stage-{token}"));
            recovery::write_executable(&stage, &artifact)?;
            let version = recovery::verify_kernel(&stage)?;
            self.replacement.prepared.insert(
                token.clone(),
                Prepared {
                    stage,
                    target,
                    digest: expected.clone(),
                },
            );
            Ok(Staged {
                token,
                digest: expected,
                version,
            })
        }
    }

    fn commit(&mut self, staged: Staged) -> Result<(), String> {
        #[cfg(windows)]
        {
            let _ = staged;
            Err(windows_upgrade_error())
        }
        #[cfg(not(windows))]
        {
            let prepared = self
                .replacement
                .prepared
                .remove(&staged.token)
                .ok_or_else(|| "unknown native replacement".to_owned())?;
            match recovery::commit(&prepared.stage, &prepared.target, &prepared.digest) {
                Ok(()) => Ok(()),
                Err(error) => {
                    let _ = recovery::recover(
                        prepared
                            .target
                            .parent()
                            .unwrap_or(std::path::Path::new(".")),
                        &prepared.target,
                    );
                    Err(error)
                }
            }
        }
    }

    fn abort(&mut self, staged: Staged) {
        #[cfg(windows)]
        let _ = staged;
        #[cfg(not(windows))]
        {
            if let Some(prepared) = self.replacement.prepared.remove(&staged.token) {
                let _ = fs::remove_file(prepared.stage);
            }
        }
    }

    fn reexec(&mut self) -> Result<(), String> {
        #[cfg(not(windows))]
        let executable =
            std::env::current_exe().map_err(|error| format!("resolve kernel: {error}"))?;
        #[cfg(unix)]
        {
            use std::os::unix::process::CommandExt;
            let error = Command::new(executable)
                .args(std::env::args_os().skip(1))
                .exec();
            Err(format!("re-exec kernel: {error}"))
        }
        #[cfg(all(not(unix), not(windows)))]
        {
            let status = Command::new(executable)
                .args(std::env::args_os().skip(1))
                .status()
                .map_err(|error| format!("re-exec kernel: {error}"))?;
            status
                .success()
                .then_some(())
                .ok_or_else(|| format!("re-exec kernel exited {status}"))
        }
        #[cfg(windows)]
        {
            Err(windows_upgrade_error())
        }
    }
}

#[cfg(windows)]
fn windows_upgrade_error() -> String {
    "in-place kernel replacement is unavailable on Windows; use `rc upgrade` for verified side-by-side activation".into()
}

impl ArtifactSourceHost for crate::host::HostState {
    fn select(&mut self) -> Result<String, String> {
        let path = std::env::var_os("RC_UPDATER_ARTIFACT_PATH")
            .map(PathBuf::from)
            .ok_or_else(|| "host has not selected an updater artifact".to_owned())?;
        let metadata = fs::metadata(&path).map_err(|error| format!("select artifact: {error}"))?;
        if !metadata.is_file() || metadata.len() > MAX_ARTIFACT as u64 {
            return Err("selected artifact is invalid or too large".into());
        }
        let token = format!("artifact-{}", self.artifact.next_id);
        self.artifact.next_id = self.artifact.next_id.wrapping_add(1);
        self.artifact.selected = Some((token.clone(), path));
        Ok(token)
    }

    fn read(&mut self, handle: String) -> Result<Vec<u8>, String> {
        let Some((selected, path)) = &self.artifact.selected else {
            return Err("no updater artifact is selected".into());
        };
        if selected != &handle {
            return Err("invalid updater artifact handle".into());
        }
        fs::read(path).map_err(|error| format!("read artifact: {error}"))
    }
}

impl NativeReplacement {
    fn target() -> Result<PathBuf, String> {
        std::env::var_os("RC_NATIVE_TARGET")
            .map(PathBuf::from)
            .map_or_else(
                || std::env::current_exe().map_err(|error| format!("resolve kernel: {error}")),
                Ok,
            )
    }
}

pub(crate) fn recover_on_startup() -> Result<(), String> {
    let target = NativeReplacement::target()?;
    let parent = target
        .parent()
        .ok_or_else(|| "kernel has no parent".to_owned())?;
    recovery::recover(parent, &target)
}
