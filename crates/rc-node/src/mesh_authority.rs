use crate::{NodeState, load_lock};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_mesh::{PeerId, RealmId};
use rc_protocol::{AuthorityDevice, AuthoritySnapshot};
use std::{collections::BTreeMap, path::Path};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TrustedMeshDevice {
    pub device_id: String,
    pub peer_id: PeerId,
    pub identity_public_key: String,
    pub transport_public_key: String,
}

#[derive(Debug)]
pub struct MeshAuthority {
    realm_id: RealmId,
    local_device_id: String,
    local_peer_id: PeerId,
    devices: BTreeMap<String, TrustedMeshDevice>,
}

impl MeshAuthority {
    pub fn from_lock(state: &NodeState, state_dir: &Path) -> anyhow::Result<Self> {
        let lock = load_lock(state_dir)?;
        let snapshot: AuthoritySnapshot = serde_json::from_str(&lock.snapshot)?;
        let local = AuthorityDevice {
            id: state.device_id.clone(),
            identity_public_key: state.identity_public_key()?,
            transport_public_key: state.transport_public_key()?,
        };
        let source = if snapshot.devices.is_empty() {
            vec![local.clone()]
        } else {
            snapshot.devices
        };
        let mut devices = BTreeMap::new();
        for device in source {
            let trusted = validate_device(device)?;
            if devices.insert(trusted.device_id.clone(), trusted).is_some() {
                anyhow::bail!("duplicate device in RC Lock mesh authority");
            }
        }
        let accepted_local = devices
            .get(&state.device_id)
            .ok_or_else(|| anyhow::anyhow!("RC Lock does not include this device"))?;
        if accepted_local.identity_public_key != local.identity_public_key
            || accepted_local.transport_public_key != local.transport_public_key
        {
            anyhow::bail!("RC Lock mesh identity does not match local Node state");
        }
        Ok(Self {
            realm_id: RealmId::new(snapshot.workspace_id)?,
            local_device_id: state.device_id.clone(),
            local_peer_id: accepted_local.peer_id.clone(),
            devices,
        })
    }

    pub fn realm_id(&self) -> &RealmId {
        &self.realm_id
    }

    pub fn local_device_id(&self) -> &str {
        &self.local_device_id
    }

    pub fn local_peer_id(&self) -> &PeerId {
        &self.local_peer_id
    }

    pub fn devices(&self) -> impl Iterator<Item = &TrustedMeshDevice> {
        self.devices.values()
    }

    pub fn device(&self, device_id: &str) -> Option<&TrustedMeshDevice> {
        self.devices.get(device_id)
    }
}

fn validate_device(device: AuthorityDevice) -> anyhow::Result<TrustedMeshDevice> {
    if device.id.is_empty() {
        anyhow::bail!("empty device ID in RC Lock mesh authority");
    }
    let peer_id = PeerId::from_public_key(&device.identity_public_key)?;
    let transport = URL_SAFE_NO_PAD.decode(&device.transport_public_key)?;
    if transport.len() != 32 {
        anyhow::bail!("invalid transport public key in RC Lock mesh authority");
    }
    Ok(TrustedMeshDevice {
        device_id: device.id,
        peer_id,
        identity_public_key: device.identity_public_key,
        transport_public_key: device.transport_public_key,
    })
}
