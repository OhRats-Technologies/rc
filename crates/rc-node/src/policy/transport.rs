use rc_protocol::{ControlIceMode, IceServer};
use std::time::Duration;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TransportAnswerRequest {
    pub mode: ControlIceMode,
    pub ice_servers: Vec<IceServer>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TransportAnswerPlan {
    pub ice_servers: Vec<IceServer>,
    pub gather_timeout: Duration,
    pub connect_timeout: Duration,
}

pub trait TransportPolicy: Send + Sync {
    fn answer_plan(
        &self,
        transport: &str,
        request: TransportAnswerRequest,
    ) -> Result<TransportAnswerPlan, String>;
}

#[derive(Debug, Default)]
pub struct NativeTransportPolicy;

impl TransportPolicy for NativeTransportPolicy {
    fn answer_plan(
        &self,
        transport: &str,
        request: TransportAnswerRequest,
    ) -> Result<TransportAnswerPlan, String> {
        if transport != "webrtc" {
            return Err("unsupported transport".into());
        }
        if request.ice_servers.len() > 8 {
            return Err("too many ICE servers".into());
        }
        let mut total_urls = 0;
        let mut ice_servers = Vec::new();
        for server in request.ice_servers {
            let urls = filter_urls(request.mode, server.urls)?;
            total_urls += urls.len();
            if total_urls > 24 {
                return Err("too many ICE URLs".into());
            }
            if !urls.is_empty() {
                ice_servers.push(IceServer {
                    urls,
                    username: server.username,
                    credential: server.credential,
                });
            }
        }
        let (gather_timeout, connect_timeout) = match request.mode {
            ControlIceMode::Host => (Duration::from_secs(2), Duration::from_secs(6)),
            ControlIceMode::Stun => (Duration::from_secs(8), Duration::from_secs(12)),
            ControlIceMode::Relay => (Duration::from_secs(15), Duration::from_secs(18)),
        };
        Ok(TransportAnswerPlan {
            ice_servers,
            gather_timeout,
            connect_timeout,
        })
    }
}

fn filter_urls(mode: ControlIceMode, urls: Vec<String>) -> Result<Vec<String>, String> {
    urls.into_iter()
        .filter_map(|url| {
            let value = url.trim().to_owned();
            if value.len() > 2_048 || value.contains(['\0', '\r', '\n']) {
                return Some(Err("invalid ICE URL".into()));
            }
            let lower = value.to_ascii_lowercase();
            let include = match mode {
                ControlIceMode::Host => false,
                ControlIceMode::Stun => lower.starts_with("stun:") || lower.starts_with("stuns:"),
                ControlIceMode::Relay => lower.starts_with("turn:") || lower.starts_with("turns:"),
            };
            include.then_some(Ok(value))
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn relay_mode_excludes_stun_urls() {
        let policy = NativeTransportPolicy;
        let plan = policy
            .answer_plan(
                "webrtc",
                TransportAnswerRequest {
                    mode: ControlIceMode::Relay,
                    ice_servers: vec![IceServer {
                        urls: vec!["stun:example.test".into(), "turn:example.test".into()],
                        username: String::new(),
                        credential: String::new(),
                    }],
                },
            )
            .unwrap();
        assert_eq!(plan.ice_servers[0].urls, ["turn:example.test"]);
    }
}
