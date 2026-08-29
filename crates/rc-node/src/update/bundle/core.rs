use super::{CORE_COMPONENTS, MAX_COMPONENT};
use flate2::read::GzDecoder;
use sha2::{Digest, Sha256};
use std::{collections::BTreeMap, io::Read, path::Path};

pub(super) const LEGACY_CORE_COMPONENTS: &[&str] = &[
    "diagnostics-cli",
    "diagnostics-reporter",
    "diagnostics-store",
    "github-source",
    "http-source",
    "local-source",
    "oci-source",
    "package-manager",
    "process-policy",
    "transport-webrtc",
];
const MAX_PROFILE_LOCK: u64 = 64 << 10;

pub(super) fn extract_core(archive: &[u8]) -> anyhow::Result<BTreeMap<String, Vec<u8>>> {
    let mut tar = tar::Archive::new(GzDecoder::new(archive));
    let mut values = BTreeMap::new();
    let mut profile_lock = None;
    for entry in tar.entries()? {
        let entry = entry?;
        let path = entry.path()?.into_owned();
        if entry.header().entry_type().is_dir() && path.as_path() == Path::new("components") {
            continue;
        }
        anyhow::ensure!(
            entry.header().entry_type().is_file(),
            "invalid core component archive"
        );
        if path.as_path() == Path::new("profile.lock") {
            anyhow::ensure!(profile_lock.is_none(), "duplicate core profile lock");
            anyhow::ensure!(
                entry.size() <= MAX_PROFILE_LOCK,
                "core profile lock is too large"
            );
            let mut bytes = Vec::with_capacity(entry.size() as usize);
            entry.take(MAX_PROFILE_LOCK + 1).read_to_end(&mut bytes)?;
            anyhow::ensure!(
                bytes.len() as u64 <= MAX_PROFILE_LOCK,
                "core profile lock is too large"
            );
            profile_lock = Some(String::from_utf8(bytes)?);
            continue;
        }
        let name = component_name(&path)?;
        anyhow::ensure!(
            CORE_COMPONENTS.contains(&name.as_str()),
            "unexpected core component {name:?}"
        );
        anyhow::ensure!(
            !values.contains_key(&name),
            "duplicate core component {name:?}"
        );
        anyhow::ensure!(entry.size() <= MAX_COMPONENT, "core component is too large");
        let mut bytes = Vec::with_capacity(entry.size() as usize);
        entry.take(MAX_COMPONENT + 1).read_to_end(&mut bytes)?;
        anyhow::ensure!(
            bytes.len() as u64 <= MAX_COMPONENT,
            "core component is too large"
        );
        values.insert(name, bytes);
    }
    match profile_lock {
        Some(lock) => validate_profile_lock(&lock, &values)?,
        None => validate_legacy(&values)?,
    }
    Ok(values)
}

fn component_name(path: &Path) -> anyhow::Result<String> {
    path.to_str()
        .and_then(|value| value.strip_prefix("components/"))
        .and_then(|value| value.strip_suffix(".wasm"))
        .filter(|value| !value.contains('/'))
        .map(str::to_owned)
        .ok_or_else(|| anyhow::anyhow!("unexpected core component path {}", path.display()))
}

fn validate_legacy(values: &BTreeMap<String, Vec<u8>>) -> anyhow::Result<()> {
    anyhow::ensure!(
        values.len() == LEGACY_CORE_COMPONENTS.len()
            && LEGACY_CORE_COMPONENTS
                .iter()
                .all(|name| values.contains_key(*name)),
        "legacy core component archive is incomplete"
    );
    Ok(())
}

fn validate_profile_lock(lock: &str, values: &BTreeMap<String, Vec<u8>>) -> anyhow::Result<()> {
    let mut lines = lock.lines();
    anyhow::ensure!(
        lines.next() == Some("schema 1"),
        "invalid core profile lock schema"
    );
    anyhow::ensure!(
        lines.next() == Some("profile ohrats:core"),
        "invalid core profile id"
    );
    let mut seen = BTreeMap::new();
    for line in lines {
        let fields = line.split_ascii_whitespace().collect::<Vec<_>>();
        anyhow::ensure!(
            fields.len() == 3 && fields[0] == "component",
            "invalid core profile entry"
        );
        let name = fields[1];
        anyhow::ensure!(
            CORE_COMPONENTS.contains(&name),
            "unexpected core profile component {name:?}"
        );
        let expected = fields[2]
            .strip_prefix("sha256:")
            .filter(|value| value.len() == 64 && value.bytes().all(|byte| byte.is_ascii_hexdigit()))
            .ok_or_else(|| anyhow::anyhow!("invalid core profile digest for {name}"))?;
        anyhow::ensure!(
            seen.insert(name, ()).is_none(),
            "duplicate core profile component {name:?}"
        );
        let bytes = values
            .get(name)
            .ok_or_else(|| anyhow::anyhow!("core profile is missing {name}"))?;
        let actual = format!("{:x}", Sha256::digest(bytes));
        anyhow::ensure!(
            actual.eq_ignore_ascii_case(expected),
            "core component digest mismatch: {name}"
        );
    }
    anyhow::ensure!(
        seen.len() == CORE_COMPONENTS.len()
            && CORE_COMPONENTS.iter().all(|name| seen.contains_key(name)),
        "core profile is incomplete"
    );
    anyhow::ensure!(
        values.len() == CORE_COMPONENTS.len(),
        "core component archive has extra members"
    );
    Ok(())
}
