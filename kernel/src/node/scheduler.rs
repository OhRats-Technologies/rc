use super::{process::wire, values};
use crate::{descriptor::SelectionMode, service::ServiceRegistry};
use rc_node::{
    ProcessChannel, ProcessEnvironment, ProcessExecutionMode, ProcessLifetime, ProcessPrincipal,
    ProcessSpec,
};
use semver::VersionReq;
use std::{
    collections::{HashMap, HashSet},
    path::PathBuf,
    sync::Arc,
    time::Duration,
};
use tokio::sync::Notify;
use wasmtime::component::Val;

const SERVICE: &str = "ohrats:rc-scheduler/driver";
mod management;

#[derive(Clone)]
pub struct ComponentScheduler {
    registry: ServiceRegistry,
    requirement: VersionReq,
    wake: Arc<Notify>,
}

pub struct Trigger {
    pub schedule_id: String,
    pub occurrence_id: String,
    pub mode: ProcessExecutionMode,
    pub cwd: Option<String>,
    pub environment: ProcessEnvironment,
    pub max_runtime_ms: Option<u64>,
    pub permit_hash: String,
}

impl ComponentScheduler {
    pub fn new(registry: ServiceRegistry) -> anyhow::Result<Self> {
        Ok(Self {
            registry,
            requirement: VersionReq::parse("^0.1")?,
            wake: Arc::new(Notify::new()),
        })
    }

    pub fn available(&self) -> anyhow::Result<bool> {
        Ok(self
            .registry
            .has_provider(SERVICE, &self.requirement, None)?)
    }

    pub fn tick(
        &self,
        now_ms: u64,
        recovering: bool,
        active: Vec<String>,
    ) -> Result<(Vec<Trigger>, Option<u64>), String> {
        let values = self
            .registry
            .call_one(
                SERVICE,
                &self.requirement,
                SelectionMode::Single,
                "tick",
                &[
                    Val::U64(now_ms),
                    Val::Bool(recovering),
                    Val::List(active.into_iter().map(Val::String).collect()),
                ],
            )
            .map_err(display)?;
        let result = values::record(
            values::result_value(values, "scheduler tick")?,
            "scheduler tick result",
        )?;
        let triggers = values::list(values::field(&result, "triggers")?.clone(), "triggers")?
            .into_iter()
            .map(parse_trigger)
            .collect::<Result<Vec<_>, _>>()?;
        let next = wire::option_u64_field(&result, "next-wake-unix-ms")?;
        Ok((triggers, next))
    }

    fn notify(&self) {
        self.wake.notify_one();
    }
}

impl Trigger {
    pub fn process_spec(self, user_id: String, max_runtime_ms: Option<u64>) -> ProcessSpec {
        let mut spec = ProcessSpec::command(
            &format!("schedule:{}:{}", self.schedule_id, self.occurrence_id),
            "",
        );
        spec.mode = self.mode;
        spec.cwd = self.cwd.unwrap_or_default();
        spec.environment = self.environment;
        spec.channel = ProcessChannel::Schedule;
        spec.lifetime = ProcessLifetime::Scheduled;
        spec.authorization_id = self.schedule_id.clone();
        spec.user_id = user_id.clone();
        spec.principal = ProcessPrincipal {
            user_id,
            role: "owner".into(),
            can_execute: true,
            can_manage_devices: false,
        };
        spec.max_runtime_ms = max_runtime_ms;
        spec
    }
}

fn parse_trigger(value: Val) -> Result<Trigger, String> {
    let fields = values::record(value, "scheduler trigger")?;
    let occurrence = values::record(values::field(&fields, "occurrence")?.clone(), "occurrence")?;
    let schedule = values::record(values::field(&fields, "schedule")?.clone(), "schedule")?;
    Ok(Trigger {
        schedule_id: values::string_field(&schedule, "id")?,
        occurrence_id: values::string_field(&occurrence, "id")?,
        mode: wire::mode_field(&schedule, "mode")?,
        cwd: wire::option_string_field(&schedule, "cwd")?,
        environment: wire::environment_field(&schedule, "environment")?,
        max_runtime_ms: wire::option_u64_field(&schedule, "max-runtime-ms")?,
        permit_hash: values::string_field(&schedule, "permit-hash")?,
    })
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}

pub async fn run(
    scheduler: ComponentScheduler,
    manager: Arc<dyn rc_node::ExecutionManager>,
    state_dir: PathBuf,
    device_id: String,
) {
    let mut recovering = true;
    let mut running = HashMap::<String, (String, String)>::new();
    loop {
        let active: HashSet<_> = manager.active_ids().into_iter().collect();
        let revoked = reconcile_running(&mut running, &active, |schedule_id, hash| {
            rc_node::schedule_authority(&state_dir, schedule_id, &device_id, hash).is_ok()
        });
        for process_id in revoked {
            let _ = manager.signal(&process_id, "KILL");
        }
        let active_schedules = running.values().map(|(id, _)| id.clone()).collect();
        let now = now_ms();
        let mut next_wake = None;
        match scheduler.tick(now, recovering, active_schedules) {
            Ok((triggers, next)) => {
                next_wake = next;
                for trigger in triggers {
                    let Ok(grant) = rc_node::schedule_authority(
                        &state_dir,
                        &trigger.schedule_id,
                        &device_id,
                        &trigger.permit_hash,
                    ) else {
                        continue;
                    };
                    let requested = trigger.max_runtime_ms;
                    if requested.is_some_and(|value| {
                        grant.max_runtime_ms != 0 && value > grant.max_runtime_ms
                    }) {
                        continue;
                    }
                    let schedule_id = trigger.schedule_id.clone();
                    let hash = trigger.permit_hash.clone();
                    let max =
                        bounded_runtime_ms(requested, grant.max_runtime_ms, grant.expires_at, now);
                    let spec = trigger.process_spec(grant.user_id, max);
                    let process_id = spec.id.clone();
                    if manager.start(spec).unwrap_or(false) {
                        running.insert(process_id, (schedule_id, hash));
                    }
                }
            }
            Err(error) => eprintln!("scheduler tick failed: {error}"),
        }
        recovering = false;
        let fallback = if running.is_empty() {
            Duration::from_secs(24 * 60 * 60)
        } else {
            Duration::from_secs(30)
        };
        let wait = next_wake
            .map(|deadline| Duration::from_millis(deadline.saturating_sub(now).max(1)))
            .unwrap_or(fallback);
        tokio::select! {
            _ = tokio::time::sleep(wait) => {}
            _ = scheduler.wake.notified() => {}
        }
    }
}

fn reconcile_running(
    running: &mut HashMap<String, (String, String)>,
    active: &HashSet<String>,
    mut authorized: impl FnMut(&str, &str) -> bool,
) -> Vec<String> {
    let mut revoked = Vec::new();
    running.retain(|process_id, (schedule_id, hash)| {
        if !active.contains(process_id) {
            return false;
        }
        if !authorized(schedule_id, hash) {
            revoked.push(process_id.clone());
            return false;
        }
        true
    });
    revoked
}

fn bounded_runtime_ms(
    requested: Option<u64>,
    permit_max: u64,
    permit_expires_at: i64,
    now_ms: u64,
) -> Option<u64> {
    let permit = (permit_max != 0).then_some(permit_max);
    let expiry = (permit_expires_at != 0).then(|| {
        u64::try_from(permit_expires_at)
            .unwrap_or_default()
            .saturating_sub(now_ms)
    });
    [requested, permit, expiry].into_iter().flatten().min()
}

fn now_ms() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as u64
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn reconciliation_drops_finished_and_kills_revoked_runs() {
        let mut running = HashMap::from([
            ("finished".into(), ("old".into(), "hash-old".into())),
            ("revoked".into(), ("nightly".into(), "hash-bad".into())),
            ("allowed".into(), ("hourly".into(), "hash-good".into())),
        ]);
        let active = HashSet::from(["revoked".into(), "allowed".into()]);
        let revoked = reconcile_running(&mut running, &active, |id, hash| {
            id == "hourly" && hash == "hash-good"
        });

        assert_eq!(revoked, vec!["revoked"]);
        assert_eq!(running.len(), 1);
        assert!(running.contains_key("allowed"));
    }

    #[test]
    fn scheduled_runtime_cannot_outlive_permit_or_expiry() {
        assert_eq!(
            bounded_runtime_ms(Some(8_000), 5_000, 20_000, 1_000),
            Some(5_000)
        );
        assert_eq!(
            bounded_runtime_ms(Some(8_000), 0, 4_000, 1_000),
            Some(3_000)
        );
        assert_eq!(bounded_runtime_ms(None, 5_000, 0, 1_000), Some(5_000));
        assert_eq!(bounded_runtime_ms(None, 0, 0, 1_000), None);
        assert_eq!(bounded_runtime_ms(None, 0, 999, 1_000), Some(0));
    }
}
