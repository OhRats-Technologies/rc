use std::collections::BTreeSet;

pub const MAX_CAPABILITIES: usize = 64;
pub const MAX_CAPABILITY_VERSIONS: usize = 16;
pub const MAX_CAPABILITY_FEATURES: usize = 32;

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CapabilityAdvertisement {
    pub id: String,
    pub versions: Vec<u32>,
    #[serde(default)]
    pub features: Vec<String>,
}

impl CapabilityAdvertisement {
    pub fn new<I, V, F>(id: I, versions: V, features: F) -> Result<Self, CapabilityError>
    where
        I: Into<String>,
        V: IntoIterator<Item = u32>,
        F: IntoIterator,
        F::Item: Into<String>,
    {
        let mut versions = versions.into_iter().collect::<Vec<_>>();
        versions.sort_unstable();
        versions.dedup();
        let mut features = features.into_iter().map(Into::into).collect::<Vec<_>>();
        features.sort();
        features.dedup();
        let value = Self {
            id: id.into(),
            versions,
            features,
        };
        value.validate()?;
        Ok(value)
    }

    pub fn validate(&self) -> Result<(), CapabilityError> {
        if !valid_id(&self.id) {
            return Err(CapabilityError::Id);
        }
        if self.versions.is_empty()
            || self.versions.len() > MAX_CAPABILITY_VERSIONS
            || self.versions.contains(&0)
            || !strictly_sorted(&self.versions)
        {
            return Err(CapabilityError::Versions);
        }
        if self.features.len() > MAX_CAPABILITY_FEATURES
            || self.features.iter().any(|feature| !valid_feature(feature))
            || !strictly_sorted(&self.features)
        {
            return Err(CapabilityError::Features);
        }
        Ok(())
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CapabilityRequirement {
    pub id: String,
    pub versions: Vec<u32>,
    pub supported_features: Vec<String>,
    pub required_features: Vec<String>,
}

impl CapabilityRequirement {
    pub fn new<I, V, S, R>(
        id: I,
        versions: V,
        supported_features: S,
        required_features: R,
    ) -> Result<Self, CapabilityError>
    where
        I: Into<String>,
        V: IntoIterator<Item = u32>,
        S: IntoIterator,
        S::Item: Into<String>,
        R: IntoIterator,
        R::Item: Into<String>,
    {
        let local = CapabilityAdvertisement::new(id, versions, supported_features)?;
        let mut required_features = required_features
            .into_iter()
            .map(Into::into)
            .collect::<Vec<_>>();
        required_features.sort();
        required_features.dedup();
        if required_features.len() > MAX_CAPABILITY_FEATURES
            || required_features
                .iter()
                .any(|feature| !local.features.binary_search(feature).is_ok())
        {
            return Err(CapabilityError::RequiredFeatures);
        }
        Ok(Self {
            id: local.id,
            versions: local.versions,
            supported_features: local.features,
            required_features,
        })
    }

    pub fn from_local(
        local: &CapabilityAdvertisement,
        required_features: impl IntoIterator<Item = impl Into<String>>,
    ) -> Result<Self, CapabilityError> {
        local.validate()?;
        Self::new(
            local.id.clone(),
            local.versions.clone(),
            local.features.clone(),
            required_features,
        )
    }

    pub fn validate(&self) -> Result<(), CapabilityError> {
        CapabilityAdvertisement {
            id: self.id.clone(),
            versions: self.versions.clone(),
            features: self.supported_features.clone(),
        }
        .validate()?;
        if self.required_features.len() > MAX_CAPABILITY_FEATURES
            || !strictly_sorted(&self.required_features)
            || self.required_features.iter().any(|feature| {
                !valid_feature(feature) || self.supported_features.binary_search(feature).is_err()
            })
        {
            return Err(CapabilityError::RequiredFeatures);
        }
        Ok(())
    }

    pub fn negotiate(&self, remote: &CapabilityAdvertisement) -> Option<NegotiatedCapability> {
        self.validate().ok()?;
        remote.validate().ok()?;
        if self.id != remote.id {
            return None;
        }
        let remote_versions = remote.versions.iter().copied().collect::<BTreeSet<_>>();
        let version = self
            .versions
            .iter()
            .rev()
            .find(|version| remote_versions.contains(version))
            .copied()?;
        let remote_features = remote.features.iter().cloned().collect::<BTreeSet<_>>();
        if self
            .required_features
            .iter()
            .any(|feature| !remote_features.contains(feature))
        {
            return None;
        }
        let features = self
            .supported_features
            .iter()
            .filter(|feature| remote_features.contains(*feature))
            .cloned()
            .collect();
        Some(NegotiatedCapability {
            id: self.id.clone(),
            version,
            features,
        })
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct NegotiatedCapability {
    pub id: String,
    pub version: u32,
    pub features: Vec<String>,
}

fn strictly_sorted<T: Ord>(values: &[T]) -> bool {
    values.windows(2).all(|pair| pair[0] < pair[1])
}

fn valid_id(value: &str) -> bool {
    valid_token(value, 128, true)
}

fn valid_feature(value: &str) -> bool {
    valid_token(value, 64, false)
}

fn valid_token(value: &str, max_len: usize, allow_slash: bool) -> bool {
    !value.is_empty()
        && value.len() <= max_len
        && value.bytes().all(|byte| {
            byte.is_ascii_lowercase()
                || byte.is_ascii_digit()
                || matches!(byte, b'.' | b'-' | b'_')
                || (allow_slash && byte == b'/')
        })
        && value
            .as_bytes()
            .first()
            .is_some_and(u8::is_ascii_alphanumeric)
        && value
            .as_bytes()
            .last()
            .is_some_and(u8::is_ascii_alphanumeric)
}

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum CapabilityError {
    #[error("invalid capability id")]
    Id,
    #[error("invalid capability versions")]
    Versions,
    #[error("invalid capability features")]
    Features,
    #[error("required capability features are not locally supported")]
    RequiredFeatures,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn negotiates_highest_common_version_and_feature_intersection() {
        let local = CapabilityAdvertisement::new(
            "rc.transport.quic",
            [1, 2, 3],
            ["datagram", "ipv6", "relay"],
        )
        .unwrap();
        let request = CapabilityRequirement::from_local(&local, ["datagram"]).unwrap();
        let remote =
            CapabilityAdvertisement::new("rc.transport.quic", [1, 2], ["datagram", "relay"])
                .unwrap();
        assert_eq!(
            request.negotiate(&remote),
            Some(NegotiatedCapability {
                id: "rc.transport.quic".into(),
                version: 2,
                features: vec!["datagram".into(), "relay".into()],
            })
        );
    }

    #[test]
    fn rejects_missing_required_features() {
        let request =
            CapabilityRequirement::new("rc.transport.quic", [1], ["datagram"], ["datagram"])
                .unwrap();
        let remote =
            CapabilityAdvertisement::new("rc.transport.quic", [1], Vec::<String>::new()).unwrap();
        assert!(request.negotiate(&remote).is_none());
    }

    #[test]
    fn rejects_noncanonical_requirements_even_when_manually_constructed() {
        let request = CapabilityRequirement {
            id: "rc.transport.quic".into(),
            versions: vec![2, 1],
            supported_features: vec!["datagram".into()],
            required_features: vec!["datagram".into()],
        };
        let remote =
            CapabilityAdvertisement::new("rc.transport.quic", [1, 2], ["datagram"]).unwrap();
        assert!(request.negotiate(&remote).is_none());
    }
}
