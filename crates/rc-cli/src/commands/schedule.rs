mod authority;
use self::authority::{create_permit, current_user, pop, sync_authority};
use super::remote::remote_device;
use crate::{ScheduleCommand, control_client::RemoteControl};
use anyhow::{Context as _, Result, bail};
use rc_api_client::{ApiClient, Credential, Device, random_url_bytes};
use rc_node::resolve_state_dir;
use rc_protocol::{
    ControlMessage, EnvironmentSpec, ExecutionMode, ScheduleDefinition, ScheduleMisfirePolicy,
    schedule_spec_hash,
};
use std::time::{SystemTime, UNIX_EPOCH};

pub(super) async fn run(command: ScheduleCommand) -> Result<()> {
    match command {
        ScheduleCommand::List { device, url, token } => list(device, url, token).await,
        ScheduleCommand::Add {
            device,
            cron,
            timezone,
            shell_source,
            max_runtime_seconds,
            url,
            token,
            mut command,
        } => {
            add(Add {
                device,
                cron,
                timezone,
                shell_source,
                max_runtime_seconds,
                url,
                token,
                command: std::mem::take(&mut command),
            })
            .await
        }
        ScheduleCommand::Remove {
            device,
            id,
            url,
            token,
        } => remove(device, id, url, token).await,
        ScheduleCommand::Enable { device, id } => set_enabled(device, id, true).await,
        ScheduleCommand::Disable { device, id } => set_enabled(device, id, false).await,
    }
}

struct Add {
    device: String,
    cron: String,
    timezone: String,
    shell_source: Option<String>,
    max_runtime_seconds: u64,
    url: Option<String>,
    token: Option<String>,
    command: Vec<String>,
}

async fn list(device: String, url: Option<String>, token: Option<String>) -> Result<()> {
    let (api, credential, device) =
        remote_device(url.as_deref(), token.as_deref(), &device).await?;
    let schedules = request(
        &api,
        &credential,
        &device,
        ControlMessage::ScheduleList {
            request_id: request_id(),
        },
    )
    .await?;
    if schedules.is_empty() {
        println!("No schedules.");
    }
    for value in schedules {
        println!(
            "{}\t{}\t{}\t{}",
            value.id,
            if value.enabled { "enabled" } else { "disabled" },
            value.cron,
            value.timezone
        );
    }
    Ok(())
}

async fn add(input: Add) -> Result<()> {
    if input.cron.trim().is_empty() || input.timezone.trim().is_empty() {
        bail!("cron and timezone must not be empty");
    }
    let mode = match (input.shell_source, input.command.as_slice()) {
        (Some(script), []) if !script.trim().is_empty() => ExecutionMode::RcShell { script },
        (None, [program, args @ ..]) => ExecutionMode::Argv {
            program: program.clone(),
            args: args.to_vec(),
        },
        (Some(_), _) => bail!("--shell cannot be combined with exact argv after --"),
        (None, []) => bail!("schedule add requires --shell SCRIPT or -- COMMAND [ARG...]"),
    };
    let (api, credential, device) =
        remote_device(input.url.as_deref(), input.token.as_deref(), &input.device).await?;
    require_scheduler(&device)?;
    let user_id = current_user(&api).await?;
    let max_runtime_ms = input
        .max_runtime_seconds
        .checked_mul(1000)
        .context("max runtime is too large")?;
    let mut schedule = ScheduleDefinition {
        id: random_url_bytes(18),
        name: None,
        cron: input.cron,
        timezone: input.timezone,
        mode,
        cwd: None,
        environment: EnvironmentSpec::default(),
        enabled: true,
        misfire: ScheduleMisfirePolicy::Skip,
        max_runtime_ms: Some(max_runtime_ms),
        permit_hash: String::new(),
        created_by: user_id,
        created_at_ms: now_ms(),
        expires_at_ms: None,
    };
    schedule.permit_hash = schedule_spec_hash(&schedule);
    create_permit(&api, &credential, &device, &schedule).await?;
    sync_authority(&api, &credential, &device.workspace_id).await?;
    request(
        &api,
        &credential,
        &device,
        ControlMessage::ScheduleUpsert {
            request_id: request_id(),
            schedule: schedule.clone(),
        },
    )
    .await?;
    println!("Created schedule {} on {}", schedule.id, device.name);
    Ok(())
}

async fn remove(
    device: String,
    id: String,
    url: Option<String>,
    token: Option<String>,
) -> Result<()> {
    let (api, credential, device) =
        remote_device(url.as_deref(), token.as_deref(), &device).await?;
    let client_id = pop(&credential)?.id.clone();
    authority::remove_permit(&api, &device, &id, &client_id).await?;
    sync_authority(&api, &credential, &device.workspace_id).await?;
    request(
        &api,
        &credential,
        &device,
        ControlMessage::ScheduleRemove {
            request_id: request_id(),
            id: id.clone(),
        },
    )
    .await?;
    println!("Removed schedule {id}");
    Ok(())
}

async fn set_enabled(device: String, id: String, enabled: bool) -> Result<()> {
    let (api, credential, device) = remote_device(None, None, &device).await?;
    request(
        &api,
        &credential,
        &device,
        ControlMessage::ScheduleSetEnabled {
            request_id: request_id(),
            id: id.clone(),
            enabled,
        },
    )
    .await?;
    println!(
        "{} schedule {id}",
        if enabled { "Enabled" } else { "Disabled" }
    );
    Ok(())
}

async fn request(
    api: &ApiClient,
    credential: &Credential,
    device: &Device,
    message: ControlMessage,
) -> Result<Vec<ScheduleDefinition>> {
    require_scheduler(device)?;
    let expected = request_id_of(&message).to_owned();
    let mut control =
        RemoteControl::open(api.clone(), credential, device, &resolve_state_dir(None)).await?;
    control.sender.send(&message).await?;
    let result = loop {
        if let ControlMessage::ScheduleResult {
            request_id,
            schedules,
            error,
        } = control.receiver.recv().await?
            && request_id == expected
        {
            if !error.is_empty() {
                break Err(anyhow::anyhow!(error));
            }
            break Ok(schedules);
        }
    };
    control.close().await;
    result
}

fn request_id_of(message: &ControlMessage) -> &str {
    match message {
        ControlMessage::ScheduleList { request_id }
        | ControlMessage::ScheduleUpsert { request_id, .. }
        | ControlMessage::ScheduleRemove { request_id, .. }
        | ControlMessage::ScheduleSetEnabled { request_id, .. } => request_id,
        _ => "",
    }
}

fn require_scheduler(device: &Device) -> Result<()> {
    if !device.supports("scheduler") {
        bail!("RC Node upgrade required: scheduler capability unavailable");
    }
    if device.workspace_id.is_empty() {
        bail!("RC server did not provide the device workspace identity");
    }
    Ok(())
}

fn request_id() -> String {
    random_url_bytes(18)
}
fn now_ms() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as u64
}
