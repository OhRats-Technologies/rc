use crate::{
    bindings::ohrats::rc_process::environment_host::{Host, PlatformKind},
    host::HostState,
};
use std::{collections::BTreeMap, path::PathBuf};

impl Host for HostState {
    fn host_platform(&mut self) -> PlatformKind {
        #[cfg(windows)]
        return PlatformKind::Windows;
        #[cfg(not(windows))]
        PlatformKind::Unix
    }

    fn snapshot(&mut self) -> Result<Vec<(String, String)>, String> {
        self.require_runtime_capability("environment")?;
        std::env::vars_os()
            .map(|(name, value)| {
                Ok((
                    name.into_string()
                        .map_err(|_| "environment name is not Unicode".to_owned())?,
                    value
                        .into_string()
                        .map_err(|_| "environment value is not Unicode".to_owned())?,
                ))
            })
            .collect()
    }

    fn find_executable(
        &mut self,
        program: String,
        environment: Vec<(String, String)>,
    ) -> Result<Option<String>, String> {
        self.require_runtime_capability("environment")?;
        if program.is_empty() || program.contains('\0') {
            return Err("invalid executable name".into());
        }
        let environment = normalized_environment(environment)?;
        let candidate = PathBuf::from(&program);
        if candidate.components().count() > 1 {
            for name in executable_names(&program, &environment) {
                let candidate = PathBuf::from(name);
                if executable(&candidate)? {
                    return path_string(candidate).map(Some);
                }
            }
            return Ok(None);
        }
        let path = environment
            .get("PATH")
            .map(String::as_str)
            .unwrap_or_default();
        for directory in std::env::split_paths(path) {
            for name in executable_names(&program, &environment) {
                let candidate = directory.join(name);
                if executable(&candidate)? {
                    return path_string(candidate).map(Some);
                }
            }
        }
        Ok(None)
    }
}

fn normalized_environment(
    values: Vec<(String, String)>,
) -> Result<BTreeMap<String, String>, String> {
    let mut result = BTreeMap::new();
    for (name, value) in values {
        if name.is_empty() || name.contains(['=', '\0']) || value.contains('\0') {
            return Err("invalid environment entry".into());
        }
        #[cfg(windows)]
        let key = name.to_ascii_uppercase();
        #[cfg(not(windows))]
        let key = name;
        if result.insert(key, value).is_some() {
            return Err("conflicting environment entries".into());
        }
    }
    Ok(result)
}

fn executable_names(program: &str, _environment: &BTreeMap<String, String>) -> Vec<String> {
    #[cfg(windows)]
    {
        if std::path::Path::new(program).extension().is_some() {
            return vec![program.to_owned()];
        }
        let extensions = _environment
            .get("PATHEXT")
            .map(String::as_str)
            .unwrap_or(".COM;.EXE;.BAT;.CMD");
        extensions
            .split(';')
            .filter(|value| !value.is_empty())
            .map(|extension| format!("{program}{extension}"))
            .collect()
    }
    #[cfg(not(windows))]
    vec![program.to_owned()]
}

fn executable(path: &std::path::Path) -> Result<bool, String> {
    let metadata = match std::fs::metadata(path) {
        Ok(value) => value,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(false),
        Err(error) => return Err(error.to_string()),
    };
    if !metadata.is_file() {
        return Ok(false);
    }
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        Ok(metadata.permissions().mode() & 0o111 != 0)
    }
    #[cfg(windows)]
    Ok(true)
}

fn path_string(path: PathBuf) -> Result<String, String> {
    path.into_os_string()
        .into_string()
        .map_err(|_| "executable path is not Unicode".into())
}

#[cfg(all(test, windows))]
mod windows_tests {
    use super::*;

    #[test]
    fn pathext_applies_to_extensionless_explicit_paths() {
        let environment = BTreeMap::from([("PATHEXT".into(), ".EXE;.CMD".into())]);
        assert_eq!(
            executable_names(r"C:\Tools\runner", &environment),
            [r"C:\Tools\runner.EXE", r"C:\Tools\runner.CMD"]
        );
        assert_eq!(
            executable_names(r"C:\Tools\runner.exe", &environment),
            [r"C:\Tools\runner.exe"]
        );
    }
}
