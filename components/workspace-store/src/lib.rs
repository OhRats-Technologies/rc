wit_bindgen::generate!({ path: "../../wit", world: "workspace-store", generate_all });

mod directory;
mod invitation;
mod membership;
mod model;
mod storage;
mod util;

use exports::ohrats::rc_workspaces::{
    directory::Guest as DirectoryGuest, invitations::Guest as InvitationsGuest,
    memberships::Guest as MembershipsGuest,
};
use ohrats::{
    rc_plugin::types::Service,
    rc_workspaces::types::{Access, Invitation, IssuedInvitation, Membership, Role, Workspace},
};

struct WorkspaceStore;

impl Guest for WorkspaceStore {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:workspace-store".into(),
            version: "0.1.0".into(),
            provides: vec![
                service("ohrats:rc-workspaces/directory"),
                service("ohrats:rc-workspaces/memberships"),
                service("ohrats:rc-workspaces/invitations"),
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

impl DirectoryGuest for WorkspaceStore {
    fn ensure_personal(user_id: String) -> Result<Workspace, String> {
        directory::ensure_personal(&user_id)
    }
    fn create(actor_id: String, name: String) -> Result<Workspace, String> {
        directory::create(&actor_id, name)
    }
    fn get(id: String) -> Result<Option<Workspace>, String> {
        directory::get(&id)
    }
    fn for_user(user_id: String) -> Result<Vec<Access>, String> {
        directory::for_user(&user_id)
    }
    fn rename(actor_id: String, id: String, name: String) -> Result<Workspace, String> {
        directory::rename(&actor_id, &id, name)
    }
    fn delete(actor_id: String, id: String) -> Result<bool, String> {
        directory::delete(&actor_id, &id)
    }
}
impl MembershipsGuest for WorkspaceStore {
    fn role_for(workspace_id: String, user_id: String) -> Result<Option<Role>, String> {
        membership::role_for(&workspace_id, &user_id)
    }
    fn list_members(workspace_id: String) -> Result<Vec<Membership>, String> {
        membership::list(&workspace_id)
    }
    fn change_role(
        actor_id: String,
        workspace_id: String,
        user_id: String,
        role: Role,
    ) -> Result<Membership, String> {
        membership::change(&actor_id, &workspace_id, &user_id, role)
    }
    fn remove(actor_id: String, workspace_id: String, user_id: String) -> Result<bool, String> {
        membership::remove(&actor_id, &workspace_id, &user_id)
    }
    fn leave(user_id: String, workspace_id: String) -> Result<bool, String> {
        membership::leave(&user_id, &workspace_id)
    }
}
impl InvitationsGuest for WorkspaceStore {
    fn issue(
        actor_id: String,
        workspace_id: String,
        role: Role,
        expires_at_ms: u64,
    ) -> Result<IssuedInvitation, String> {
        invitation::issue(&actor_id, &workspace_id, role, expires_at_ms)
    }
    fn inspect(token: String) -> Result<Option<Invitation>, String> {
        invitation::inspect(&token)
    }
    fn consume(token: String, user_id: String) -> Result<Option<Membership>, String> {
        invitation::consume(&token, &user_id)
    }
    fn revoke(
        actor_id: String,
        workspace_id: String,
        invitation_id: String,
    ) -> Result<bool, String> {
        invitation::revoke(&actor_id, &workspace_id, &invitation_id)
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
export!(WorkspaceStore);
