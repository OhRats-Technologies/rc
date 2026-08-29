wit_bindgen::generate!({ path: "../../wit", world: "ssh-policy-fixture", generate_all });

use ohrats::{
    rc_plugin::types::{Command, Requirement, Selection},
    rc_ssh::{
        credentials, policy,
        types::{CommandKind, SessionRequest, WorkspaceRole},
    },
};

struct Fixture;
impl Guest for Fixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:ssh-policy-fixture".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: vec![
                requirement("ohrats:rc-ssh/credentials"),
                requirement("ohrats:rc-ssh/policy"),
            ],
            commands: vec![
                command(
                    "ssh-policy-seed",
                    "Seed SSH policy state",
                    "rc ssh-policy-seed <id> <ed25519-key> <rsa-key>",
                ),
                command(
                    "ssh-policy-verify",
                    "Verify SSH policy state",
                    "rc ssh-policy-verify <id>",
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
            "ssh-policy-seed" => seed(&args),
            "ssh-policy-verify" => verify(&args),
            _ => Err("unsupported command".into()),
        }
    }
}

fn seed(args: &[String]) -> Result<u32, String> {
    let [id, ed, rsa] = args else {
        return Err("usage: ssh-policy-seed <id> <ed25519-key> <rsa-key>".into());
    };
    let user = format!("user-{id}");
    let client = format!("client-{id}");
    let first = credentials::register(&user, &client, "Ed25519", ed, 100)?;
    let second = credentials::register(&user, &client, "RSA", rsa, 101)?;
    if first.algorithm != "ssh-ed25519"
        || second.algorithm != "ssh-rsa"
        || !first.fingerprint.starts_with("SHA256:")
    {
        return Err("key normalization failed".into());
    }
    if credentials::register(&user, &client, "duplicate", ed, 102).is_ok() {
        return Err("duplicate fingerprint accepted".into());
    }
    let mismatch = ed.replacen("ssh-ed25519", "ssh-rsa", 1);
    if credentials::register(&user, &client, "mismatch", &mismatch, 103).is_ok() {
        return Err("embedded algorithm mismatch accepted".into());
    }
    println!("{}", first.id);
    Ok(0)
}

fn verify(args: &[String]) -> Result<u32, String> {
    let [id] = args else {
        return Err("usage: ssh-policy-verify <id>".into());
    };
    let user = format!("user-{id}");
    let client = format!("client-{id}");
    let keys = credentials::list_keys(&user)?;
    if keys.len() != 2 {
        return Err("restart persistence failed".into());
    }
    let key = &keys[1];
    for role in [WorkspaceRole::Operator, WorkspaceRole::Owner] {
        policy::authorize(&request(key.id.clone(), &user, &client, role, "", false))?;
    }
    if policy::authorize(&request(
        key.id.clone(),
        &user,
        &client,
        WorkspaceRole::Viewer,
        "",
        false,
    ))
    .is_ok()
    {
        return Err("viewer authorized".into());
    }
    let sftp = policy::authorize(&request(
        key.id.clone(),
        &user,
        &client,
        WorkspaceRole::Owner,
        "internal-sftp",
        false,
    ))?;
    let scp = policy::authorize(&request(
        key.id.clone(),
        &user,
        &client,
        WorkspaceRole::Owner,
        "scp -t /tmp/x",
        false,
    ))?;
    let rsync = policy::authorize(&request(
        key.id.clone(),
        &user,
        &client,
        WorkspaceRole::Owner,
        "rsync --server -logDtpre.iLsfxCIvu . /tmp",
        false,
    ))?;
    if !matches!(sftp.command_kind, CommandKind::Sftp)
        || !matches!(scp.command_kind, CommandKind::Scp)
        || !matches!(rsync.command_kind, CommandKind::Rsync)
    {
        return Err("command mapping failed".into());
    }
    let mut forbidden = request(
        key.id.clone(),
        &user,
        &client,
        WorkspaceRole::Owner,
        "",
        false,
    );
    forbidden.port_forwarding = true;
    if policy::authorize(&forbidden).is_ok() {
        return Err("forwarding accepted".into());
    }
    let mut renamed = request(
        key.id.clone(),
        &user,
        &client,
        WorkspaceRole::Owner,
        "",
        false,
    );
    renamed.device_id = "friendly device name".into();
    if policy::authorize(&renamed).is_ok() {
        return Err("mutable device route accepted".into());
    }
    let parts: Vec<_> = key.normalized.split_whitespace().collect();
    let line = policy::authorized_key_line(parts[0], parts[1])?.ok_or("authorized key missing")?;
    for option in [
        "no-agent-forwarding",
        "no-port-forwarding",
        "no-X11-forwarding",
        "no-user-rc",
    ] {
        if !line.contains(option) {
            return Err("forced key options incomplete".into());
        }
    }
    if !credentials::revoke(&key.id, &user)?
        || policy::authorize(&request(
            key.id.clone(),
            &user,
            &client,
            WorkspaceRole::Owner,
            "",
            false,
        ))
        .is_ok()
    {
        return Err("revocation failed".into());
    }
    println!("ssh policy state: ok");
    Ok(0)
}

fn request(
    key: String,
    user: &str,
    client: &str,
    role: WorkspaceRole,
    command: &str,
    forwarding: bool,
) -> SessionRequest {
    SessionRequest {
        key_id: key,
        user_id: user.into(),
        control_client_id: client.into(),
        control_client_expires_at_ms: 0,
        device_id: "0192f7aa-7e6e-7000-8000-000000000001".into(),
        workspace_role: role,
        original_command: command.into(),
        terminal: false,
        agent_forwarding: forwarding,
        port_forwarding: false,
        x11_forwarding: false,
        tunnel: false,
        requested_at_ms: 1000,
    }
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
export!(Fixture);
