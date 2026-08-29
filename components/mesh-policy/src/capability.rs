use crate::{
    ohrats::rc_mesh::types::{
        CapabilityAdvertisement, CapabilityRequirement, NegotiatedCapability, Rejection,
    },
    validate,
};
use std::collections::BTreeSet;

pub fn negotiate(
    local: CapabilityRequirement,
    remote: CapabilityAdvertisement,
) -> Result<Option<NegotiatedCapability>, Rejection> {
    validate::capability(&CapabilityAdvertisement {
        id: local.id.clone(),
        versions: local.versions.clone(),
        features: local.supported_features.clone(),
    })?;
    validate::capability(&remote)?;
    if local.required_features.len() > validate::MAX_CAPABILITY_FEATURES
        || !strictly_sorted(&local.required_features)
        || local
            .required_features
            .iter()
            .any(|feature| local.supported_features.binary_search(feature).is_err())
    {
        return Err(Rejection::InvalidCapability);
    }
    if local.id != remote.id {
        return Ok(None);
    }
    let remote_versions = remote.versions.iter().copied().collect::<BTreeSet<_>>();
    let Some(version) = local
        .versions
        .iter()
        .rev()
        .find(|version| remote_versions.contains(version))
        .copied()
    else {
        return Ok(None);
    };
    let remote_features = remote.features.iter().cloned().collect::<BTreeSet<_>>();
    if local
        .required_features
        .iter()
        .any(|feature| !remote_features.contains(feature))
    {
        return Ok(None);
    }
    Ok(Some(NegotiatedCapability {
        id: local.id,
        version,
        features: local
            .supported_features
            .into_iter()
            .filter(|feature| remote_features.contains(feature))
            .collect(),
    }))
}

fn strictly_sorted<T: Ord>(values: &[T]) -> bool {
    values.windows(2).all(|pair| pair[0] < pair[1])
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn chooses_highest_common_version_and_intersection() {
        let result = negotiate(
            CapabilityRequirement {
                id: "rc.transport.quic".into(),
                versions: vec![1, 2, 3],
                supported_features: vec!["datagram".into(), "relay".into()],
                required_features: vec!["datagram".into()],
            },
            CapabilityAdvertisement {
                id: "rc.transport.quic".into(),
                versions: vec![1, 2],
                features: vec!["datagram".into()],
            },
        )
        .unwrap()
        .unwrap();
        assert_eq!(result.version, 2);
        assert_eq!(result.features, vec!["datagram"]);
    }
}
