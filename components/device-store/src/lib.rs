wit_bindgen::generate!({
    path: "../../wit",
    world: "device-store",
    generate_all,
});

mod enrollment;
mod model;
mod presence;
mod registry;
mod storage;
mod validate;

use exports::ohrats::rc_devices::{
    enrollments::Guest as EnrollmentsGuest, presence::Guest as PresenceGuest,
    registry::Guest as RegistryGuest,
};
use ohrats::{
    rc_devices::types::{
        Device, EnrollmentError, EnrollmentInput, IssuedEnrollment, NodeStatus, NodeUpdate,
        Presence, Tombstone,
    },
    rc_plugin::types::Service,
};

struct DeviceStore;

impl Guest for DeviceStore {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:device-store".into(),
            version: "0.1.0".into(),
            provides: vec![
                service("ohrats:rc-devices/registry"),
                service("ohrats:rc-devices/enrollments"),
                service("ohrats:rc-devices/presence"),
            ],
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

impl RegistryGuest for DeviceStore {
    fn get(id: String) -> Result<Option<Device>, String> {
        registry::get(&id)
    }

    fn all(workspace_id: Option<String>) -> Result<Vec<Device>, String> {
        registry::list(workspace_id.as_deref())
    }

    fn rename(id: String, name: String) -> Result<Device, String> {
        registry::rename(&id, name)
    }

    fn revoke(id: String, revoked_at_ms: u64) -> Result<Option<Tombstone>, String> {
        registry::revoke(&id, revoked_at_ms)
    }

    fn resolve_node(id: String, identity_public_key: String) -> Result<NodeStatus, String> {
        registry::status(&id, &identity_public_key)
    }
}

impl EnrollmentsGuest for DeviceStore {
    fn issue(
        workspace_id: String,
        created_by: String,
        now_ms: u64,
        expires_at_ms: u64,
    ) -> Result<IssuedEnrollment, String> {
        enrollment::issue(workspace_id, created_by, now_ms, expires_at_ms)
    }

    fn consume(
        token: String,
        now_ms: u64,
        input: EnrollmentInput,
    ) -> Result<Device, EnrollmentError> {
        enrollment::consume(token, now_ms, input)
    }
}

impl PresenceGuest for DeviceStore {
    fn renew(
        id: String,
        identity_public_key: String,
        now_ms: u64,
        lease_expires_at_ms: u64,
        update: NodeUpdate,
    ) -> Result<NodeStatus, String> {
        presence::renew(
            &id,
            &identity_public_key,
            now_ms,
            lease_expires_at_ms,
            update,
        )
    }

    fn get(id: String, now_ms: u64) -> Result<Option<Presence>, String> {
        presence::get(&id, now_ms)
    }

    fn expire(now_ms: u64) -> Result<u64, String> {
        presence::expire(now_ms)
    }
}

fn service(name: &str) -> Service {
    Service {
        name: name.into(),
        version: "0.1.0".into(),
        priority: 100,
        keys: Vec::new(),
    }
}

export!(DeviceStore);
