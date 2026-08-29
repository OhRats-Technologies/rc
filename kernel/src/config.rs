use anyhow::Context as _;
use std::path::{Path, PathBuf};

pub const WASMTIME_CACHE_BYTES: u64 = 256 * 1024 * 1024;
pub const WASMTIME_CACHE_FILES: u64 = 4_096;

pub fn default_data_dir() -> PathBuf {
    std::env::var_os("RC_DATA_DIR")
        .map(PathBuf::from)
        .unwrap_or_else(|| {
            let home = std::env::var_os("HOME")
                .map(PathBuf::from)
                .unwrap_or_else(|| PathBuf::from("."));
            home.join(".local/share/rc")
        })
}

pub fn default_component_dir() -> PathBuf {
    std::env::var_os("RC_COMPONENT_DIR")
        .map(PathBuf::from)
        .unwrap_or_else(|| default_data_dir().join("components"))
}

pub fn wasmtime_cache_dir(component_dir: &Path) -> anyhow::Result<PathBuf> {
    wasmtime_cache_dir_with_override(
        component_dir,
        std::env::var_os("RC_WASMTIME_CACHE_DIR").map(PathBuf::from),
    )
}

fn wasmtime_cache_dir_with_override(
    component_dir: &Path,
    cache_override: Option<PathBuf>,
) -> anyhow::Result<PathBuf> {
    let path = cache_override.unwrap_or_else(|| {
        component_dir
            .parent()
            .unwrap_or(component_dir)
            .join("cache/wasmtime")
    });
    if path.is_absolute() {
        Ok(path)
    } else {
        Ok(std::env::current_dir()
            .context("failed to resolve the kernel cache directory")?
            .join(path))
    }
}

pub fn prepare_private_dir(path: &Path) -> anyhow::Result<()> {
    std::fs::create_dir_all(path).with_context(|| {
        format!(
            "failed to create private cache directory {}",
            path.display()
        )
    })?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt as _;
        std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o700))
            .with_context(|| format!("failed to protect cache directory {}", path.display()))?;
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::wasmtime_cache_dir_with_override;
    use std::path::{Path, PathBuf};

    #[test]
    fn wasmtime_cache_is_below_component_root_cache() {
        assert_eq!(
            wasmtime_cache_dir_with_override(Path::new("/data/components"), None).unwrap(),
            Path::new("/data/cache/wasmtime")
        );
    }

    #[test]
    fn wasmtime_cache_override_is_used() {
        assert_eq!(
            wasmtime_cache_dir_with_override(
                Path::new("/data/components"),
                Some(PathBuf::from("/tmp/rc-test-cache")),
            )
            .unwrap(),
            Path::new("/tmp/rc-test-cache")
        );
    }
}
