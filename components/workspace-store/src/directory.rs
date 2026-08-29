use crate::ohrats::rc_workspaces::types::{Access, Workspace};
use crate::{
    membership,
    model::{StoredMembership, StoredRole, StoredWorkspace, access},
    storage,
    util::*,
};

pub fn ensure_personal(user: &str) -> Result<Workspace, String> {
    valid_id(user, "user id")?;
    storage::commit(|| {
        if let Some(id) = storage::get(PERSONAL, user.as_bytes())? {
            let value = storage::get(WORKSPACES, &id)?
                .ok_or_else(|| "personal workspace index is corrupt".to_owned())?;
            let stored: StoredWorkspace = decode(&value)?;
            return Ok((
                stored.clone().into(),
                vec![storage::put(WORKSPACES, id, value)],
            ));
        }
        ensure_user_capacity(user)?;
        let id = id(&[b"personal", user.as_bytes()]);
        let workspace = StoredWorkspace {
            id: id.clone(),
            name: "Personal".into(),
            personal_for: Some(user.into()),
            created_at_ms: now_ms(),
        };
        let member = StoredMembership {
            workspace_id: id.clone(),
            user_id: user.into(),
            role: StoredRole::Owner,
            created_at_ms: workspace.created_at_ms,
        };
        let member_bytes = encode(&member)?;
        Ok((
            workspace.clone().into(),
            vec![
                storage::put(WORKSPACES, id.as_bytes().to_vec(), encode(&workspace)?),
                storage::put(PERSONAL, user.as_bytes().to_vec(), id.as_bytes().to_vec()),
                storage::put(MEMBERS, pair(&id, user), member_bytes.clone()),
                storage::put(USER_MEMBERS, pair(user, &id), member_bytes),
            ],
        ))
    })
}

pub fn create(actor: &str, name: String) -> Result<Workspace, String> {
    valid_id(actor, "actor id")?;
    valid_name(&name)?;
    let nonce = random(32)?;
    let workspace_id = id(&[b"workspace", actor.as_bytes(), name.as_bytes(), &nonce]);
    storage::commit(|| {
        ensure_user_capacity(actor)?;
        let workspace = StoredWorkspace {
            id: workspace_id.clone(),
            name: name.clone(),
            personal_for: None,
            created_at_ms: now_ms(),
        };
        let member = StoredMembership {
            workspace_id: workspace_id.clone(),
            user_id: actor.into(),
            role: StoredRole::Owner,
            created_at_ms: workspace.created_at_ms,
        };
        let bytes = encode(&member)?;
        Ok((
            workspace.clone().into(),
            vec![
                storage::put(
                    WORKSPACES,
                    workspace_id.as_bytes().to_vec(),
                    encode(&workspace)?,
                ),
                storage::put(MEMBERS, pair(&workspace_id, actor), bytes.clone()),
                storage::put(USER_MEMBERS, pair(actor, &workspace_id), bytes),
            ],
        ))
    })
}

pub fn get(id: &str) -> Result<Option<Workspace>, String> {
    valid_id(id, "workspace id")?;
    storage::get(WORKSPACES, id.as_bytes())?
        .map(|v| decode::<StoredWorkspace>(&v).map(Into::into))
        .transpose()
}

pub fn for_user(user: &str) -> Result<Vec<Access>, String> {
    valid_id(user, "user id")?;
    storage::scan(USER_MEMBERS, &prefix(user), MAX_USER_WORKSPACES)?
        .into_iter()
        .map(|e| {
            let member: StoredMembership = decode(&e.value)?;
            let bytes = storage::get(WORKSPACES, member.workspace_id.as_bytes())?
                .ok_or_else(|| "membership references missing workspace".to_owned())?;
            Ok(access(decode(&bytes)?, member))
        })
        .collect()
}

pub fn rename(actor: &str, workspace: &str, name: String) -> Result<Workspace, String> {
    valid_id(actor, "actor id")?;
    valid_id(workspace, "workspace id")?;
    valid_name(&name)?;
    storage::commit(|| {
        membership::require_owner(workspace, actor)?;
        let bytes = storage::get(WORKSPACES, workspace.as_bytes())?
            .ok_or_else(|| "workspace not found".to_owned())?;
        let mut value: StoredWorkspace = decode(&bytes)?;
        value.name = name.clone();
        Ok((
            value.clone().into(),
            vec![storage::put(
                WORKSPACES,
                workspace.as_bytes().to_vec(),
                encode(&value)?,
            )],
        ))
    })
}

pub fn delete(actor: &str, workspace: &str) -> Result<bool, String> {
    valid_id(actor, "actor id")?;
    valid_id(workspace, "workspace id")?;
    storage::commit(|| {
        membership::require_owner(workspace, actor)?;
        let Some(bytes) = storage::get(WORKSPACES, workspace.as_bytes())? else {
            return Ok((false, Vec::new()));
        };
        let value: StoredWorkspace = decode(&bytes)?;
        let mut changes = vec![storage::delete(WORKSPACES, workspace.as_bytes().to_vec())];
        if let Some(user) = value.personal_for {
            changes.push(storage::delete(PERSONAL, user.into_bytes()));
        }
        for e in storage::scan(MEMBERS, &prefix(workspace), MAX_MEMBERS)? {
            let m: StoredMembership = decode(&e.value)?;
            changes.push(storage::delete(MEMBERS, e.key));
            changes.push(storage::delete(USER_MEMBERS, pair(&m.user_id, workspace)));
        }
        for e in storage::scan(INVITE_IDS, &prefix(workspace), MAX_INVITATIONS)? {
            if let Some(hash) = storage::get(INVITE_IDS, &e.key)? {
                changes.push(storage::delete(INVITES, hash));
            }
            changes.push(storage::delete(INVITE_IDS, e.key));
        }
        Ok((true, changes))
    })
}

pub(crate) fn ensure_user_capacity(user: &str) -> Result<(), String> {
    if storage::scan(USER_MEMBERS, &prefix(user), MAX_USER_WORKSPACES)?.len() >= MAX_USER_WORKSPACES
    {
        Err("user workspace capacity reached".into())
    } else {
        Ok(())
    }
}
