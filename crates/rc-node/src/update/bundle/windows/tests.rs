use super::*;
use std::{ffi::OsString, sync::Mutex};

static ENVIRONMENT: Mutex<()> = Mutex::new(());

struct Environment(Vec<(&'static str, Option<OsString>)>);

impl Environment {
    fn set(root: &Path) -> Self {
        let values = [
            ("RC_DATA_DIR", root.join("data")),
            ("RC_COMPONENT_DIR", root.join("components")),
            ("RC_INSTALL_BIN_DIR", root.join("bin")),
        ];
        let old = values
            .iter()
            .map(|(name, _)| (*name, std::env::var_os(name)))
            .collect();
        for (name, value) in values {
            unsafe { std::env::set_var(name, value) };
        }
        Self(old)
    }
}

impl Drop for Environment {
    fn drop(&mut self) {
        for (name, value) in self.0.drain(..) {
            match value {
                Some(value) => unsafe { std::env::set_var(name, value) },
                None => unsafe { std::env::remove_var(name) },
            }
        }
    }
}

#[test]
fn interrupted_activation_restores_cli_components_and_pointer() {
    let _lock = ENVIRONMENT.lock().unwrap();
    let root = std::env::temp_dir().join(format!(
        "rc-windows-activation-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos()
    ));
    std::fs::create_dir(&root).unwrap();
    let environment = Environment::set(&root);
    let backup = rollback_dir().unwrap();
    fs::create_dir_all(backup.join("components")).unwrap();
    fs::create_dir_all(rc_platform::binary_dir().unwrap()).unwrap();
    fs::create_dir_all(rc_platform::component_dir().unwrap()).unwrap();
    fs::write(backup.join("rc.exe"), b"old-cli").unwrap();
    fs::write(backup.join("components/shell.wasm"), b"old-shell").unwrap();
    fs::write(
        rc_platform::binary_dir().unwrap().join("rc.exe"),
        b"new-cli",
    )
    .unwrap();
    fs::write(
        rc_platform::component_dir().unwrap().join("shell.wasm"),
        b"new-shell",
    )
    .unwrap();
    fs::write(
        rc_platform::component_dir().unwrap().join("shell.core"),
        b"new-marker",
    )
    .unwrap();
    atomic_write(
        &rc_platform::runtime_activation_file().unwrap(),
        b"C:\\new\n",
    )
    .unwrap();
    let journal = ActivationJournal {
        previous: Some("C:\\old".into()),
        names: vec!["shell".into()],
    };
    atomic_write(
        &journal_path().unwrap(),
        &serde_json::to_vec(&journal).unwrap(),
    )
    .unwrap();

    recover_interrupted().unwrap();

    assert_eq!(
        fs::read(rc_platform::binary_dir().unwrap().join("rc.exe")).unwrap(),
        b"old-cli"
    );
    assert_eq!(
        fs::read(rc_platform::component_dir().unwrap().join("shell.wasm")).unwrap(),
        b"old-shell"
    );
    assert!(
        !rc_platform::component_dir()
            .unwrap()
            .join("shell.core")
            .exists()
    );
    assert_eq!(
        fs::read_to_string(rc_platform::runtime_activation_file().unwrap()).unwrap(),
        "C:\\old\n"
    );
    assert!(!journal_path().unwrap().exists());
    drop(environment);
    std::fs::remove_dir_all(root).unwrap();
}
