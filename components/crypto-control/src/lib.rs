wit_bindgen::generate!({
    path: "../../wit",
    world: "crypto-control",
    generate_all,
});

use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use exports::ohrats::rc_crypto::control::{
    Guest as ControlGuest, GuestSession, NodeOpen, Session, SessionBorrow,
};
use ohrats::{
    rc_keys::{
        host_custody,
        types::{KeyAlgorithm, PublicKey},
    },
    rc_plugin::types::Service,
};
use sha2::{Digest, Sha256};

const MAX_PUBLIC_KEY_TEXT: usize = 128;
const MAX_CONTEXT_TEXT: usize = 256;

struct CryptoControl;

struct ControlSession {
    key: host_custody::SessionKey,
}

impl Guest for CryptoControl {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:crypto-control".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-crypto/control".into(),
                version: "0.1.0".into(),
                priority: 100,
                keys: Vec::new(),
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

impl ControlGuest for CryptoControl {
    type Session = ControlSession;

    fn static_slot(device_id: String) -> String {
        static_slot(&device_id)
    }

    fn static_public(device_id: String) -> Result<String, String> {
        validate_text(&device_id, "device id")?;
        let key = host_custody::ensure(&static_slot(&device_id), KeyAlgorithm::X25519)?;
        encode_public(key.public_key()?)
    }

    fn open_node(
        device_id: String,
        client_id: String,
        challenge: String,
        client_public_key: String,
    ) -> Result<NodeOpen, String> {
        validate_text(&device_id, "device id")?;
        validate_text(&client_id, "client id")?;
        validate_text(&challenge, "challenge")?;
        let peer = decode_public(&client_public_key)?;
        let static_key = host_custody::ensure(&static_slot(&device_id), KeyAlgorithm::X25519)?;
        let ephemeral = host_custody::generate(KeyAlgorithm::X25519)?;
        let static_public = encode_public(static_key.public_key()?)?;
        let ephemeral_public = encode_public(ephemeral.public_key()?)?;
        let shared_static = static_key.agree(&peer)?;
        let shared_ephemeral = ephemeral.agree(&peer)?;
        let salt = Sha256::digest(challenge.as_bytes()).to_vec();
        let info = format!("rc-e2e-v2\n{device_id}\n{client_id}").into_bytes();
        let key = host_custody::derive(&shared_static, &shared_ephemeral, &salt, &info)?;
        Ok(NodeOpen {
            session: Session::new(ControlSession { key }),
            static_public_key: static_public,
            ephemeral_public_key: ephemeral_public,
        })
    }

    fn encrypt(
        session: SessionBorrow<'_>,
        direction: u8,
        sequence: u64,
        session_id: String,
        label: String,
        plaintext: Vec<u8>,
    ) -> Result<String, String> {
        let session = session.get::<ControlSession>();
        let ciphertext = session.key.encrypt(
            &frame_nonce(direction, sequence),
            &frame_aad(&session_id, sequence, &label),
            &plaintext,
        )?;
        Ok(URL_SAFE_NO_PAD.encode(ciphertext))
    }

    fn decrypt(
        session: SessionBorrow<'_>,
        direction: u8,
        sequence: u64,
        session_id: String,
        label: String,
        ciphertext: String,
    ) -> Result<Vec<u8>, String> {
        let session = session.get::<ControlSession>();
        let ciphertext = URL_SAFE_NO_PAD
            .decode(ciphertext)
            .map_err(|_| "invalid control ciphertext encoding".to_owned())?;
        session.key.decrypt(
            &frame_nonce(direction, sequence),
            &frame_aad(&session_id, sequence, &label),
            &ciphertext,
        )
    }
}

impl GuestSession for ControlSession {}

fn static_slot(device_id: &str) -> String {
    format!("control:{device_id}:transport")
}

fn encode_public(value: PublicKey) -> Result<String, String> {
    if value.algorithm != KeyAlgorithm::X25519 || value.bytes.len() != 32 {
        return Err("control custody returned an invalid X25519 key".into());
    }
    Ok(URL_SAFE_NO_PAD.encode(value.bytes))
}

fn decode_public(value: &str) -> Result<Vec<u8>, String> {
    if value.len() > MAX_PUBLIC_KEY_TEXT {
        return Err("invalid X25519 public key".into());
    }
    let bytes = URL_SAFE_NO_PAD
        .decode(value)
        .map_err(|_| "invalid X25519 public key".to_owned())?;
    if bytes.len() != 32 {
        return Err("invalid X25519 public key".into());
    }
    Ok(bytes)
}

fn validate_text(value: &str, label: &str) -> Result<(), String> {
    if value.is_empty() || value.len() > MAX_CONTEXT_TEXT || value.contains('\n') {
        Err(format!("invalid {label}"))
    } else {
        Ok(())
    }
}

fn frame_nonce(direction: u8, sequence: u64) -> Vec<u8> {
    let mut nonce = [0_u8; 12];
    nonce[0] = direction;
    nonce[4..].copy_from_slice(&sequence.to_be_bytes());
    nonce.to_vec()
}

fn frame_aad(session_id: &str, sequence: u64, label: &str) -> Vec<u8> {
    format!("rc-frame-v1\n{session_id}\n{sequence}\n{label}").into_bytes()
}

export!(CryptoControl);
