use super::{encode, env_nonempty};
use crate::{SshKeyCommand, account};
use anyhow::{Context, Result, bail};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::Signer;
use futures_util::{SinkExt, StreamExt};
use rc_api_client::{ApiClient, Credential};
use rc_node::{DEFAULT_SERVER, load_config, resolve_state_dir};
use tokio::io::{AsyncReadExt, AsyncWriteExt};

pub(super) async fn key(command: SshKeyCommand) -> Result<()> {
    match command {
        SshKeyCommand::Add { name, file, url } => add_key(name, file, url).await,
        SshKeyCommand::List { url } => list_keys(url).await,
        SshKeyCommand::Remove { id, url } => remove_key(id, url).await,
    }
}

async fn add_key(name: Option<String>, file: Option<String>, url: Option<String>) -> Result<()> {
    let (server, credential) = account::defaults(url.as_deref(), None)?;
    let client = ApiClient::new(&server, credential.clone())?;
    let Credential::Pop(key) = credential else {
        bail!("SSH key registration requires RC CLI login")
    };
    let path = file
        .map(std::path::PathBuf::from)
        .unwrap_or_else(default_public_key_path);
    let public_key = std::fs::read_to_string(&path)
        .with_context(|| format!("read {}", path.display()))?
        .trim()
        .to_owned();
    if public_key.is_empty() {
        bail!("SSH public key is empty");
    }
    let payload = format!("rc-ssh-key-v1\n{}\n{public_key}", key.id);
    let signature = URL_SAFE_NO_PAD.encode(key.signing_key().sign(payload.as_bytes()).to_bytes());
    let key_name = name.unwrap_or_else(|| {
        path.file_stem()
            .and_then(|value| value.to_str())
            .unwrap_or("SSH key")
            .to_owned()
    });
    #[derive(serde::Deserialize)]
    struct Created {
        id: String,
        algorithm: String,
    }
    let created:Created=client.post("/api/v1/ssh/keys",&serde_json::json!({"name":key_name,"publicKey":public_key,"clientId":key.id,"signature":signature})).await?;
    println!(
        "Added SSH key {} ({}, {})",
        key_name, created.id, created.algorithm
    );
    Ok(())
}

async fn list_keys(url: Option<String>) -> Result<()> {
    #[derive(serde::Deserialize)]
    struct Key {
        id: String,
        name: String,
        algorithm: String,
    }
    #[derive(serde::Deserialize)]
    struct Response {
        keys: Vec<Key>,
    }
    let client = account::client(url.as_deref(), None)?;
    let response: Response = client.get("/api/v1/ssh/keys").await?;
    for key in response.keys {
        println!("{}  {}  {}", key.id, key.name, key.algorithm);
    }
    Ok(())
}

async fn remove_key(id: String, url: Option<String>) -> Result<()> {
    let client = account::client(url.as_deref(), None)?;
    let _: serde_json::Value = client
        .delete(&format!("/api/v1/ssh/keys/{}", encode(&id)))
        .await?;
    println!("Removed SSH key {id}");
    Ok(())
}

pub(super) async fn config(url: Option<String>, token: Option<String>) -> Result<()> {
    let (server, credential) = account::defaults(url.as_deref(), token.as_deref())?;
    let client = ApiClient::new(&server, credential)?;
    let devices = client.devices().await?;
    let parsed = url::Url::parse(&server).context("invalid RC server URL")?;
    let host = parsed
        .host_str()
        .ok_or_else(|| anyhow::anyhow!("invalid RC server URL"))?;
    for (index, device) in devices.iter().enumerate() {
        if index > 0 {
            println!();
        }
        println!(
            "# {} — {}",
            device.name.replace('\n', " "),
            device.workspace.replace('\n', " ")
        );
        println!("Host rc-{}", device.id);
        println!("  HostName {host}");
        println!("  User rc");
        println!("  HostKeyAlias {host}");
        println!("  SetEnv RC_DEVICE_ID={}", device.id);
        println!(
            "  ProxyCommand rc ssh-proxy --url {}",
            server.trim_end_matches('/')
        );
    }
    Ok(())
}

pub(super) async fn proxy(url: Option<String>) -> Result<()> {
    let server = server_only(url.as_deref());
    let mut endpoint = url::Url::parse(&server).context("invalid RC server URL")?;
    match endpoint.scheme() {
        "https" => endpoint
            .set_scheme("wss")
            .map_err(|_| anyhow::anyhow!("invalid RC server URL"))?,
        "http" => endpoint
            .set_scheme("ws")
            .map_err(|_| anyhow::anyhow!("invalid RC server URL"))?,
        "ws" | "wss" => {}
        _ => bail!("invalid RC server URL"),
    };
    endpoint.set_path("/api/v1/ssh/tunnel");
    endpoint.set_query(None);
    endpoint.set_fragment(None);
    let (socket, _) = tokio_tungstenite::connect_async(endpoint.as_str())
        .await
        .context("connect RC SSH tunnel")?;
    let (mut send, mut receive) = socket.split();
    let outbound = tokio::spawn(async move {
        let mut stdin = tokio::io::stdin();
        let mut buffer = [0_u8; 32 * 1024];
        loop {
            match stdin.read(&mut buffer).await {
                Ok(0) => {
                    let _ = send.close().await;
                    break;
                }
                Ok(count) => {
                    if send
                        .send(tokio_tungstenite::tungstenite::Message::Binary(
                            buffer[..count].to_vec().into(),
                        ))
                        .await
                        .is_err()
                    {
                        break;
                    }
                }
                Err(_) => break,
            }
        }
    });
    let mut stdout = tokio::io::stdout();
    while let Some(message) = receive.next().await {
        match message? {
            tokio_tungstenite::tungstenite::Message::Binary(data) => {
                stdout.write_all(&data).await?;
                stdout.flush().await?
            }
            tokio_tungstenite::tungstenite::Message::Close(_) => break,
            _ => {}
        }
    }
    outbound.abort();
    Ok(())
}

fn default_public_key_path() -> std::path::PathBuf {
    std::env::var_os("HOME")
        .map(std::path::PathBuf::from)
        .unwrap_or_else(|| std::path::PathBuf::from("."))
        .join(".ssh/id_ed25519.pub")
}
fn server_only(explicit: Option<&str>) -> String {
    let dir = resolve_state_dir(None);
    let account = rc_node::load_account(&dir).unwrap_or_default();
    let config = load_config(&dir).unwrap_or_default();
    explicit
        .map(str::to_owned)
        .or_else(|| env_nonempty("RC_URL"))
        .or_else(|| (!account.server.is_empty()).then_some(account.server))
        .or_else(|| (!config.server.is_empty()).then_some(config.server))
        .unwrap_or_else(|| DEFAULT_SERVER.into())
}
