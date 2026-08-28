use sha2::{Digest, Sha256};
use std::{env, fs, path::PathBuf};

fn main() {
    let source = PathBuf::from("assets/rc.css");
    println!("cargo:rerun-if-changed={}", source.display());
    let bytes = fs::read(&source).expect("read WebUI CSS");
    let digest = format!("{:x}", Sha256::digest(&bytes));
    let generated = format!(
        "pub const CSS_PATH: &str = \"/assets/rc.{}.css\";\n",
        &digest[..16]
    );
    let output = PathBuf::from(env::var_os("OUT_DIR").expect("OUT_DIR"));
    fs::write(output.join("assets.rs"), generated).expect("write WebUI asset metadata");
}
