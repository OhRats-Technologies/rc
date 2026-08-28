wit_bindgen::generate!({
    path: "../../wit",
    world: "http-source",
});

use exports::ohrats::rc_plugin::package_source::{Guest as SourceGuest, PackageArtifact};
use ohrats::rc_plugin::{
    http_client::{self, Header, Request},
    types::Service,
};
use sha2::{Digest, Sha256};

struct HttpSource;

impl Guest for HttpSource {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:http-source".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-plugin/package-source".into(),
                version: "0.1.0".into(),
                priority: 80,
                keys: vec!["http".into(), "https".into()],
            }],
            requires: Vec::new(),
            commands: Vec::new(),
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        Err(format!("unsupported command {command:?}"))
    }
}

impl SourceGuest for HttpSource {
    fn resolve(scheme: String, spec: String) -> Result<PackageArtifact, String> {
        if !matches!(scheme.as_str(), "http" | "https") {
            return Err(format!("HTTP source does not handle {scheme:?}"));
        }
        let url = normalize_url(&scheme, &spec)?;
        let response = http_client::send(&Request {
            method: "GET".into(),
            url,
            headers: vec![Header {
                name: "accept".into(),
                value: "application/wasm, application/octet-stream".into(),
            }],
            body: Vec::new(),
            max_response_bytes: 48 * 1024 * 1024,
        })?;
        if !(200..300).contains(&response.status) {
            return Err(format!("HTTP source returned status {}", response.status));
        }
        let name = artifact_name(&response.final_url)?;
        let digest = format!("sha256:{:x}", Sha256::digest(&response.body));
        Ok(PackageArtifact {
            name,
            source: response.final_url,
            digest,
            bytes: response.body,
        })
    }
}

fn normalize_url(scheme: &str, spec: &str) -> Result<String, String> {
    let value = if spec.starts_with("http://") || spec.starts_with("https://") {
        spec.to_owned()
    } else {
        format!("{scheme}:{spec}")
    };
    if value.starts_with(&format!("{scheme}://")) {
        Ok(value)
    } else {
        Err(format!("source URL does not use {scheme}"))
    }
}

fn artifact_name(url: &str) -> Result<String, String> {
    let path = url.split(['?', '#']).next().unwrap_or(url);
    let filename = path
        .rsplit('/')
        .next()
        .filter(|value| !value.is_empty())
        .ok_or_else(|| "source URL has no artifact filename".to_owned())?;
    let name = filename.strip_suffix(".wasm").unwrap_or(filename);
    if valid_name(name) {
        Ok(name.into())
    } else {
        Err(format!("invalid artifact filename {filename:?}"))
    }
}

fn valid_name(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 96
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.'))
}

#[cfg(test)]
mod tests {
    use super::{artifact_name, normalize_url};

    #[test]
    fn derives_component_name_from_redirected_url() {
        assert_eq!(
            artifact_name("https://example.test/releases/logger.wasm?download=1").unwrap(),
            "logger"
        );
    }

    #[test]
    fn rejects_cross_scheme_resolution() {
        assert!(normalize_url("https", "http://example.test/plugin.wasm").is_err());
    }
}

export!(HttpSource);
