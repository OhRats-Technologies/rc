use crate::{Credential, Device};
use reqwest::{Method, Response};
use serde::{Serialize, de::DeserializeOwned};
use url::Url;

#[derive(Debug, thiserror::Error)]
pub enum ApiError {
    #[error(transparent)]
    Auth(#[from] crate::AuthError),
    #[error("invalid RC server URL")]
    Url,
    #[error("RC request failed: {0}")]
    Transport(#[from] reqwest::Error),
    #[error("{status}: {message}")]
    Status { status: u16, message: String },
}

#[derive(Clone)]
pub struct ApiClient {
    base: Url,
    credential: Credential,
    http: reqwest::Client,
}

impl ApiClient {
    pub fn new(server: &str, credential: Credential) -> Result<Self, ApiError> {
        let base = Url::parse(server.trim_end_matches('/')).map_err(|_| ApiError::Url)?;
        Ok(Self {
            base,
            credential,
            http: reqwest::Client::new(),
        })
    }

    pub async fn get<T: DeserializeOwned>(&self, path: &str) -> Result<T, ApiError> {
        self.json(Method::GET, path, Option::<&()>::None).await
    }

    pub async fn post<I: Serialize + ?Sized, T: DeserializeOwned>(
        &self,
        path: &str,
        body: &I,
    ) -> Result<T, ApiError> {
        self.json(Method::POST, path, Some(body)).await
    }

    pub async fn delete<T: DeserializeOwned>(&self, path: &str) -> Result<T, ApiError> {
        self.json(Method::DELETE, path, Option::<&()>::None).await
    }

    pub async fn request_empty(&self, method: Method, path: &str) -> Result<(), ApiError> {
        let response = self.send(method, path, &[], false).await?;
        ensure_success(response).await.map(|_| ())
    }

    pub async fn devices(&self) -> Result<Vec<Device>, ApiError> {
        #[derive(serde::Deserialize)]
        struct Out {
            devices: Vec<Device>,
        }
        Ok(self.get::<Out>("/api/v1/devices").await?.devices)
    }

    pub async fn device(&self, id: &str) -> Result<Device, ApiError> {
        #[derive(serde::Deserialize)]
        struct Out {
            device: Device,
        }
        Ok(self
            .get::<Out>(&format!("/api/v1/devices/{}", path_segment(id)))
            .await?
            .device)
    }

    async fn json<I: Serialize + ?Sized, T: DeserializeOwned>(
        &self,
        method: Method,
        path: &str,
        body: Option<&I>,
    ) -> Result<T, ApiError> {
        let bytes = match body {
            Some(value) => serde_json::to_vec(value).map_err(|error| ApiError::Status {
                status: 0,
                message: error.to_string(),
            })?,
            None => Vec::new(),
        };
        let response = self.send(method, path, &bytes, body.is_some()).await?;
        let response = ensure_success(response).await?;
        response.json().await.map_err(ApiError::Transport)
    }

    async fn send(
        &self,
        method: Method,
        path: &str,
        body: &[u8],
        has_json: bool,
    ) -> Result<Response, ApiError> {
        let url = self.base.join(path).map_err(|_| ApiError::Url)?;
        let request_uri = match url.query() {
            Some(query) => format!("{}?{query}", url.path()),
            None => url.path().to_owned(),
        };
        let mut request = self.http.request(method.clone(), url);
        match &self.credential {
            Credential::Bearer(token) => {
                request = request.bearer_auth(token);
            }
            Credential::Pop(key) => {
                for (name, value) in key.headers(method.as_str(), &request_uri, body) {
                    request = request.header(name, value);
                }
            }
        }
        if has_json {
            request = request
                .header("content-type", "application/json")
                .body(body.to_vec());
        }
        request.send().await.map_err(ApiError::Transport)
    }
}

pub async fn public_post<I: Serialize + ?Sized, T: DeserializeOwned>(
    server: &str,
    path: &str,
    body: &I,
) -> Result<T, ApiError> {
    let base = Url::parse(server.trim_end_matches('/')).map_err(|_| ApiError::Url)?;
    let response = reqwest::Client::new()
        .post(base.join(path).map_err(|_| ApiError::Url)?)
        .json(body)
        .send()
        .await?;
    let response = ensure_success(response).await?;
    response.json().await.map_err(ApiError::Transport)
}

async fn ensure_success(response: Response) -> Result<Response, ApiError> {
    if response.status().is_success() {
        return Ok(response);
    }
    let status = response.status().as_u16();
    let text = response.text().await.unwrap_or_default();
    let message = serde_json::from_str::<serde_json::Value>(&text)
        .ok()
        .and_then(|value| value.get("error")?.as_str().map(str::to_owned))
        .unwrap_or_else(|| text.chars().take(4096).collect());
    Err(ApiError::Status { status, message })
}

fn path_segment(value: &str) -> String {
    url::form_urlencoded::byte_serialize(value.as_bytes()).collect()
}
