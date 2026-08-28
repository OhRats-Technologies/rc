use crate::ohrats::rc_workspaces::types::{Invitation, IssuedInvitation, Membership, Role};
use crate::{
    membership,
    model::{StoredInvitation, StoredMembership, StoredRole},
    storage,
    util::*,
};

const MAX_LIFETIME_MS: u64 = 30 * 24 * 60 * 60 * 1000;

pub fn issue(
    actor: &str,
    workspace: &str,
    role: Role,
    expires: u64,
) -> Result<IssuedInvitation, String> {
    valid_id(actor, "actor id")?;
    valid_id(workspace, "workspace id")?;
    if role == Role::Owner {
        return Err("invitations cannot grant Owner role".into());
    }
    let now = now_ms();
    if expires <= now || expires > now.saturating_add(MAX_LIFETIME_MS) {
        return Err("invitation expiry is outside the allowed range".into());
    }
    let token = token()?;
    let hash = token_key(&token).expect("generated token is valid");
    let invitation_id = id(&[b"invite", workspace.as_bytes(), &hash]);
    storage::commit(|| {
        membership::require_owner(workspace, actor)?;
        if storage::scan(INVITE_IDS, &prefix(workspace), MAX_INVITATIONS)?.len() >= MAX_INVITATIONS
        {
            return Err("workspace invitation capacity reached".into());
        }
        let value = StoredInvitation {
            id: invitation_id.clone(),
            workspace_id: workspace.into(),
            role: role.into(),
            created_by: actor.into(),
            created_at_ms: now,
            expires_at_ms: expires,
        };
        Ok((
            IssuedInvitation {
                invitation: value.clone().into(),
                token: token.clone(),
            },
            vec![
                storage::put(INVITES, hash.clone(), encode(&value)?),
                storage::put(INVITE_IDS, pair(workspace, &invitation_id), hash.clone()),
            ],
        ))
    })
}

pub fn inspect(token: &str) -> Result<Option<Invitation>, String> {
    let Some(hash) = token_key(token) else {
        return Ok(None);
    };
    let Some(bytes) = storage::get(INVITES, &hash)? else {
        return Ok(None);
    };
    let value: StoredInvitation = decode(&bytes)?;
    if value.expires_at_ms <= now_ms() {
        expire(&value, hash)?;
        return Ok(None);
    }
    Ok(Some(value.into()))
}

pub fn consume(token: &str, user: &str) -> Result<Option<Membership>, String> {
    valid_id(user, "user id")?;
    let Some(hash) = token_key(token) else {
        return Ok(None);
    };
    storage::commit(|| {
        let Some(bytes) = storage::get(INVITES, &hash)? else {
            return Ok((None, Vec::new()));
        };
        let invitation: StoredInvitation = decode(&bytes)?;
        let mut changes = removal(&invitation, hash.clone());
        if invitation.expires_at_ms <= now_ms() {
            return Ok((None, changes));
        }
        let existing = membership::get(&invitation.workspace_id, user)?;
        if existing.is_none() {
            if storage::scan(MEMBERS, &prefix(&invitation.workspace_id), MAX_MEMBERS)?.len()
                >= MAX_MEMBERS
            {
                return Err("workspace member capacity reached".into());
            }
            crate::directory::ensure_user_capacity(user)?;
        }
        let member = StoredMembership {
            workspace_id: invitation.workspace_id.clone(),
            user_id: user.into(),
            role: existing
                .as_ref()
                .map_or(invitation.role, |m| stronger(m.role, invitation.role)),
            created_at_ms: existing.map_or_else(now_ms, |m| m.created_at_ms),
        };
        let encoded = encode(&member)?;
        changes.push(storage::put(
            MEMBERS,
            pair(&member.workspace_id, user),
            encoded.clone(),
        ));
        changes.push(storage::put(
            USER_MEMBERS,
            pair(user, &member.workspace_id),
            encoded,
        ));
        Ok((Some(member.into()), changes))
    })
}

pub fn revoke(actor: &str, workspace: &str, invitation_id: &str) -> Result<bool, String> {
    valid_id(actor, "actor id")?;
    valid_id(workspace, "workspace id")?;
    valid_id(invitation_id, "invitation id")?;
    storage::commit(|| {
        membership::require_owner(workspace, actor)?;
        let index = pair(workspace, invitation_id);
        let Some(hash) = storage::get(INVITE_IDS, &index)? else {
            return Ok((false, Vec::new()));
        };
        Ok((
            true,
            vec![
                storage::delete(INVITES, hash),
                storage::delete(INVITE_IDS, index),
            ],
        ))
    })
}

fn expire(value: &StoredInvitation, hash: Vec<u8>) -> Result<(), String> {
    storage::commit(|| Ok(((), removal(value, hash.clone()))))
}
fn removal(
    value: &StoredInvitation,
    hash: Vec<u8>,
) -> Vec<crate::ohrats::rc_storage::types::Change> {
    vec![
        storage::delete(INVITES, hash),
        storage::delete(INVITE_IDS, pair(&value.workspace_id, &value.id)),
    ]
}
fn stronger(a: StoredRole, b: StoredRole) -> StoredRole {
    if rank(a) >= rank(b) { a } else { b }
}
fn rank(role: StoredRole) -> u8 {
    match role {
        StoredRole::Viewer => 0,
        StoredRole::Operator => 1,
        StoredRole::Owner => 2,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
    #[test]
    fn malformed_tokens_have_no_lookup_key() {
        assert!(token_key("secret").is_none());
    }
    #[test]
    fn generated_tokens_are_256_bits() {
        let value = token().unwrap();
        assert_eq!(URL_SAFE_NO_PAD.decode(value).unwrap().len(), 32);
    }
    #[test]
    fn roles_never_downgrade_on_invite() {
        assert_eq!(rank(stronger(StoredRole::Operator, StoredRole::Viewer)), 1);
    }
}
