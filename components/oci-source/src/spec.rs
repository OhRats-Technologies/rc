pub struct OciSpec {
    pub registry: String,
    pub repository: String,
    pub reference: String,
}

impl OciSpec {
    pub fn parse(value: &str) -> Result<Self, String> {
        let value = value.trim_start_matches("//");
        let (registry, remainder) = value
            .split_once('/')
            .ok_or_else(|| "OCI source must include registry/repository".to_owned())?;
        if !valid_registry(registry) {
            return Err(format!("invalid OCI registry {registry:?}"));
        }
        let (repository, reference) = if let Some((repository, digest)) = remainder.rsplit_once('@')
        {
            validate_digest(digest)?;
            (repository, digest)
        } else {
            let slash = remainder.rfind('/').unwrap_or(0);
            match remainder[slash..].rfind(':') {
                Some(offset) => {
                    let index = slash + offset;
                    (&remainder[..index], &remainder[index + 1..])
                }
                None => (remainder, "latest"),
            }
        };
        if !valid_repository(repository) || !valid_reference(reference) {
            return Err(format!("invalid OCI reference {value:?}"));
        }
        Ok(Self {
            registry: registry.into(),
            repository: repository.into(),
            reference: reference.into(),
        })
    }

    pub fn manifest_url(&self) -> String {
        format!(
            "{}://{}/v2/{}/manifests/{}",
            self.scheme(),
            self.registry,
            self.repository,
            self.reference
        )
    }

    pub fn blob_url(&self, digest: &str) -> String {
        format!(
            "{}://{}/v2/{}/blobs/{digest}",
            self.scheme(),
            self.registry,
            self.repository
        )
    }

    pub fn source(&self, manifest_digest: &str) -> String {
        format!(
            "oci:{}/{}@{manifest_digest}",
            self.registry, self.repository
        )
    }

    pub fn package_name(&self) -> String {
        self.repository
            .rsplit('/')
            .next()
            .unwrap_or("component")
            .to_owned()
    }

    pub fn scope(&self) -> String {
        format!("repository:{}:pull", self.repository)
    }

    fn scheme(&self) -> &str {
        if self.registry.starts_with("localhost") || self.registry.starts_with("127.0.0.1") {
            "http"
        } else {
            "https"
        }
    }
}

pub fn validate_digest(value: &str) -> Result<(), String> {
    if value
        .strip_prefix("sha256:")
        .is_some_and(|hex| hex.len() == 64 && hex.bytes().all(|byte| byte.is_ascii_hexdigit()))
    {
        Ok(())
    } else {
        Err(format!("invalid OCI digest {value:?}"))
    }
}

fn valid_registry(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 255
        && value.bytes().all(|byte| {
            byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'-' | b':' | b'[' | b']')
        })
}

fn valid_repository(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 255
        && value.split('/').all(|part| {
            !part.is_empty()
                && part.bytes().all(|byte| {
                    byte.is_ascii_lowercase()
                        || byte.is_ascii_digit()
                        || matches!(byte, b'.' | b'-' | b'_')
                })
        })
}

fn valid_reference(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 128
        && (value.starts_with("sha256:")
            || value
                .bytes()
                .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'-' | b'_')))
}

#[cfg(test)]
mod tests {
    use super::OciSpec;

    #[test]
    fn parses_registry_repository_and_tag() {
        let value = OciSpec::parse("ghcr.io/ohrats-technologies/rc/webui:2.4.1").unwrap();
        assert_eq!(value.registry, "ghcr.io");
        assert_eq!(value.repository, "ohrats-technologies/rc/webui");
        assert_eq!(value.reference, "2.4.1");
        assert_eq!(value.package_name(), "webui");
    }

    #[test]
    fn keeps_digest_references_exact() {
        let digest = format!("sha256:{}", "a".repeat(64));
        let value = OciSpec::parse(&format!("ghcr.io/example/plugin@{digest}")).unwrap();
        assert_eq!(value.reference, digest);
    }

    #[test]
    fn permits_plain_http_only_for_local_registry_testing() {
        let value = OciSpec::parse("127.0.0.1:5000/example/plugin:latest").unwrap();
        assert!(value.manifest_url().starts_with("http://127.0.0.1:5000/"));
        let remote = OciSpec::parse("registry.example/example/plugin:latest").unwrap();
        assert!(
            remote
                .manifest_url()
                .starts_with("https://registry.example/")
        );
    }
}
