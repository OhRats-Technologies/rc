use crate::{PeerId, RealmId, sign_payload, verify_payload};

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AuthorityHead {
    pub generation: u64,
    pub hash: String,
    pub valid_until: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ReleaseHead {
    pub version: String,
    pub target: String,
    pub sha256: String,
    pub size: u64,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MeshStateDigest {
    pub v: u32,
    pub realm_id: RealmId,
    pub origin: PeerId,
    pub sequence: u64,
    pub issued_at: i64,
    pub expires_at: i64,
    pub authority: AuthorityHead,
    pub device_operations_root: String,
    pub revocation_epoch: u64,
    #[serde(default)]
    pub releases: Vec<ReleaseHead>,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SignedStateDigest {
    pub digest: MeshStateDigest,
    pub signature: String,
}

impl SignedStateDigest {
    pub fn sign(digest: MeshStateDigest, identity_seed: &str) -> anyhow::Result<Self> {
        validate(&digest)?;
        let payload = serde_json::to_vec(&digest)?;
        Ok(Self {
            signature: sign_payload(identity_seed, "rc-mesh-state-v1", &payload)?,
            digest,
        })
    }

    pub fn verify(&self, public_key: &str, now_ms: i64) -> Result<(), StateDigestError> {
        validate(&self.digest)?;
        if self.digest.issued_at > now_ms + 60_000 || self.digest.expires_at <= now_ms {
            return Err(StateDigestError::Expired);
        }
        let expected =
            PeerId::from_public_key(public_key).map_err(|_| StateDigestError::Identity)?;
        if expected != self.digest.origin {
            return Err(StateDigestError::Identity);
        }
        let payload = serde_json::to_vec(&self.digest).map_err(|_| StateDigestError::Encoding)?;
        verify_payload(public_key, "rc-mesh-state-v1", &payload, &self.signature)
            .map_err(|_| StateDigestError::Signature)
    }
}

fn validate(digest: &MeshStateDigest) -> Result<(), StateDigestError> {
    if digest.v != 1
        || digest.realm_id.as_str().is_empty()
        || digest.origin.as_str().is_empty()
        || digest.expires_at <= digest.issued_at
        || digest.authority.hash.len() != 64
        || digest.device_operations_root.len() != 64
        || digest.releases.len() > 64
        || digest.releases.iter().any(|release| {
            release.version.is_empty()
                || release.target.is_empty()
                || release.sha256.len() != 64
                || release.size == 0
        })
    {
        return Err(StateDigestError::Shape);
    }
    Ok(())
}

#[derive(Debug, thiserror::Error)]
pub enum StateDigestError {
    #[error("invalid mesh state digest")]
    Shape,
    #[error("mesh state digest expired")]
    Expired,
    #[error("mesh state digest identity mismatch")]
    Identity,
    #[error("invalid mesh state digest signature")]
    Signature,
    #[error("mesh state digest encoding failed")]
    Encoding,
}
