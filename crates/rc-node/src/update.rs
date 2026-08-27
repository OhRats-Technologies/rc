use flate2::read::GzDecoder;
use serde::Deserialize;
use sha2::{Digest, Sha256};
use std::{
    fs::{self, File},
    io::{self, Read, Write},
    path::Path,
    process::Command,
};

const DEFAULT_RELEASE_API: &str =
    "https://api.github.com/repos/OhRats-Technologies/rc/releases/latest";
const MAX_ARCHIVE: usize = 100 << 20;
const MAX_BINARY: u64 = 100 << 20;

#[derive(Debug, Clone, Deserialize)]
struct GithubRelease {
    tag_name: String,
    assets: Vec<GithubAsset>,
}

#[derive(Debug, Clone, Deserialize)]
struct GithubAsset {
    name: String,
    browser_download_url: String,
    digest: Option<String>,
}

pub async fn replace_executable(current_version: &str) -> anyhow::Result<bool> {
    let api = std::env::var("RC_RELEASE_API")
        .ok()
        .filter(|value| !value.trim().is_empty())
        .unwrap_or_else(|| DEFAULT_RELEASE_API.into());
    replace_executable_from(current_version, &api).await
}

pub async fn replace_executable_from(
    current_version: &str,
    release_api: &str,
) -> anyhow::Result<bool> {
    let client = reqwest::Client::builder()
        .user_agent("rc-updater")
        .build()?;
    let release: GithubRelease = client
        .get(release_api)
        .send()
        .await?
        .error_for_status()?
        .json()
        .await?;
    let version = release.tag_name.trim_start_matches('v');
    match compare_versions(version, current_version)? {
        std::cmp::Ordering::Less => {
            anyhow::bail!("refusing downgrade from {current_version} to {version}")
        }
        std::cmp::Ordering::Equal => return Ok(false),
        std::cmp::Ordering::Greater => {}
    }
    let name = format!("rc-{}-{}.tar.gz", release_os(), release_arch());
    let asset = release
        .assets
        .iter()
        .find(|asset| asset.name == name)
        .ok_or_else(|| anyhow::anyhow!("release does not contain {name}"))?;
    let digest = asset
        .digest
        .as_deref()
        .and_then(|value| value.strip_prefix("sha256:"))
        .filter(|value| value.len() == 64 && value.chars().all(|c| c.is_ascii_hexdigit()))
        .ok_or_else(|| anyhow::anyhow!("release asset is missing its GitHub SHA-256 digest"))?;
    let archive = download(&client, &asset.browser_download_url, MAX_ARCHIVE).await?;
    if !hex_lower(&Sha256::digest(&archive)).eq_ignore_ascii_case(digest) {
        anyhow::bail!("release hash mismatch");
    }
    install_archive(&archive, version)
}

pub fn exec_current() -> io::Result<()> {
    let executable = std::env::current_exe()?;
    #[cfg(unix)]
    {
        use std::os::unix::process::CommandExt;
        Err(Command::new(executable)
            .args(std::env::args_os().skip(1))
            .envs(std::env::vars_os())
            .exec())
    }
    #[cfg(not(unix))]
    {
        let _ = executable;
        Err(io::Error::new(
            io::ErrorKind::Unsupported,
            "in-place restart is unsupported",
        ))
    }
}

async fn download(client: &reqwest::Client, url: &str, limit: usize) -> anyhow::Result<Vec<u8>> {
    let response = client.get(url).send().await?.error_for_status()?;
    if response
        .content_length()
        .is_some_and(|length| length as usize > limit)
    {
        anyhow::bail!("release artifact is too large");
    }
    let bytes = response.bytes().await?;
    if bytes.len() > limit {
        anyhow::bail!("release artifact is too large");
    }
    Ok(bytes.to_vec())
}

fn install_archive(archive: &[u8], version: &str) -> anyhow::Result<bool> {
    let executable = std::env::current_exe()?;
    let replacement = executable
        .parent()
        .ok_or_else(|| anyhow::anyhow!("invalid executable path"))?
        .join(format!(".rc-update-{}", std::process::id()));
    extract_rc(archive, &replacement)?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&replacement, fs::Permissions::from_mode(0o755))?;
    }
    let output = Command::new(&replacement).arg("version").output()?;
    if !output.status.success()
        || String::from_utf8_lossy(&output.stdout).trim() != format!("RC {version}")
    {
        let _ = fs::remove_file(&replacement);
        anyhow::bail!("downloaded executable does not match release version");
    }
    match fs::rename(&replacement, &executable) {
        Ok(()) => Ok(true),
        Err(error) => {
            let _ = fs::remove_file(&replacement);
            Err(error.into())
        }
    }
}

fn extract_rc(archive: &[u8], destination: &Path) -> anyhow::Result<()> {
    let mut tar = tar::Archive::new(GzDecoder::new(archive));
    let mut binary = None;
    for entry in tar.entries()? {
        let entry = entry?;
        if entry.path()?.as_ref() != Path::new("rc") || binary.is_some() {
            anyhow::bail!("release archive must contain only rc");
        }
        if !entry.header().entry_type().is_file() || entry.size() > MAX_BINARY {
            anyhow::bail!("invalid rc release archive");
        }
        let mut bytes = Vec::with_capacity(entry.size() as usize);
        entry.take(MAX_BINARY + 1).read_to_end(&mut bytes)?;
        if bytes.len() as u64 > MAX_BINARY {
            anyhow::bail!("release executable is too large");
        }
        binary = Some(bytes);
    }
    let binary = binary.ok_or_else(|| anyhow::anyhow!("release archive does not contain rc"))?;
    let mut file = File::create(destination)?;
    file.write_all(&binary)?;
    file.flush()?;
    file.sync_all()?;
    Ok(())
}

fn compare_versions(left: &str, right: &str) -> anyhow::Result<std::cmp::Ordering> {
    Ok(parse_version(left)?.cmp(&parse_version(right)?))
}

fn parse_version(value: &str) -> anyhow::Result<semver::Version> {
    Ok(semver::Version::parse(
        value.trim().trim_start_matches('v'),
    )?)
}

fn release_os() -> &'static str {
    match std::env::consts::OS {
        "macos" => "darwin",
        value => value,
    }
}

fn release_arch() -> &'static str {
    match std::env::consts::ARCH {
        "x86_64" => "amd64",
        "aarch64" => "arm64",
        value => value,
    }
}

fn hex_lower(bytes: &[u8]) -> String {
    bytes.iter().map(|byte| format!("{byte:02x}")).collect()
}

#[cfg(test)]
mod tests {
    use super::{compare_versions, extract_rc};
    use flate2::{Compression, write::GzEncoder};
    use std::cmp::Ordering;
    use tar::{Builder, Header};

    #[test]
    fn version_ordering_includes_prereleases() -> anyhow::Result<()> {
        assert_eq!(
            compare_versions("0.16.0-alpha.2", "0.16.0-alpha.1")?,
            Ordering::Greater
        );
        assert_eq!(
            compare_versions("0.16.0", "0.16.0-alpha.2")?,
            Ordering::Greater
        );
        assert_eq!(compare_versions("v0.16.0", "0.16.0")?, Ordering::Equal);
        assert_eq!(
            compare_versions("0.15.11", "0.16.0-alpha.1")?,
            Ordering::Less
        );
        Ok(())
    }

    #[test]
    fn release_archive_rejects_extra_or_duplicate_entries() -> anyhow::Result<()> {
        let destination = std::env::temp_dir().join(format!(
            "rc-update-test-{}-{}",
            std::process::id(),
            rand::random::<u64>()
        ));
        extract_rc(&archive(&[("rc", b"binary")])?, &destination)?;
        assert_eq!(std::fs::read(&destination)?, b"binary");
        std::fs::remove_file(&destination)?;

        assert!(
            extract_rc(
                &archive(&[("rc", b"binary"), ("extra", b"bad")])?,
                &destination
            )
            .is_err()
        );
        assert!(extract_rc(&archive(&[("rc", b"one"), ("rc", b"two")])?, &destination).is_err());
        Ok(())
    }

    fn archive(entries: &[(&str, &[u8])]) -> anyhow::Result<Vec<u8>> {
        let encoder = GzEncoder::new(Vec::new(), Compression::default());
        let mut builder = Builder::new(encoder);
        for (name, contents) in entries {
            let mut header = Header::new_gnu();
            header.set_size(contents.len() as u64);
            header.set_mode(0o755);
            header.set_cksum();
            builder.append_data(&mut header, *name, *contents)?;
        }
        let encoder = builder.into_inner()?;
        Ok(encoder.finish()?)
    }
}
