use anyhow::Context as _;
use std::path::{Path, PathBuf};

pub const WASMTIME_CACHE_BYTES: u64 = 256 * 1024 * 1024;
pub const WASMTIME_CACHE_FILES: u64 = 4_096;

pub fn default_component_dir() -> PathBuf {
    rc_platform::component_dir().expect("RC component directory is unavailable")
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
    rc_platform::protect_private_path(path, true)
        .with_context(|| format!("failed to protect cache directory {}", path.display()))?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::wasmtime_cache_dir_with_override;
    use std::path::{Path, PathBuf};

    #[cfg(windows)]
    const COMPONENTS: &str = r"C:\data\components";
    #[cfg(not(windows))]
    const COMPONENTS: &str = "/data/components";
    #[cfg(windows)]
    const CACHE: &str = r"C:\data\cache\wasmtime";
    #[cfg(not(windows))]
    const CACHE: &str = "/data/cache/wasmtime";
    #[cfg(windows)]
    const OVERRIDE: &str = r"C:\temp\rc-test-cache";
    #[cfg(not(windows))]
    const OVERRIDE: &str = "/tmp/rc-test-cache";

    #[test]
    fn wasmtime_cache_is_below_component_root_cache() {
        assert_eq!(
            wasmtime_cache_dir_with_override(Path::new(COMPONENTS), None).unwrap(),
            Path::new(CACHE)
        );
    }

    #[test]
    fn wasmtime_cache_override_is_used() {
        assert_eq!(
            wasmtime_cache_dir_with_override(Path::new(COMPONENTS), Some(PathBuf::from(OVERRIDE)),)
                .unwrap(),
            Path::new(OVERRIDE)
        );
    }
}
