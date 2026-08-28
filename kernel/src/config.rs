use std::path::PathBuf;

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
