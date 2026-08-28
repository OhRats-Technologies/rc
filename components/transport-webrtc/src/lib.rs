wit_bindgen::generate!({
    path: "../../wit",
    world: "transport-webrtc",
    generate_all,
});

use exports::ohrats::rc_transport::provider::Guest as ProviderGuest;
use ohrats::{
    rc_plugin::types::Service,
    rc_transport::types::{AnswerPlan, AnswerRequest, IceMode, IceServer},
};

const MAX_SERVERS: usize = 8;
const MAX_URLS: usize = 24;

struct WebrtcTransport;

impl Guest for WebrtcTransport {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:transport-webrtc".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-transport/provider".into(),
                version: "0.1.0".into(),
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
