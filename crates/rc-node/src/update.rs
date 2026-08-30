mod bundle;

use serde::Deserialize;
use sha2::{Digest, Sha256};
use std::{io, process::Command};

const DEFAULT_RELEASE_API: &str =
    "https://api.github.com/repos/OhRats-Technologies/rc/releases/latest";
const MAX_ARCHIVE: usize = 100 << 20;

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

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct UpgradeManifest {
    schema: u32,
    minimum_upgrader: String,
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
    let ordering = compare_versions(version, current_version)?;
    match ordering {
        std::cmp::Ordering::Less => {
            anyhow::bail!("refusing downgrade from {current_version} to {version}")
        }
        std::cmp::Ordering::Equal if bundle::runtime_complete() => return Ok(false),
        std::cmp::Ordering::Equal => {}
        std::cmp::Ordering::Greater => {}
    }
    enforce_upgrade_manifest(&client, &release, current_version).await?;
    let platform = format!("{}-{}", release_os(), release_arch());
    let kernel = asset_bytes(&client, &release, &format!("rc-kernel-{platform}.tar.gz")).await?;
    let core = asset_bytes(&client, &release, core_asset_name(&release)).await?;
    let rc = if ordering == std::cmp::Ordering::Greater {
        Some(asset_bytes(&client, &release, &format!("rc-{platform}.tar.gz")).await?)
    } else {
        None
    };
    bundle::install(rc.as_deref(), &kernel, &core, version).map_err(|error| {
        anyhow::anyhow!(
            "RC platform upgrade failed: {error:#}. Reinstall from the verified release installer to repair the native runtime and core profile together"
        )
    })?;
    Ok(true)
}

async fn enforce_upgrade_manifest(
    client: &reqwest::Client,
    release: &GithubRelease,
    current_version: &str,
) -> anyhow::Result<()> {
    if !release
        .assets
        .iter()
        .any(|asset| asset.name == "rc-upgrade.json")
    {
        return Ok(());
    }
    let bytes = asset_bytes(client, release, "rc-upgrade.json").await?;
    let manifest: UpgradeManifest = serde_json::from_slice(&bytes)?;
    anyhow::ensure!(manifest.schema == 1, "unsupported RC upgrade manifest");
    if compare_versions(current_version, &manifest.minimum_upgrader)?.is_lt() {
        anyhow::bail!(
            "this RC release requires upgrader {} or newer; use the verified install.sh/install.ps1 release installer once to migrate the complete native runtime",
            manifest.minimum_upgrader
        );
    }
    Ok(())
}

fn core_asset_name(release: &GithubRelease) -> &'static str {
    if release
        .assets
        .iter()
        .any(|asset| asset.name == "rc-core-profile.tar.gz")
    {
        "rc-core-profile.tar.gz"
    } else {
        "rc-core-components.tar.gz"
    }
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
        let replacement = rc_platform::active_runtime_dir()
            .map(|directory| directory.join(rc_platform::executable_name("rc-kernel")))
            .filter(|path| path.is_file())
            .unwrap_or(executable);
        Command::new(replacement)
            .args(std::env::args_os().skip(1))
            .envs(std::env::vars_os())
            .spawn()?;
        std::process::exit(0)
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

async fn asset_bytes(
    client: &reqwest::Client,
    release: &GithubRelease,
    name: &str,
) -> anyhow::Result<Vec<u8>> {
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
        .ok_or_else(|| {
            anyhow::anyhow!("release asset {name} is missing its GitHub SHA-256 digest")
        })?;
    let bytes = download(client, &asset.browser_download_url, MAX_ARCHIVE).await?;
    anyhow::ensure!(
        hex_lower(&Sha256::digest(&bytes)).eq_ignore_ascii_case(digest),
        "release hash mismatch for {name}"
    );
    Ok(bytes)
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
    use super::{GithubAsset, GithubRelease, compare_versions, core_asset_name};
    use std::cmp::Ordering;

    #[test]
    fn core_asset_prefers_current_profile() {
        let release = GithubRelease {
            tag_name: "v1.0.0".into(),
            assets: vec![GithubAsset {
                name: "rc-core-profile.tar.gz".into(),
                browser_download_url: String::new(),
                digest: None,
            }],
        };
        assert_eq!(core_asset_name(&release), "rc-core-profile.tar.gz");
        let legacy = GithubRelease {
            tag_name: "v0.19.2".into(),
            assets: Vec::new(),
        };
        assert_eq!(core_asset_name(&legacy), "rc-core-components.tar.gz");
    }

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
}
