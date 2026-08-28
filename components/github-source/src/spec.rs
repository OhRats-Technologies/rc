pub struct GithubSpec {
    pub owner: String,
    pub repository: String,
    pub path: String,
    pub revision: Option<String>,
}

impl GithubSpec {
    pub fn parse(value: &str) -> Result<Self, String> {
        let (body, revision) = value
            .split_once('#')
            .map_or((value, None), |(body, revision)| (body, Some(revision)));
        let (repository, path) = body
            .split_once("//")
            .map_or((body, ""), |(repository, path)| (repository, path));
        let mut parts = repository.split('/');
        let owner = parts.next().unwrap_or_default();
        let repository = parts.next().unwrap_or_default();
        if parts.next().is_some() || !valid_token(owner) || !valid_token(repository) {
            return Err(format!("invalid GitHub repository {repository:?}"));
        }
        if !path.is_empty() && !valid_path(path) {
            return Err(format!("invalid GitHub package path {path:?}"));
        }
        if revision.is_some_and(|value| !valid_revision(value)) {
            return Err("invalid GitHub revision".into());
        }
        Ok(Self {
            owner: owner.into(),
            repository: repository.into(),
            path: path.trim_matches('/').into(),
            revision: revision.map(str::to_owned),
        })
    }

    pub fn package_name(&self) -> Result<String, String> {
        let value = self
            .path
            .rsplit('/')
            .next()
            .filter(|value| !value.is_empty())
            .unwrap_or(&self.repository);
        let value = value.strip_suffix(".wasm").unwrap_or(value);
        valid_token(value)
            .then(|| value.to_owned())
            .ok_or_else(|| format!("invalid GitHub package name {value:?}"))
    }

    pub fn display(&self, revision: &str) -> String {
        let path = if self.path.is_empty() {
            String::new()
        } else {
            format!("//{}", self.path)
        };
        format!(
            "github:{}/{}{}#{revision}",
            self.owner, self.repository, path
        )
    }
}

fn valid_token(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 100
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.'))
}

fn valid_revision(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 128
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.' | b'/'))
}

fn valid_path(value: &str) -> bool {
    value.len() <= 512
        && value
            .split('/')
            .all(|part| part != ".." && (part.is_empty() || valid_token(part)))
}

#[cfg(test)]
mod tests {
    use super::GithubSpec;

    #[test]
    fn parses_monorepo_subpath_and_release_tag() {
        let value =
            GithubSpec::parse("OhRats-Technologies/rc//components/webui#webui-v2.4.1").unwrap();
        assert_eq!(value.owner, "OhRats-Technologies");
        assert_eq!(value.repository, "rc");
        assert_eq!(value.path, "components/webui");
        assert_eq!(value.revision.as_deref(), Some("webui-v2.4.1"));
        assert_eq!(value.package_name().unwrap(), "webui");
    }

    #[test]
    fn accepts_a_raw_component_path_at_a_revision() {
        let value = GithubSpec::parse("fern/plugins//dist/logger.wasm#abcdef12").unwrap();
        assert_eq!(value.package_name().unwrap(), "logger");
        assert_eq!(
            value.display("abcdef12"),
            "github:fern/plugins//dist/logger.wasm#abcdef12"
        );
    }

    #[test]
    fn rejects_repository_traversal() {
        assert!(GithubSpec::parse("fern/plugins//../private.wasm#main").is_err());
    }
}
