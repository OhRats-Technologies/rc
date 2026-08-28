use crate::{ohrats::rc_ssh::types::PublicKey, store};
use base64::{
    Engine as _,
    engine::general_purpose::{STANDARD, STANDARD_NO_PAD},
};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

fn trace(message: &str) {
    crate::ohrats::rc_plugin::host::log(crate::ohrats::rc_plugin::types::LogLevel::Trace, message);
}

const MAX_KEYS: usize = 20;
#[derive(Clone, Deserialize, Serialize)]
pub struct StoredKey {
    pub id: String,
    pub user_id: String,
    pub control_client_id: String,
    pub name: String,
    pub algorithm: String,
    pub key_data: String,
    pub normalized: String,
    pub fingerprint: String,
    pub created_at_ms: u64,
    pub last_used_at_ms: Option<u64>,
}

pub fn register(
    user_id: String,
    client_id: String,
    name: String,
    input: String,
    created: u64,
) -> Result<PublicKey, String> {
    trace("SSH key registration started");
    validate_id(&user_id, "user")?;
    validate_id(&client_id, "control client")?;
    if list(&user_id)?.len() >= MAX_KEYS {
        return Err("SSH key limit reached".into());
    }
    let (algorithm, key_data, normalized, fingerprint) = normalize(&input)?;
    trace("SSH key normalized");
    let stored = StoredKey {
        id: fingerprint_id(&fingerprint),
        user_id,
        control_client_id: client_id,
        name: clean_name(name),
        algorithm,
        key_data,
        normalized,
        fingerprint,
        created_at_ms: created,
        last_used_at_ms: None,
    };
    store::insert(
        stored.id.as_bytes().to_vec(),
        serde_json::to_vec(&stored).map_err(|_| "SSH key serialization failed")?,
        &stored.fingerprint,
    )?;
    trace("SSH key registration committed");
    Ok(stored.into())
}
pub fn get(id: &str) -> Result<Option<PublicKey>, String> {
    Ok(load(id)?.map(Into::into))
}
pub fn list(user: &str) -> Result<Vec<PublicKey>, String> {
    let mut keys = store::scan()?
        .into_iter()
        .map(|e| {
            serde_json::from_slice::<StoredKey>(&e.value)
                .map_err(|_| "invalid stored SSH key".into())
        })
        .collect::<Result<Vec<_>, String>>()?;
    keys.retain(|key| key.user_id == user);
    keys.sort_by_key(|key| std::cmp::Reverse(key.created_at_ms));
    Ok(keys.into_iter().map(Into::into).collect())
}
pub fn revoke(id: &str, user: &str) -> Result<bool, String> {
    match load(id)? {
        Some(key) if key.user_id == user => store::remove(id.as_bytes()),
        _ => Ok(false),
    }
}
pub fn find(algorithm: &str, data: &str) -> Result<Option<StoredKey>, String> {
    Ok(store::scan()?
        .into_iter()
        .filter_map(|e| serde_json::from_slice::<StoredKey>(&e.value).ok())
        .find(|k| k.algorithm == algorithm && k.key_data == data))
}
fn load(id: &str) -> Result<Option<StoredKey>, String> {
    store::get(id.as_bytes())?
        .map(|v| serde_json::from_slice(&v).map_err(|_| "invalid stored SSH key".into()))
        .transpose()
}

fn normalize(input: &str) -> Result<(String, String, String, String), String> {
    let value = input.trim();
    if value.is_empty() || value.len() > 16384 || value.contains("PRIVATE KEY") {
        return Err("invalid SSH public key".into());
    }
    let mut parts = value.split_whitespace();
    let algorithm = parts.next().unwrap_or("");
    let data = parts.next().unwrap_or("");
    if !matches!(algorithm, "ssh-ed25519" | "ssh-rsa") {
        return Err("unsupported SSH public key algorithm".into());
    }
    let bytes = STANDARD
        .decode(data)
        .map_err(|_| "invalid SSH public key")?;
    let embedded = wire_string(&bytes).ok_or("invalid SSH public key")?;
    if embedded != algorithm.as_bytes() {
        return Err("SSH public key algorithm mismatch".into());
    }
    validate_wire(algorithm, &bytes)?;
    let fingerprint = format!("SHA256:{}", STANDARD_NO_PAD.encode(Sha256::digest(&bytes)));
    Ok((
        algorithm.into(),
        data.into(),
        format!("{algorithm} {data}"),
        fingerprint,
    ))
}
fn validate_wire(algorithm: &str, bytes: &[u8]) -> Result<(), String> {
    let mut at = 4 + algorithm.len();
    if algorithm == "ssh-ed25519" {
        let key = take(bytes, &mut at).ok_or("invalid SSH public key")?;
        if key.len() != 32 || at != bytes.len() {
            return Err("invalid SSH public key".into());
        }
    } else {
        let exponent = take(bytes, &mut at).ok_or("invalid SSH public key")?;
        let modulus = take(bytes, &mut at).ok_or("invalid SSH public key")?;
        if exponent.is_empty() || modulus.len() < 128 || at != bytes.len() {
            return Err("invalid SSH public key".into());
        }
    }
    Ok(())
}
fn wire_string(bytes: &[u8]) -> Option<&[u8]> {
    let mut at = 0;
    take(bytes, &mut at)
}
fn take<'a>(bytes: &'a [u8], at: &mut usize) -> Option<&'a [u8]> {
    let len = u32::from_be_bytes(bytes.get(*at..*at + 4)?.try_into().ok()?) as usize;
    *at += 4;
    let out = bytes.get(*at..*at + len)?;
    *at += len;
    Some(out)
}
fn validate_id(value: &str, kind: &str) -> Result<(), String> {
    if value.trim().is_empty() || value.len() > 128 {
        Err(format!("invalid {kind} id"))
    } else {
        Ok(())
    }
}
fn clean_name(value: String) -> String {
    let value: String = value.trim().chars().take(80).collect();
    if value.is_empty() {
        "SSH key".into()
    } else {
        value
    }
}
fn fingerprint_id(fingerprint: &str) -> String {
    Sha256::digest(fingerprint.as_bytes())[..16]
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}
impl From<StoredKey> for PublicKey {
    fn from(k: StoredKey) -> Self {
        Self {
            id: k.id,
            user_id: k.user_id,
            control_client_id: k.control_client_id,
            name: k.name,
            algorithm: k.algorithm,
            normalized: k.normalized,
            fingerprint: k.fingerprint,
            created_at_ms: k.created_at_ms,
            last_used_at_ms: k.last_used_at_ms,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::normalize;
    use base64::{Engine as _, engine::general_purpose::STANDARD};

    fn field(value: &[u8]) -> Vec<u8> {
        let mut out = (value.len() as u32).to_be_bytes().to_vec();
        out.extend_from_slice(value);
        out
    }

    #[test]
    fn accepts_ed25519_and_rsa_wire_keys() {
        let mut ed = field(b"ssh-ed25519");
        ed.extend(field(&[7; 32]));
        assert_eq!(
            normalize(&format!("ssh-ed25519 {} comment", STANDARD.encode(ed)))
                .unwrap()
                .0,
            "ssh-ed25519"
        );
        let mut rsa = field(b"ssh-rsa");
        rsa.extend(field(&[1, 0, 1]));
        rsa.extend(field(&[9; 256]));
        assert_eq!(
            normalize(&format!("ssh-rsa {}", STANDARD.encode(rsa)))
                .unwrap()
                .0,
            "ssh-rsa"
        );
    }

    #[test]
    fn rejects_mismatched_embedded_algorithm() {
        let mut ed = field(b"ssh-ed25519");
        ed.extend(field(&[7; 32]));
        assert!(normalize(&format!("ssh-rsa {}", STANDARD.encode(ed))).is_err());
    }
}
