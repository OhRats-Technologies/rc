use rc_protocol::IceServer;
use serde::Deserialize;
use std::{
    sync::Arc,
    time::{Duration, Instant},
};
use tokio::sync::Mutex;

#[derive(Clone)]
pub struct TurnProvider(Arc<Inner>);

struct Inner {
    token_id: Option<String>,
    api_token: Option<String>,
    fixed_servers: Option<Vec<IceServer>>,
    client: reqwest::Client,
    cache: Mutex<Option<Cached>>,
}

#[derive(Clone)]
struct Cached {
    servers: Vec<IceServer>,
    expires: Instant,
}

impl TurnProvider {
    pub fn new(token_id: Option<String>, api_token: Option<String>) -> Self {
        Self(Arc::new(Inner {
            token_id,
            api_token,
            fixed_servers: None,
            client: reqwest::Client::new(),
            cache: Mutex::new(None),
        }))
    }

    pub fn fixed(servers: Vec<IceServer>) -> Self {
        Self(Arc::new(Inner {
            token_id: None,
            api_token: None,
            fixed_servers: Some(servers),
            client: reqwest::Client::new(),
            cache: Mutex::new(None),
        }))
    }

    pub async fn ice_servers(&self) -> anyhow::Result<Vec<IceServer>> {
        if let Some(servers) = &self.0.fixed_servers {
            return Ok(servers.clone());
        }
        let fallback = || {
            vec![IceServer {
                urls: vec!["stun:stun.cloudflare.com:3478".into()],
                username: String::new(),
                credential: String::new(),
            }]
        };
        let (Some(token_id), Some(api_token)) = (&self.0.token_id, &self.0.api_token) else {
            return Ok(fallback());
        };
        let mut cache = self.0.cache.lock().await;
        if let Some(value) = cache
            .as_ref()
            .filter(|value| value.expires > Instant::now())
        {
            return Ok(value.servers.clone());
        }
        #[derive(Deserialize)]
        #[serde(rename_all = "camelCase")]
        struct Response {
            ice_servers: Vec<IceServer>,
        }
        let url = format!(
            "https://rtc.live.cloudflare.com/v1/turn/keys/{token_id}/credentials/generate-ice-servers"
        );
        let response = self
            .0
            .client
            .post(url)
            .bearer_auth(api_token)
            .json(&serde_json::json!({ "ttl": 3600 }))
            .send()
            .await?;
        if !response.status().is_success() {
            anyhow::bail!(
                "Cloudflare TURN credential request failed: {}",
                response.status()
            );
        }
        let mut servers = response.json::<Response>().await?.ice_servers;
        if servers.is_empty() {
            servers = fallback();
        }
        *cache = Some(Cached {
            servers: servers.clone(),
            expires: Instant::now() + Duration::from_secs(3000),
        });
        Ok(servers)
    }
}
