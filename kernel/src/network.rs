use crate::{
    bindings::ohrats::rc_plugin::http_client::{Header, Host as HttpClientHost, Request, Response},
    host::HostState,
};
use reqwest::{Method, header::HeaderName};
use std::io::Read;

const MAX_REQUEST_BYTES: usize = 2 * 1024 * 1024;
const MAX_RESPONSE_BYTES: usize = 48 * 1024 * 1024;
const MAX_HEADERS: usize = 128;
const MAX_HEADER_BYTES: usize = 8 * 1024;

impl HttpClientHost for HostState {
    fn send(&mut self, request: Request) -> Result<Response, String> {
        send(self, request).map_err(|error| format!("{error:#}"))
    }
}

fn send(state: &HostState, request: Request) -> anyhow::Result<Response> {
    anyhow::ensure!(
        request.body.len() <= MAX_REQUEST_BYTES,
        "HTTP request body exceeds 2 MiB"
    );
    anyhow::ensure!(
        request.headers.len() <= MAX_HEADERS,
        "HTTP request has too many headers"
    );
    let maximum = usize::try_from(request.max_response_bytes)
        .map_err(|_| anyhow::anyhow!("invalid HTTP response limit"))?;
    anyhow::ensure!(
        maximum > 0 && maximum <= MAX_RESPONSE_BYTES,
        "HTTP response limit must be between 1 byte and 48 MiB"
    );
    let method = Method::from_bytes(request.method.as_bytes())?;
    anyhow::ensure!(
        !matches!(method, Method::CONNECT | Method::TRACE),
        "HTTP method is not allowed"
    );
    let url = reqwest::Url::parse(&request.url)?;
    anyhow::ensure!(
        matches!(url.scheme(), "http" | "https"),
        "HTTP adapter accepts only http and https URLs"
    );
    let mut builder = state.environment.http.request(method, url);
    for header in request.headers {
        anyhow::ensure!(
            header.name.len() <= 128 && header.value.len() <= MAX_HEADER_BYTES,
            "HTTP request header is too large"
        );
        let name = HeaderName::from_bytes(header.name.as_bytes())?;
        let value = reqwest::header::HeaderValue::from_str(&header.value)?;
        builder = builder.header(name, value);
    }
    if !request.body.is_empty() {
        builder = builder.body(request.body);
    }
    let response = builder.send()?;
    let status = response.status().as_u16();
    let final_url = response.url().to_string();
    let headers = response_headers(response.headers());
    let mut body = Vec::with_capacity(maximum.min(64 * 1024));
    response.take((maximum + 1) as u64).read_to_end(&mut body)?;
    anyhow::ensure!(
        body.len() <= maximum,
        "HTTP response exceeds configured limit"
    );
    Ok(Response {
        status,
        final_url,
        headers,
        body,
    })
}

fn response_headers(values: &reqwest::header::HeaderMap) -> Vec<Header> {
    values
        .iter()
        .take(MAX_HEADERS)
        .filter_map(|(name, value)| {
            let value = value.to_str().ok()?;
            (value.len() <= MAX_HEADER_BYTES).then(|| Header {
                name: name.as_str().to_owned(),
                value: value.to_owned(),
            })
        })
        .collect()
}
