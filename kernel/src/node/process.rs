pub(super) mod wire;

use self::wire::*;
use super::values;
use crate::{descriptor::SelectionMode, service::ServiceRegistry};
use rc_node::{
    ProcessAccessRequest, ProcessAction, ProcessPolicy, ProcessResizeRequest, ProcessSignal,
    ProcessSignalRequest, ProcessStartPlan, ProcessStartRequest, ProcessTerminalSize,
};
use semver::VersionReq;
use wasmtime::component::Val;

const SERVICE: &str = "ohrats:rc-process/policy";

#[derive(Clone)]
pub struct ComponentProcessPolicy {
    registry: ServiceRegistry,
    requirement: VersionReq,
}

impl ComponentProcessPolicy {
    pub fn new(registry: ServiceRegistry) -> anyhow::Result<Self> {
        Ok(Self {
            registry,
            requirement: VersionReq::parse("^0.3")?,
        })
    }

    pub fn available(&self) -> anyhow::Result<bool> {
        Ok(self
            .registry
            .has_provider(SERVICE, &self.requirement, None)?)
    }

    fn call(&self, function: &str, request: Val) -> Result<Vec<Val>, String> {
        self.registry
            .call_one(
                SERVICE,
                &self.requirement,
                SelectionMode::Single,
                function,
                &[request],
            )
            .map_err(|error| error.to_string())
    }
}

impl ProcessPolicy for ComponentProcessPolicy {
    fn authorize_start(&self, request: ProcessStartRequest) -> Result<ProcessStartPlan, String> {
        let values = self.call("authorize-start", start_request(request))?;
        let fields = values::record(
            values::result_value(values, "process policy")?,
            "start plan",
        )?;
        Ok(ProcessStartPlan {
            mode: mode_field(&fields, "mode")?,
            cwd: option_string_field(&fields, "cwd")?,
            environment: environment_field(&fields, "environment")?,
            terminal: terminal_option(&fields)?,
            scrollback_bytes: values::u32_field(&fields, "scrollback-bytes")?,
            stdin_chunk_bytes: values::u32_field(&fields, "stdin-chunk-bytes")?,
            authorization_timeout_ms: values::u32_field(&fields, "authorization-timeout-ms")?,
            terminate_grace_ms: values::u32_field(&fields, "terminate-grace-ms")?,
            reattach_grace_ms: values::u32_field(&fields, "reattach-grace-ms")?,
            max_runtime_ms: option_u64_field(&fields, "max-runtime-ms")?,
        })
    }

    fn authorize_access(&self, request: ProcessAccessRequest) -> Result<(), String> {
        values::unit_result(
            self.call("authorize-access", access_request(request))?,
            "process policy",
        )
    }

    fn normalize_resize(
        &self,
        request: ProcessResizeRequest,
    ) -> Result<ProcessTerminalSize, String> {
        let value = Val::Record(vec![
            ("access".into(), access_request(request.access)),
            ("cols".into(), Val::U16(request.cols)),
            ("rows".into(), Val::U16(request.rows)),
        ]);
        let values = self.call("normalize-resize", value)?;
        let fields = values::record(
            values::result_value(values, "process policy")?,
            "terminal size",
        )?;
        Ok(ProcessTerminalSize {
            cols: values::u16_field(&fields, "cols")?,
            rows: values::u16_field(&fields, "rows")?,
        })
    }

    fn authorize_signal(&self, request: ProcessSignalRequest) -> Result<ProcessSignal, String> {
        let value = Val::Record(vec![
            ("access".into(), access_request(request.access)),
            (
                "signal".into(),
                Val::Enum(signal_name(request.signal).into()),
            ),
        ]);
        match values::result_value(self.call("authorize-signal", value)?, "process policy")? {
            Val::Enum(value) => parse_signal(&value),
            _ => Err("process policy returned a non-signal".into()),
        }
    }
}

pub(super) fn start_request(request: ProcessStartRequest) -> Val {
    Val::Record(vec![
        ("execution-id".into(), Val::String(request.execution_id)),
        ("mode".into(), mode_value(request.mode)),
        ("cwd".into(), option_string(request.cwd)),
        ("environment".into(), environment_value(request.environment)),
        ("terminal".into(), terminal_value(request.terminal)),
        (
            "channel".into(),
            Val::Enum(channel_name(request.channel).into()),
        ),
        (
            "lifetime".into(),
            Val::Enum(lifetime_name(request.lifetime).into()),
        ),
        ("principal".into(), principal_value(request.principal)),
        ("max-runtime-ms".into(), option_u64(request.max_runtime_ms)),
    ])
}

fn access_request(request: ProcessAccessRequest) -> Val {
    Val::Record(vec![
        ("execution-id".into(), Val::String(request.execution_id)),
        ("owner-user-id".into(), Val::String(request.owner_user_id)),
        (
            "action".into(),
            Val::Enum(action_name(request.action).into()),
        ),
        ("principal".into(), principal_value(request.principal)),
    ])
}

fn principal_value(value: rc_node::ProcessPrincipal) -> Val {
    Val::Record(vec![
        ("user-id".into(), Val::String(value.user_id)),
        ("role".into(), Val::String(value.role)),
        ("can-execute".into(), Val::Bool(value.can_execute)),
        (
            "can-manage-devices".into(),
            Val::Bool(value.can_manage_devices),
        ),
    ])
}

fn action_name(value: ProcessAction) -> &'static str {
    match value {
        ProcessAction::Observe => "observe",
        ProcessAction::Attach => "attach",
        ProcessAction::Input => "input",
        ProcessAction::CloseInput => "close-input",
        ProcessAction::Resize => "resize",
        ProcessAction::Signal => "signal",
    }
}
