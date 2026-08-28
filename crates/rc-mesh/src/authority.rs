use crate::{RealmId, ServiceId};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum CoordinatorRole {
    Tier0,
    Secondary,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CoordinatorPolicy {
    role: CoordinatorRole,
    upstream: Option<ServiceId>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RevocationLease {
    pub realm: RealmId,
    pub epoch: u64,
    pub issued_at: i64,
    pub valid_until: i64,
    pub tombstone_root: String,
}

impl CoordinatorPolicy {
    pub fn tier0() -> Self {
        Self {
            role: CoordinatorRole::Tier0,
            upstream: None,
        }
    }

    pub fn secondary(upstream: ServiceId) -> Self {
        Self {
            role: CoordinatorRole::Secondary,
            upstream: Some(upstream),
        }
    }

    pub fn from_parts(
        role: CoordinatorRole,
        upstream: Option<ServiceId>,
    ) -> Result<Self, PolicyError> {
        match (role, upstream) {
            (CoordinatorRole::Tier0, None) => Ok(Self::tier0()),
            (CoordinatorRole::Secondary, Some(upstream)) => Ok(Self::secondary(upstream)),
            (CoordinatorRole::Tier0, Some(_)) => Err(PolicyError::Tier0Upstream),
            (CoordinatorRole::Secondary, None) => Err(PolicyError::MissingUpstream),
        }
    }

    pub fn role(&self) -> CoordinatorRole {
        self.role
    }

    pub fn upstream(&self) -> Option<&ServiceId> {
        self.upstream.as_ref()
    }
}

impl RevocationLease {
    pub fn validate(&self, now_ms: i64) -> Result<(), RevocationLeaseError> {
        if self.epoch == 0
            || self.issued_at > now_ms
            || self.valid_until <= self.issued_at
            || self.tombstone_root.len() != 64
            || !self
                .tombstone_root
                .bytes()
                .all(|byte| byte.is_ascii_hexdigit())
        {
            return Err(RevocationLeaseError::Invalid);
        }
        if self.valid_until <= now_ms {
            return Err(RevocationLeaseError::Expired);
        }
        Ok(())
    }
}

#[derive(Debug, Clone, thiserror::Error, PartialEq, Eq)]
pub enum PolicyError {
    #[error("Tier-0 coordinators cannot have an upstream authority")]
    Tier0Upstream,
    #[error("secondary coordinators require an upstream authority")]
    MissingUpstream,
}

#[derive(Debug, Clone, thiserror::Error, PartialEq, Eq)]
pub enum RevocationLeaseError {
    #[error("invalid revocation lease")]
    Invalid,
    #[error("revocation lease expired")]
    Expired,
}
