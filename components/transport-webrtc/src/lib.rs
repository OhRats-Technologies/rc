wit_bindgen::generate!({
    path: "../../wit",
    world: "transport-webrtc",
    generate_all,
});

use exports::ohrats::rc_transport::provider::Guest as ProviderGuest;
use ohrats::{
    rc_plugin::types::Service,
    rc_transport::types::{
        AnswerPlan, AnswerRequest, Attempt, CandidateKind, IceMode, IceServer, RouteClass,
        SelectedRoute,
    },
};

const MAX_SERVERS: usize = 8;
const MAX_URLS: usize = 24;

struct WebrtcTransport;

impl Guest for WebrtcTransport {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:transport-webrtc".into(),
            version: "0.2.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-transport/provider".into(),
                version: "0.2.0".into(),
                priority: 100,
                keys: vec!["webrtc".into()],
            }],
            requires: Vec::new(),
            commands: Vec::new(),
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        Err(format!("unsupported command {command:?}"))
    }
}

impl ProviderGuest for WebrtcTransport {
    fn plan_attempts(transport: String, ice_servers: Vec<IceServer>) -> Result<Vec<Attempt>, String> {
        if transport != "webrtc" {
            return Err("unsupported transport".into());
        }
        let has_stun = has_scheme(&ice_servers, &["stun:", "stuns:"])?;
        let has_turn = has_scheme(&ice_servers, &["turn:", "turns:"])?;
        let mut attempts = vec![attempt(IceMode::Host, 2_000, 6_000, 0)];
        if has_stun {
            attempts.push(attempt(IceMode::Stun, 8_000, 12_000, 1_200));
        }
        if has_turn {
            attempts.push(attempt(IceMode::Relay, 15_000, 18_000, 1_200));
        }
        Ok(attempts)
    }

    fn plan_answer(transport: String, request: AnswerRequest) -> Result<AnswerPlan, String> {
        if transport != "webrtc" {
            return Err("unsupported transport".into());
        }
        if request.ice_servers.len() > MAX_SERVERS {
            return Err("too many ICE servers".into());
        }
        let mut total_urls = 0;
        let mut servers = Vec::new();
        for server in request.ice_servers {
            let filtered = filter_urls(&request.mode, server.urls)?;
            total_urls += filtered.len();
            if total_urls > MAX_URLS {
                return Err("too many ICE URLs".into());
            }
            if !filtered.is_empty() {
                servers.push(IceServer {
                    urls: filtered,
                    username: server.username,
                    credential: server.credential,
                });
            }
        }
        let (gather_timeout_ms, connect_timeout_ms) = match request.mode {
            IceMode::Host => (2_000, 6_000),
            IceMode::Stun => (8_000, 12_000),
            IceMode::Relay => (15_000, 18_000),
        };
        Ok(AnswerPlan {
            ice_servers: servers,
            gather_timeout_ms,
            connect_timeout_ms,
        })
    }

    fn classify_route(route: SelectedRoute) -> RouteClass {
        if route.local == CandidateKind::Relay || route.remote == CandidateKind::Relay {
            RouteClass::TurnRelay
        } else if route.local == CandidateKind::ServerReflexive
            || route.remote == CandidateKind::ServerReflexive
        {
            RouteClass::DirectStun
        } else if route.local == CandidateKind::Host && route.remote == CandidateKind::Host {
            RouteClass::DirectHost
        } else {
            RouteClass::Unknown
        }
    }
}

fn attempt(
    mode: IceMode,
    gather_timeout_ms: u32,
    connect_timeout_ms: u32,
    retry_delay_ms: u32,
) -> Attempt {
    Attempt {
        mode,
        gather_timeout_ms,
        connect_timeout_ms,
        retry_delay_ms,
    }
}

fn has_scheme(servers: &[IceServer], schemes: &[&str]) -> Result<bool, String> {
    let mut count = 0;
    for url in servers.iter().flat_map(|server| &server.urls) {
        count += 1;
        if count > MAX_URLS || url.len() > 2_048 || url.contains(['\0', '\r', '\n']) {
            return Err("invalid ICE server list".into());
        }
        let lower = url.trim().to_ascii_lowercase();
        if schemes.iter().any(|scheme| lower.starts_with(scheme)) {
            return Ok(true);
        }
    }
    Ok(false)
}

fn filter_urls(mode: &IceMode, urls: Vec<String>) -> Result<Vec<String>, String> {
    urls.into_iter()
        .filter_map(|url| {
            let value = url.trim().to_owned();
            if value.len() > 2_048 || value.contains(['\0', '\r', '\n']) {
                return Some(Err("invalid ICE URL".into()));
            }
            let lower = value.to_ascii_lowercase();
            let include = match mode {
                IceMode::Host => false,
                IceMode::Stun => lower.starts_with("stun:") || lower.starts_with("stuns:"),
                IceMode::Relay => lower.starts_with("turn:") || lower.starts_with("turns:"),
            };
            include.then_some(Ok(value))
        })
        .collect()
}

export!(WebrtcTransport);

#[cfg(test)]
mod tests {
    use super::*;

    fn server(urls: &[&str]) -> IceServer {
        IceServer {
            urls: urls.iter().map(|value| (*value).into()).collect(),
            username: "user".into(),
            credential: "secret".into(),
        }
    }

    #[test]
    fn host_mode_has_no_external_ice_servers() {
        let plan = WebrtcTransport::plan_answer(
            "webrtc".into(),
            AnswerRequest {
                mode: IceMode::Host,
                ice_servers: vec![server(&["stun:example.test", "turn:example.test"])],
            },
        )
        .unwrap();
        assert!(plan.ice_servers.is_empty());
    }

    #[test]
    fn attempts_are_host_then_stun_then_turn() {
        let attempts = WebrtcTransport::plan_attempts(
            "webrtc".into(),
            vec![server(&[
                "turn:relay.example.test",
                "stun:stun.example.test",
            ])],
        )
        .unwrap();
        assert_eq!(
            attempts.iter().map(|attempt| attempt.mode).collect::<Vec<_>>(),
            [IceMode::Host, IceMode::Stun, IceMode::Relay]
        );
    }

    #[test]
    fn stun_and_relay_modes_are_separated() {
        let input = vec![server(&[
            "stun:example.test",
            "turn:example.test?transport=udp",
        ])];
        let stun = WebrtcTransport::plan_answer(
            "webrtc".into(),
            AnswerRequest {
                mode: IceMode::Stun,
                ice_servers: input.clone(),
            },
        )
        .unwrap();
        let relay = WebrtcTransport::plan_answer(
            "webrtc".into(),
            AnswerRequest {
                mode: IceMode::Relay,
                ice_servers: input,
            },
        )
        .unwrap();
        assert_eq!(stun.ice_servers[0].urls, ["stun:example.test"]);
        assert_eq!(
            relay.ice_servers[0].urls,
            ["turn:example.test?transport=udp"]
        );
    }
}
