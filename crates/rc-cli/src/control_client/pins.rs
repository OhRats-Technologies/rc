use anyhow::{Context, Result, bail};
use rc_api_client::Device;
use serde::{Deserialize, Serialize};
use std::{collections::BTreeMap, fs, io, path::Path};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct DevicePin {
    identity_key: String,
    transport_key: String,
}

pub(super) fn verify_device_pin(dir: &Path, device: &Device) -> Result<()> {
    let path = dir.join("device-pins.json");
    let mut pins: BTreeMap<String, DevicePin> = match fs::read(&path) {
        Ok(bytes) => serde_json::from_slice(&bytes).context("read device pins")?,
        Err(error) if error.kind() == io::ErrorKind::NotFound => BTreeMap::new(),
        Err(error) => return Err(error.into()),
    };
    let next = DevicePin {
        identity_key: device.identity_public_key.clone(),
        transport_key: device.transport_public_key.clone(),
    };
    if let Some(current) = pins.get(&device.id) {
        if current != &next {
            bail!(
                "RC Node cryptographic identity changed; re-enroll the device before trusting it again"
            );
        }
        return Ok(());
    }
    pins.insert(device.id.clone(), next);
    save_pins(dir, &path, &pins)?;
    Ok(())
}

fn save_pins(dir: &Path, path: &Path, value: &BTreeMap<String, DevicePin>) -> io::Result<()> {
    fs::create_dir_all(dir)?;
    let bytes = serde_json::to_vec_pretty(value).map_err(io::Error::other)?;
    let temp = path.with_extension(format!("tmp-{}", std::process::id()));
    fs::write(&temp, bytes)?;
    set_mode(&temp, 0o600)?;
    fs::rename(&temp, path)?;
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
