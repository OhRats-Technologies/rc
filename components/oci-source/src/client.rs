use crate::{
    exports::ohrats::rc_plugin::package_source::PackageArtifact,
    ohrats::rc_plugin::http_client::{self, Header, Request, Response},
    spec::{OciSpec, validate_digest},
};
use serde::Deserialize;
use sha2::{Digest, Sha256};
use std::collections::BTreeMap;

const MANIFEST_ACCEPT: &str = concat!(
    "application/vnd.oci.image.manifest.v1+json, ",
    "application/vnd.oci.artifact.manifest.v1+json, ",
    "application/vnd.docker.distribution.manifest.v2+json"
);

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Manifest {
    layers: Vec<Layer>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Layer {
    media_type: String,
    digest: String,
    size: u64,
    #[serde(default)]
    annotations: BTreeMap<String, String>,
}

#[derive(Deserialize)]
struct TokenResponse {
    #[serde(default)]
    token: Option<String>,
    #[serde(default)]
    access_token: Option<String>,
}

pub fn pull(value: &OciSpec) -> Result<PackageArtifact, String> {
    let (manifest_response, token) = authenticated_get(
        value,
        &value.manifest_url(),
        MANIFEST_ACCEPT,
        2 * 1024 * 1024,
    )?;
    let manifest_digest = header(&manifest_response, "docker-content-digest")
        .map(str::to_owned)
        .unwrap_or_else(|| digest(&manifest_response.body));
    validate_digest(&manifest_digest)?;
    if value.reference.starts_with("sha256:")
        && !value.reference.eq_ignore_ascii_case(&manifest_digest)
    {
        return Err("OCI manifest digest does not match requested digest".into());
    }
    let manifest: Manifest = serde_json::from_slice(&manifest_response.body)
        .map_err(|error| format!("invalid OCI manifest: {error}"))?;
    let layer = select_layer(&manifest.layers)?;
    validate_digest(&layer.digest)?;
    if layer.size > 48 * 1024 * 1024 {
        return Err("OCI component layer exceeds 48 MiB".into());
    }
    let response = get(
        &value.blob_url(&layer.digest),
        "application/wasm, application/octet-stream",
        48 * 1024 * 1024,
        token.as_deref(),
    )?;
    if !(200..300).contains(&response.status) {
        return Err(format!(
            "OCI registry returned status {} for blob",
            response.status
        ));
    }
    let actual = digest(&response.body);
    if !actual.eq_ignore_ascii_case(&layer.digest) {
        return Err("OCI component layer digest mismatch".into());
    }
    Ok(PackageArtifact {
        name: layer
            .annotations
            .get("org.opencontainers.image.title")
            .and_then(|title| title.strip_suffix(".wasm"))
            .filter(|name| valid_name(name))
            .map(str::to_owned)
            .unwrap_or_else(|| value.package_name()),
        source: value.source(&manifest_digest),
        digest: actual,
        bytes: response.body,
    })
}

fn authenticated_get(
    value: &OciSpec,
    url: &str,
    accept: &str,
    maximum: u64,
) -> Result<(Response, Option<String>), String> {
    let first = get(url, accept, maximum, None)?;
    if first.status != 401 {
        ensure_success(&first, "manifest")?;
        return Ok((first, None));
    }
    let challenge = header(&first, "www-authenticate").ok_or_else(|| {
        "OCI registry returned 401 without an authentication challenge".to_owned()
    })?;
    let token = bearer_token(challenge, &value.scope())?;
    let response = get(url, accept, maximum, Some(&token))?;
    ensure_success(&response, "manifest")?;
    Ok((response, Some(token)))
}

fn bearer_token(challenge: &str, fallback_scope: &str) -> Result<String, String> {
    let parameters = challenge
        .strip_prefix("Bearer ")
        .ok_or_else(|| "OCI registry did not offer Bearer authentication".to_owned())?;
    let values = parameters
        .split(',')
        .filter_map(|part| {
            let (name, value) = part.trim().split_once('=')?;
            Some((name, value.trim_matches('"')))
        })
        .collect::<BTreeMap<_, _>>();
    let realm = values
        .get("realm")
        .ok_or_else(|| "OCI Bearer challenge has no realm".to_owned())?;
    let mut query = Vec::new();
    if let Some(service) = values.get("service") {
        query.push(format!("service={}", encode(service)));
    }
    let scope = values.get("scope").copied().unwrap_or(fallback_scope);
    query.push(format!("scope={}", encode(scope)));
    let separator = if realm.contains('?') { '&' } else { '?' };
    let response = get(
        &format!("{realm}{separator}{}", query.join("&")),
        "application/json",
        1024 * 1024,
        None,
    )?;
    ensure_success(&response, "token")?;
    let value: TokenResponse = serde_json::from_slice(&response.body)
        .map_err(|error| format!("invalid OCI token response: {error}"))?;
    value
        .token
        .or(value.access_token)
        .filter(|token| !token.is_empty())
        .ok_or_else(|| "OCI token response has no token".into())
}

fn get(url: &str, accept: &str, maximum: u64, token: Option<&str>) -> Result<Response, String> {
    let mut headers = vec![Header {
        name: "accept".into(),
        value: accept.into(),
    }];
    if let Some(token) = token {
        headers.push(Header {
            name: "authorization".into(),
            value: format!("Bearer {token}"),
        });
    }
    http_client::send(&Request {
        method: "GET".into(),
        url: url.into(),
        headers,
        body: Vec::new(),
        max_response_bytes: maximum,
    })
}

fn select_layer(layers: &[Layer]) -> Result<&Layer, String> {
    let candidates = layers
        .iter()
        .filter(|layer| {
            layer.media_type.contains("wasm")
                || layer
                    .annotations
                    .get("org.opencontainers.image.title")
                    .is_some_and(|title| title.ends_with(".wasm"))
        })
        .collect::<Vec<_>>();
    match candidates.as_slice() {
        [layer] => Ok(*layer),
        [] if layers.len() == 1 => Ok(&layers[0]),
        [] => Err("OCI manifest has no WebAssembly component layer".into()),
        _ => Err("OCI manifest has multiple WebAssembly component layers".into()),
    }
}

fn ensure_success(response: &Response, label: &str) -> Result<(), String> {
    if (200..300).contains(&response.status) {
        Ok(())
    } else {
        Err(format!(
            "OCI registry returned status {} for {label}",
            response.status
        ))
    }
}

fn header<'a>(response: &'a Response, name: &str) -> Option<&'a str> {
    response
        .headers
        .iter()
        .find(|header| header.name.eq_ignore_ascii_case(name))
        .map(|header| header.value.as_str())
}

fn digest(bytes: &[u8]) -> String {
    format!("sha256:{:x}", Sha256::digest(bytes))
}

fn encode(value: &str) -> String {
    value
        .bytes()
        .map(|byte| match byte {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                (byte as char).to_string()
            }
            _ => format!("%{byte:02X}"),
        })
        .collect()
}

fn valid_name(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 96
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.'))
}
