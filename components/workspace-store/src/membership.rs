use crate::ohrats::rc_storage::types::Change;
use crate::ohrats::rc_workspaces::types::{Membership, Role};
use crate::{
    model::{StoredMembership, StoredRole},
    storage,
    util::*,
};

pub fn get(workspace: &str, user: &str) -> Result<Option<StoredMembership>, String> {
    storage::get(MEMBERS, &pair(workspace, user))?
        .map(|v| decode(&v))
        .transpose()
}

pub fn require_owner(workspace: &str, actor: &str) -> Result<(), String> {
    match get(workspace, actor)?.map(|m| m.role) {
        Some(StoredRole::Owner) => Ok(()),
        _ => Err("Owner authorization required".into()),
    }
}

pub fn list(workspace: &str) -> Result<Vec<Membership>, String> {
    valid_id(workspace, "workspace id")?;
    storage::scan(MEMBERS, &prefix(workspace), MAX_MEMBERS)?
        .into_iter()
        .map(|e| decode::<StoredMembership>(&e.value).map(Into::into))
        .collect()
}

pub fn role_for(workspace: &str, user: &str) -> Result<Option<Role>, String> {
    valid_id(workspace, "workspace id")?;
    valid_id(user, "user id")?;
    Ok(get(workspace, user)?.map(|m| m.role.into()))
}

pub fn change(actor: &str, workspace: &str, user: &str, role: Role) -> Result<Membership, String> {
    valid_id(actor, "actor id")?;
    valid_id(workspace, "workspace id")?;
    valid_id(user, "user id")?;
    storage::commit(|| {
        require_owner(workspace, actor)?;
        let current = get(workspace, user)?;
        if current.is_none() {
            if storage::scan(MEMBERS, &prefix(workspace), MAX_MEMBERS)?.len() >= MAX_MEMBERS {
                return Err("workspace member capacity reached".into());
            }
            crate::directory::ensure_user_capacity(user)?;
        }
        if current
            .as_ref()
            .is_some_and(|m| m.role == StoredRole::Owner)
            && role != Role::Owner
            && owners(workspace)? == 1
        {
            return Err("workspace must retain at least one Owner".into());
        }
        let value = StoredMembership {
            workspace_id: workspace.into(),
            user_id: user.into(),
            role: role.into(),
            created_at_ms: current.map_or_else(now_ms, |m| m.created_at_ms),
        };
        let bytes = encode(&value)?;
        Ok((
            value.clone().into(),
            vec![
                storage::put(MEMBERS, pair(workspace, user), bytes.clone()),
                storage::put(USER_MEMBERS, pair(user, workspace), bytes),
            ],
        ))
    })
}

pub fn remove(actor: &str, workspace: &str, user: &str) -> Result<bool, String> {
    valid_id(actor, "actor id")?;
    valid_id(workspace, "workspace id")?;
    valid_id(user, "user id")?;
    storage::commit(|| {
        require_owner(workspace, actor)?;
        remove_changes(workspace, user)
    })
}

pub fn leave(user: &str, workspace: &str) -> Result<bool, String> {
    valid_id(user, "user id")?;
    valid_id(workspace, "workspace id")?;
    storage::commit(|| remove_changes(workspace, user))
}

fn remove_changes(workspace: &str, user: &str) -> Result<(bool, Vec<Change>), String> {
    let Some(current) = get(workspace, user)? else {
        return Ok((false, Vec::new()));
    };
    if current.role == StoredRole::Owner && owners(workspace)? == 1 {
        return Err("workspace must retain at least one Owner".into());
    }
    Ok((
        true,
        vec![
            storage::delete(MEMBERS, pair(workspace, user)),
            storage::delete(USER_MEMBERS, pair(user, workspace)),
        ],
    ))
}

fn owners(workspace: &str) -> Result<usize, String> {
    storage::scan(MEMBERS, &prefix(workspace), MAX_MEMBERS)?
        .into_iter()
        .map(|e| decode::<StoredMembership>(&e.value))
        .try_fold(0, |n, m| Ok(n + usize::from(m?.role == StoredRole::Owner)))
}
