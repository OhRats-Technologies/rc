use crate::{
    exports::ohrats::rc_plugin::package_source::PackageArtifact,
    ohrats::rc_plugin::http_client::{self, Header, Request, Response},
    spec::GithubSpec,
};
use serde::Deserialize;
use sha2::{Digest, Sha256};

#[derive(Deserialize)]
struct Release {
    tag_name: String,
    assets: Vec<Asset>,
}

#[derive(Deserialize)]
struct Asset {
    name: String,
    browser_download_url: String,
    #[serde(default)]
    digest: Option<String>,
}

pub fn raw_component(value: &GithubSpec) -> Result<PackageArtifact, String> {
    let revision = value.revision.as_deref().unwrap_or("main");
    let url = format!(
        "https://raw.githubusercontent.com/{}/{}/{revision}/{}",
        value.owner, value.repository, value.path
    );
    let response = get(
        &url,
        "application/wasm, application/octet-stream",
        48 * 1024 * 1024,
    )?;
    artifact(
        value.package_name()?,
        value.display(revision),
        response,
        None,
    )
}

pub fn release_component(value: &GithubSpec) -> Result<PackageArtifact, String> {
    let endpoint = value.revision.as_ref().map_or_else(
        || {
            format!(
                "https://api.github.com/repos/{}/{}/releases/latest",
                value.owner, value.repository
            )
        },
        |revision| {
            format!(
                "https://api.github.com/repos/{}/{}/releases/tags/{revision}",
                value.owner, value.repository
            )
        },
    );
    let response = get(&endpoint, "application/vnd.github+json", 2 * 1024 * 1024)?;
    let release: Release = serde_json::from_slice(&response.body)
        .map_err(|error| format!("invalid GitHub release response: {error}"))?;
    let package = value.package_name()?;
    let names = [
        format!("{package}.wasm"),
        format!("rc-component-{package}.wasm"),
    ];
    let asset = release
        .assets
        .into_iter()
        .find(|asset| names.contains(&asset.name))
        .ok_or_else(|| {
            format!(
                "GitHub release {} has no {package} component",
                release.tag_name
            )
        })?;
    let artifact_response = get(
        &asset.browser_download_url,
        "application/wasm, application/octet-stream",
        48 * 1024 * 1024,
    )?;
    artifact(
        package,
        value.display(&release.tag_name),
        artifact_response,
        asset.digest.as_deref(),
    )
}

fn get(url: &str, accept: &str, maximum: u64) -> Result<Response, String> {
    let response = http_client::send(&Request {
        method: "GET".into(),
        url: url.into(),
        headers: vec![Header {
            name: "accept".into(),
            value: accept.into(),
        }],
        body: Vec::new(),
        max_response_bytes: maximum,
    })?;
    if (200..300).contains(&response.status) {
        Ok(response)
    } else {
        Err(format!(
            "GitHub returned status {} for {url}",
            response.status
        ))
    }
}

fn artifact(
    name: String,
    source: String,
    response: Response,
    expected: Option<&str>,
) -> Result<PackageArtifact, String> {
    let digest = format!("sha256:{:x}", Sha256::digest(&response.body));
    if let Some(expected) = expected.filter(|value| value.starts_with("sha256:"))
        && !digest.eq_ignore_ascii_case(expected)
    {
        return Err("GitHub release asset digest mismatch".into());
    }
    Ok(PackageArtifact {
        name,
        source,
        digest,
        bytes: response.body,
    })
}
