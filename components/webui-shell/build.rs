use sha2::{Digest, Sha256};
use std::{env, fs, path::PathBuf};

fn main() {
    let assets = [
        ("STYLES_PATH", "assets/styles.css", "css"),
        ("PUBLIC_STYLES_PATH", "assets/public.css", "css"),
        ("COPY_SCRIPT_PATH", "assets/copy.js", "js"),
        ("SOCIAL_CARD_PATH", "assets/social-card.png", "png"),
    ];
    let mut generated = String::new();
    for (name, source, extension) in assets {
        let source = PathBuf::from(source);
        println!("cargo:rerun-if-changed={}", source.display());
        let bytes = fs::read(&source).expect("read WebUI asset");
        let digest = format!("{:x}", Sha256::digest(&bytes));
        generated.push_str(&format!(
            "pub const {name}: &str = \"/assets/rc.{}.{}\";\n",
            &digest[..16],
            extension
        ));
    }
    println!("cargo:rerun-if-changed=assets/public_snapshots");
    let output = PathBuf::from(env::var_os("OUT_DIR").expect("OUT_DIR"));
    fs::write(output.join("assets.rs"), generated).expect("write WebUI asset metadata");
}
