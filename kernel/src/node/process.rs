use super::values;
use crate::{descriptor::SelectionMode, service::ServiceRegistry};
use rc_node::{
    ProcessAccessRequest, ProcessAction, ProcessChannel, ProcessPolicy, ProcessResizeRequest,
    ProcessSignalRequest, ProcessStartPlan, ProcessStartRequest, ProcessTerminalSize,
};
use rc_protocol::TerminalSpec;
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
            requirement: VersionReq::parse("^0.1")?,
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
            command: values::string_field(&fields, "command")?,
            cwd: values::string_field(&fields, "cwd")?,
            terminal: terminal_option(&fields)?,
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

    fn normalize_signal(&self, request: ProcessSignalRequest) -> Result<String, String> {
        let value = Val::Record(vec![
            ("access".into(), access_request(request.access)),
            ("signal".into(), Val::String(request.signal)),
        ]);
        match values::result_value(self.call("normalize-signal", value)?, "process policy")? {
            Val::String(value) => Ok(value),
            _ => Err("process policy returned a non-string signal".into()),
        }
    }
}

fn start_request(request: ProcessStartRequest) -> Val {
    Val::Record(vec![
        ("process-id".into(), Val::String(request.process_id)),
        ("command".into(), Val::String(request.command)),
        ("cwd".into(), Val::String(request.cwd)),
        ("terminal".into(), terminal_value(request.terminal)),
        (
            "channel".into(),
            Val::Enum(channel_name(request.channel).into()),
        ),
        ("principal".into(), principal_value(request.principal)),
    ])
}

fn access_request(request: ProcessAccessRequest) -> Val {
    Val::Record(vec![
        ("process-id".into(), Val::String(request.process_id)),
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

fn terminal_value(value: Option<TerminalSpec>) -> Val {
    Val::Option(value.map(|value| {
        Box::new(Val::Record(vec![
            ("cols".into(), Val::U16(value.cols)),
            ("rows".into(), Val::U16(value.rows)),
            ("term".into(), Val::String(value.term)),
        ]))
    }))
}

fn terminal_option(fields: &[(String, Val)]) -> Result<Option<TerminalSpec>, String> {
    values::option_record_field(fields, "terminal")?
        .map(|fields| {
            Ok(TerminalSpec {
                cols: values::u16_field(&fields, "cols")?,
                rows: values::u16_field(&fields, "rows")?,
                term: values::string_field(&fields, "term")?,
            })
        })
        .transpose()
}

fn channel_name(value: ProcessChannel) -> &'static str {
    match value {
        ProcessChannel::Control => "control",
        ProcessChannel::Ssh => "ssh",
        ProcessChannel::Mcp => "mcp",
    }
}

fn action_name(value: ProcessAction) -> &'static str {
    match value {
        ProcessAction::Attach => "attach",
        ProcessAction::Input => "input",
        ProcessAction::CloseInput => "close-input",
        ProcessAction::Resize => "resize",
        ProcessAction::Signal => "signal",
    }
}
