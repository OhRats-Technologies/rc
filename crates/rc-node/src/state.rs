use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::SigningKey;
use rand::RngCore;
use serde::{Deserialize, Serialize, de::DeserializeOwned};
use std::{
    fs, io,
    path::{Path, PathBuf},
};
use x25519_dalek::{PublicKey, StaticSecret};

pub const STATE_VERSION: u8 = 1;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct NodeState {
    pub v: u8,
    pub device_id: String,
    pub identity_seed: String,
    pub transport_secret: String,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct NodeConfig {
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub server: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub name: String,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AccountSession {
    pub server: String,
    pub client_id: String,
    pub signing_seed: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub user: String,
}

impl NodeState {
    pub fn generate(device_id: String) -> Self {
        let identity = SigningKey::generate(&mut rand::rngs::OsRng);
        let mut transport = [0_u8; 32];
        rand::rngs::OsRng.fill_bytes(&mut transport);
        Self {
            v: STATE_VERSION,
            device_id,
            identity_seed: URL_SAFE_NO_PAD.encode(identity.to_bytes()),
            transport_secret: URL_SAFE_NO_PAD.encode(transport),
        }
    }

    pub fn identity_public_key(&self) -> io::Result<String> {
        let seed: [u8; 32] = decode32(&self.identity_seed)?;
        Ok(URL_SAFE_NO_PAD.encode(SigningKey::from_bytes(&seed).verifying_key().as_bytes()))
    }

    pub fn transport_public_key(&self) -> io::Result<String> {
        let secret = StaticSecret::from(decode32(&self.transport_secret)?);
        Ok(URL_SAFE_NO_PAD.encode(PublicKey::from(&secret).as_bytes()))
    }

    pub fn validate(&self) -> io::Result<()> {
        if self.v != STATE_VERSION || self.device_id.is_empty() {
            return Err(io::Error::new(
                io::ErrorKind::InvalidData,
                "invalid RC Node state",
            ));
        }
        let _ = self.identity_public_key()?;
        let _ = self.transport_public_key()?;
        Ok(())
    }
}

pub fn resolve_state_dir(explicit: Option<&str>) -> PathBuf {
    if let Some(value) = explicit.filter(|value| !value.trim().is_empty()) {
        return PathBuf::from(value);
    }
    if let Ok(value) = std::env::var("RC_STATE_DIR")
        && !value.trim().is_empty()
    {
        return PathBuf::from(value);
    }
    std::env::var_os("HOME")
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from("."))
        .join(".config/rc")
}

pub fn state_path(dir: &Path) -> PathBuf {
    dir.join("device.json")
}
pub fn config_path(dir: &Path) -> PathBuf {
    dir.join("config.json")
}
pub fn account_path(dir: &Path) -> PathBuf {
    dir.join("account.json")
}
pub fn lock_path(dir: &Path) -> PathBuf {
    dir.join("lock.json")
}

pub fn load_state(dir: &Path) -> io::Result<NodeState> {
    let value: NodeState = load_json(&state_path(dir))?;
    value.validate()?;
    Ok(value)
}
pub fn save_state(dir: &Path, value: &NodeState) -> io::Result<()> {
    value.validate()?;
    save_json(dir, &state_path(dir), value)
}
pub fn load_config(dir: &Path) -> io::Result<NodeConfig> {
    load_optional_json(&config_path(dir))
}
pub fn save_config(dir: &Path, value: &NodeConfig) -> io::Result<()> {
    save_json(dir, &config_path(dir), value)
}
pub fn load_account(dir: &Path) -> io::Result<AccountSession> {
    load_json(&account_path(dir))
}
pub fn save_account(dir: &Path, value: &AccountSession) -> io::Result<()> {
    save_json(dir, &account_path(dir), value)
}

fn decode32(value: &str) -> io::Result<[u8; 32]> {
    URL_SAFE_NO_PAD
        .decode(value)
        .map_err(io::Error::other)?
        .try_into()
        .map_err(|_| io::Error::new(io::ErrorKind::InvalidData, "invalid key length"))
}
fn load_optional_json<T: DeserializeOwned + Default>(path: &Path) -> io::Result<T> {
    match load_json(path) {
        Ok(value) => Ok(value),
        Err(error) if error.kind() == io::ErrorKind::NotFound => Ok(T::default()),
        Err(error) => Err(error),
    }
}
fn load_json<T: DeserializeOwned>(path: &Path) -> io::Result<T> {
    let bytes = fs::read(path)?;
    serde_json::from_slice(&bytes)
        .map_err(|error| io::Error::new(io::ErrorKind::InvalidData, error))
}
fn save_json<T: Serialize>(dir: &Path, path: &Path, value: &T) -> io::Result<()> {
    fs::create_dir_all(dir)?;
    set_mode(dir, 0o700)?;
    let mut bytes = serde_json::to_vec_pretty(value).map_err(io::Error::other)?;
    bytes.push(b'\n');
    let temporary = path.with_extension(format!("tmp-{}", std::process::id()));
    fs::write(&temporary, bytes)?;
    set_mode(&temporary, 0o600)?;
    fs::rename(&temporary, path)?;
    set_mode(path, 0o600)
}
#[cfg(unix)]
fn set_mode(path: &Path, mode: u32) -> io::Result<()> {
    use std::os::unix::fs::PermissionsExt;
    fs::set_permissions(path, fs::Permissions::from_mode(mode))
}
#[cfg(not(unix))]
fn set_mode(_: &Path, _: u32) -> io::Result<()> {
    Ok(())
}
