use super::values;
use crate::{descriptor::SelectionMode, service::ServiceRegistry};
use rc_node::{TransportAnswerPlan, TransportAnswerRequest, TransportPolicy};
use rc_protocol::{ControlIceMode, IceServer};
use semver::VersionReq;
use std::time::Duration;
use wasmtime::component::Val;

const SERVICE: &str = "ohrats:rc-transport/provider";

#[derive(Clone)]
pub struct ComponentTransportPolicy {
    registry: ServiceRegistry,
    requirement: VersionReq,
}

impl ComponentTransportPolicy {
    pub fn new(registry: ServiceRegistry) -> anyhow::Result<Self> {
        Ok(Self {
            registry,
            requirement: VersionReq::parse("^0.1")?,
        })
    }

    pub fn available(&self, transport: &str) -> anyhow::Result<bool> {
        Ok(self
            .registry
            .has_provider(SERVICE, &self.requirement, Some(transport))?)
    }
}

impl TransportPolicy for ComponentTransportPolicy {
    fn answer_plan(
        &self,
        transport: &str,
        request: TransportAnswerRequest,
    ) -> Result<TransportAnswerPlan, String> {
        let params = [
            Val::String(transport.to_owned()),
            Val::Record(vec![
                ("mode".into(), Val::Enum(mode_name(request.mode).into())),
                (
                    "ice-servers".into(),
                    Val::List(request.ice_servers.into_iter().map(server_value).collect()),
                ),
            ]),
        ];
        let values = self
            .registry
            .call_one(
                SERVICE,
                &self.requirement,
                SelectionMode::Keyed,
                "plan-answer",
                &params,
            )
            .map_err(|error| error.to_string())?;
        let fields = values::record(
            values::result_value(values, "transport policy")?,
            "answer plan",
        )?;
        Ok(TransportAnswerPlan {
            ice_servers: values::list_field(&fields, "ice-servers")?
                .into_iter()
                .map(server_from_value)
                .collect::<Result<_, _>>()?,
            gather_timeout: Duration::from_millis(u64::from(values::u32_field(
                &fields,
                "gather-timeout-ms",
            )?)),
            connect_timeout: Duration::from_millis(u64::from(values::u32_field(
                &fields,
                "connect-timeout-ms",
            )?)),
        })
    }
}

fn server_value(server: IceServer) -> Val {
    Val::Record(vec![
        (
            "urls".into(),
            Val::List(server.urls.into_iter().map(Val::String).collect()),
        ),
        ("username".into(), Val::String(server.username)),
        ("credential".into(), Val::String(server.credential)),
    ])
}

fn server_from_value(value: Val) -> Result<IceServer, String> {
    let fields = values::record(value, "ICE server")?;
    let urls = values::list_field(&fields, "urls")?
        .into_iter()
        .map(|value| match value {
            Val::String(value) => Ok::<String, String>(value),
            _ => Err::<String, String>("ICE URL is not a string".into()),
        })
        .collect::<Result<_, _>>()?;
    Ok(IceServer {
        urls,
        username: values::string_field(&fields, "username")?,
        credential: values::string_field(&fields, "credential")?,
    })
}

fn mode_name(value: ControlIceMode) -> &'static str {
    match value {
        ControlIceMode::Host => "host",
        ControlIceMode::Stun => "stun",
        ControlIceMode::Relay => "relay",
    }
}
