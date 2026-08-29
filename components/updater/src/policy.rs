use crate::ohrats::rc_updater::{artifact_source, native_replacement};
use semver::Version;
use sha2::Digest as _;

const MAX_ARTIFACT: usize = 160 * 1024 * 1024;

pub fn upgrade(args: &[String]) -> Result<u32, String> {
    let (expected, reexec) = parse(args)?;
    let current = native_replacement::current().map_err(|error| error.to_string())?;
    let current_version = Version::parse(&current.version)
        .map_err(|_| "running kernel reported an invalid semantic version".to_owned())?;
    let handle = artifact_source::select().map_err(|error| error.to_string())?;
    let artifact = artifact_source::read(&handle).map_err(|error| error.to_string())?;
    if artifact.len() > MAX_ARTIFACT {
        return Err("kernel artifact exceeds 160 MiB".into());
    }
    let expected = normalize_digest(expected)?;
    let actual = digest(&artifact);
    if actual != expected {
        return Err("kernel artifact digest does not match the pinned digest".into());
    }
    if current.digest == expected {
        println!("kernel already at {}", current.version);
        return Ok(0);
    }
    let staged =
        native_replacement::prepare(&artifact, expected).map_err(|error| error.to_string())?;
    let target_version = Version::parse(&staged.version)
        .map_err(|_| "staged kernel reported an invalid semantic version".to_owned())?;
    if let Err(error) = validate_candidate(&current_version, &target_version) {
        native_replacement::abort(&staged);
        return Err(error);
    }
    if let Err(error) = native_replacement::commit(&staged) {
        native_replacement::abort(&staged);
        return Err(error);
    }
    if reexec {
        native_replacement::reexec().map_err(|error| error.to_string())?;
    }
    println!("upgraded kernel to {target_version}");
    Ok(0)
}

fn validate_candidate(current: &Version, candidate: &Version) -> Result<(), String> {
    if candidate <= current {
        return Err("kernel candidate is not newer than the running version".into());
    }
    if current.major != candidate.major {
        return Err("kernel candidate is incompatible with the running major version".into());
    }
    Ok(())
}

fn parse(args: &[String]) -> Result<(&str, bool), String> {
    let reexec = args.iter().any(|arg| arg == "--reexec");
    let values = args
        .iter()
        .filter(|arg| arg.as_str() != "--reexec")
        .collect::<Vec<_>>();
    match values.as_slice() {
        [expected] => Ok((expected, reexec)),
        _ => Err("usage: rc upgrade EXPECTED-DIGEST [--reexec]".into()),
    }
}

fn normalize_digest(value: &str) -> Result<&str, String> {
    if value.len() != 71
        || !value.starts_with("sha256:")
        || !value[7..]
            .bytes()
            .all(|byte| byte.is_ascii_digit() || matches!(byte, b'a'..=b'f'))
    {
        return Err("expected sha256:<64 lowercase hexadecimal digits>".into());
    }
    Ok(value)
}

fn digest(bytes: &[u8]) -> String {
    format!("sha256:{:x}", sha2::Sha256::digest(bytes))
}

#[cfg(test)]
mod tests {
    use super::{digest, normalize_digest, validate_candidate};
    use semver::Version;

    #[test]
    fn digest_is_content_addressed() {
        assert_eq!(
            digest(b"rc"),
            "sha256:7581e0d493f6ab3fc525c4dce2a5f835c0399ebb3207d49e609cbccb2d902a42"
        );
    }

    #[test]
    fn digest_shape_is_strict() {
        assert!(
            normalize_digest(
                "sha256:0000000000000000000000000000000000000000000000000000000000000000"
            )
            .is_ok()
        );
        assert!(normalize_digest("sha256:ABC").is_err());
    }

    #[test]
    fn candidate_major_mismatch_is_rejected() {
        assert!(
            validate_candidate(
                &Version::parse("1.1.0").unwrap(),
                &Version::parse("2.0.0").unwrap()
            )
            .is_err()
        );
    }
}
