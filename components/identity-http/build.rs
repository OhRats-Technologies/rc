use sha2::{Digest, Sha256};
use std::{env, fs, path::PathBuf};

fn main() {
    let source = PathBuf::from("assets/auth.js");
    println!("cargo:rerun-if-changed={}", source.display());
    let bytes = fs::read(&source).expect("read identity browser asset");
    let digest = format!("{:x}", Sha256::digest(&bytes));
    let generated = format!(
        "pub const AUTH_SCRIPT_PATH: &str = \"/assets/identity-auth.{}.js\";\n",
        &digest[..16]
    );
    let output = PathBuf::from(env::var_os("OUT_DIR").expect("OUT_DIR"));
    fs::write(output.join("assets.rs"), generated).expect("write identity asset metadata");
}
