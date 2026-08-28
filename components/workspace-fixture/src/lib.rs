wit_bindgen::generate!({ path: "../../wit", world: "workspace-fixture", generate_all });

use ohrats::{
    rc_plugin::types::{Command, Requirement, Selection},
    rc_workspaces::{directory, invitations, memberships, types::Role},
};
use std::{
    thread,
    time::{Duration, SystemTime, UNIX_EPOCH},
};

struct WorkspaceFixture;

impl Guest for WorkspaceFixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:workspace-fixture".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: vec![
                requirement("ohrats:rc-workspaces/directory"),
                requirement("ohrats:rc-workspaces/memberships"),
                requirement("ohrats:rc-workspaces/invitations"),
            ],
            commands: vec![
                command(
                    "workspace-seed",
                    "Create workspace smoke state",
                    "rc workspace-seed <id>",
                ),
                command(
                    "workspace-verify",
                    "Verify workspace smoke state",
                    "rc workspace-verify <id> <token>",
                ),
            ],
        }
    }
    fn activate() -> Result<(), String> {
        Ok(())
    }
    fn deactivate() {}
    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        match command.as_str() {
            "workspace-seed" => seed(&args),
            "workspace-verify" => verify(&args),
            _ => Err(format!("unsupported command {command:?}")),
        }
    }
}

fn seed(args: &[String]) -> Result<u32, String> {
    let [fixture] = args else {
        return Err("usage: rc workspace-seed <id>".into());
    };
    validate(fixture)?;
    let owner = user(fixture, "owner");
    let personal = directory::ensure_personal(&owner)?;
    if directory::ensure_personal(&owner)?.id != personal.id
        || personal.name != "Personal"
        || personal.personal_for.as_deref() != Some(&owner)
    {
        return Err("Personal workspace was not idempotent".into());
    }
    if memberships::remove(&owner, &personal.id, &owner).is_ok()
        || memberships::leave(&owner, &personal.id).is_ok()
    {
        return Err("sole Personal Owner could be removed".into());
    }
    let workspace = directory::create(&owner, &format!("Fixture {fixture}"))?;
    let operator = user(fixture, "operator");
    memberships::change_role(&owner, &workspace.id, &operator, Role::Operator)?;
    let leaver = user(fixture, "leaver");
    memberships::change_role(&owner, &workspace.id, &leaver, Role::Viewer)?;
    if !memberships::leave(&leaver, &workspace.id)?
        || memberships::role_for(&workspace.id, &leaver)?.is_some()
        || directory::for_user(&leaver)?
            .iter()
            .any(|access| access.workspace.id == workspace.id)
    {
        return Err("membership leave did not clean both indexes".into());
    }
    let downgrade = invitations::issue(
        &owner,
        &workspace.id,
        Role::Viewer,
        now().saturating_add(60_000),
    )?;
    if invitations::consume(&downgrade.token, &operator)?
        .is_none_or(|member| member.role != Role::Operator)
    {
        return Err("invitation downgraded an existing role".into());
    }
    let issued = invitations::issue(
        &owner,
        &workspace.id,
        Role::Viewer,
        now().saturating_add(60_000),
    )?;
    if invitations::issue(
        &owner,
        &workspace.id,
        Role::Owner,
        now().saturating_add(60_000),
    )
    .is_ok()
    {
        return Err("invitation granted Owner role".into());
    }
    let expiring = invitations::issue(
        &owner,
        &workspace.id,
        Role::Viewer,
        now().saturating_add(100),
    )?;
    thread::sleep(Duration::from_millis(150));
    if invitations::consume(&expiring.token, &user(fixture, "expired"))?.is_some()
        || invitations::inspect(&expiring.token)?.is_some()
    {
        return Err("expired invitation remained usable".into());
    }
    let revoked = invitations::issue(
        &owner,
        &workspace.id,
        Role::Viewer,
        now().saturating_add(60_000),
    )?;
    if !invitations::revoke(&owner, &workspace.id, &revoked.invitation.id)?
        || invitations::inspect(&revoked.token)?.is_some()
    {
        return Err("invitation revocation failed".into());
    }
    println!("{}:{}", workspace.id, issued.token);
    Ok(0)
}

fn verify(args: &[String]) -> Result<u32, String> {
    let [fixture, payload] = args else {
        return Err("usage: rc workspace-verify <id> <workspace:token>".into());
    };
    validate(fixture)?;
    let (workspace_id, token) = payload
        .split_once(':')
        .ok_or_else(|| "invalid fixture payload".to_owned())?;
    let owner = user(fixture, "owner");
    let operator = user(fixture, "operator");
    let invited = user(fixture, "invited");
    let restored =
        directory::get(workspace_id)?.ok_or_else(|| "workspace was not restored".to_owned())?;
    let personal = directory::ensure_personal(&owner)?;
    if personal.personal_for.as_deref() != Some(&owner)
        || directory::ensure_personal(&owner)?.id != personal.id
    {
        return Err("Personal workspace was not restored idempotently".into());
    }
    if restored.name != format!("Fixture {fixture}")
        || memberships::role_for(workspace_id, &operator)? != Some(Role::Operator)
    {
        return Err("workspace membership was not restored".into());
    }
    if directory::for_user(&operator)?
        .iter()
        .all(|a| a.workspace.id != workspace_id || a.role != Role::Operator)
    {
        return Err("workspace access listing omitted operator".into());
    }
    if invitations::inspect(token)?.is_none() {
        return Err("invitation was not restored".into());
    }
    let membership = invitations::consume(token, &invited)?
        .ok_or_else(|| "invitation was not consumed".to_owned())?;
    if membership.role != Role::Viewer
        || invitations::consume(token, &invited)?.is_some()
        || invitations::inspect(token)?.is_some()
    {
        return Err("invitation was not single-use".into());
    }
    if memberships::list_members(workspace_id)?.len() != 3 {
        return Err("membership listing was incomplete".into());
    }
    memberships::change_role(&owner, workspace_id, &invited, Role::Owner)?;
    memberships::change_role(&invited, workspace_id, &owner, Role::Viewer)?;
    if memberships::remove(&invited, workspace_id, &invited).is_ok() {
        return Err("last Owner could be removed".into());
    }
    if !memberships::remove(&invited, workspace_id, &operator)? {
        return Err("operator removal failed".into());
    }
    let renamed = format!("Renamed {fixture}");
    directory::rename(&invited, workspace_id, &renamed)?;
    if directory::rename(&owner, workspace_id, "Unauthorized").is_ok()
        || directory::delete(&owner, workspace_id).is_ok()
    {
        return Err("non-Owner changed workspace".into());
    }
    if !directory::delete(&invited, workspace_id)? || directory::get(workspace_id)?.is_some() {
        return Err("Owner deletion failed".into());
    }
    for user in [&owner, &operator, &invited] {
        if memberships::role_for(workspace_id, user)?.is_some()
            || directory::for_user(user)?
                .iter()
                .any(|access| access.workspace.id == workspace_id)
        {
            return Err("workspace deletion left membership state".into());
        }
    }
    if !memberships::list_members(workspace_id)?.is_empty() {
        return Err("workspace deletion left member rows".into());
    }
    println!("workspace state: ok");
    Ok(0)
}

fn requirement(name: &str) -> Requirement {
    Requirement {
        name: name.into(),
        version: "^0.1".into(),
        selection: Selection::Single,
    }
}
fn command(name: &str, summary: &str, usage: &str) -> Command {
    Command {
        name: name.into(),
        summary: summary.into(),
        usage: usage.into(),
    }
}
fn user(fixture: &str, suffix: &str) -> String {
    format!("fixture-{fixture}-{suffix}")
}
fn now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |v| v.as_millis() as u64)
}
fn validate(value: &str) -> Result<(), String> {
    if !value.is_empty()
        && value.len() <= 40
        && value
            .bytes()
            .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-')
    {
        Ok(())
    } else {
        Err("invalid fixture id".into())
    }
}
export!(WorkspaceFixture);
