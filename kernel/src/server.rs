use crate::{runtime::Runtime, service::ServiceRegistry, watch};
use axum::{
    Router,
    body::{Body, to_bytes},
    extract::{ConnectInfo, State},
    http::{HeaderMap, HeaderName, HeaderValue, Request, Response, StatusCode},
};
use semver::VersionReq;
use std::{net::SocketAddr, thread};
use wasmtime::component::Val;

mod limit;
mod stream;

use limit::StreamLimiter;

const HANDLER_SERVICE: &str = "ohrats:rc-http/handler";
const STREAM_SERVICE: &str = "ohrats:rc-http/stream-handler";
const MAX_BODY: usize = 2 * 1024 * 1024;
const MAX_HEADERS: usize = 128;
const MAX_HEADER_BYTES: usize = 8 * 1024;

#[derive(Clone)]
struct ServerState {
    registry: ServiceRegistry,
    stream_limiter: StreamLimiter,
}

pub fn run(mut runtime: Runtime, listen: Option<SocketAddr>) -> anyhow::Result<()> {
    let listen = listen.unwrap_or_else(default_listen);
    let registry = runtime.service_registry();
    let stream_limiter = StreamLimiter::configured()?;
    let tokio = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()?;
    let listener = tokio.block_on(tokio::net::TcpListener::bind(listen))?;
    let actual_listen = listener.local_addr()?;
    thread::spawn(move || {
        if let Err(error) = watch::run(&mut runtime) {
            eprintln!("component watcher stopped: {error:#}");
        }
    });
    println!("RC kernel HTTP listening on {actual_listen}");
    tokio.block_on(async move {
        let app = Router::new().fallback(dispatch).with_state(ServerState {
            registry,
            stream_limiter,
        });
        axum::serve(
            listener,
            app.into_make_service_with_connect_info::<SocketAddr>(),
        )
        .with_graceful_shutdown(async {
            let _ = tokio::signal::ctrl_c().await;
        })
        .await?;
        Ok(())
    })
}

async fn dispatch(
    State(state): State<ServerState>,
    ConnectInfo(remote): ConnectInfo<SocketAddr>,
    request: Request<Body>,
) -> Response<Body> {
    if request.uri().path() == "/healthz" {
        return plain(StatusCode::OK, "ok");
    }
    match component_request(state, remote, request).await {
        Ok(Some(response)) => response,
        Ok(None) => plain(StatusCode::NOT_FOUND, "not found"),
        Err(error) => {
            eprintln!("component HTTP request failed: {error:#}");
            plain(StatusCode::INTERNAL_SERVER_ERROR, "internal server error")
        }
    }
}

async fn component_request(
    state: ServerState,
    remote: SocketAddr,
    request: Request<Body>,
) -> anyhow::Result<Option<Response<Body>>> {
    let (parts, body) = request.into_parts();
    let body = to_bytes(body, MAX_BODY).await?;
    let value = request_value(&parts, remote, &body)?;
    let requirement = VersionReq::parse("^0.1")?;
    let stream_result = stream::open(
        state.registry.pinned(STREAM_SERVICE, &requirement)?,
        &value,
        &state.stream_limiter,
    );
    let mut first_error = match stream_result {
        Ok(Some(response)) => return Ok(Some(response)),
        Ok(None) => None,
        Err(error) => Some(error),
    };
    let calls = state
        .registry
        .call_all(HANDLER_SERVICE, &requirement, "handle", &[value])?;
    for (provider, result) in calls {
        match result.and_then(parse_handler_result) {
            Ok(Some(response)) => return Ok(Some(response)),
            Ok(None) => {}
            Err(error) if first_error.is_none() => {
                first_error = Some(anyhow::anyhow!("{provider}: {error:#}"));
            }
            Err(_) => {}
        }
    }
    if let Some(error) = first_error {
        Err(error)
    } else {
        Ok(None)
    }
}

fn request_value(
    parts: &axum::http::request::Parts,
    remote: SocketAddr,
    body: &[u8],
) -> anyhow::Result<Val> {
    let headers = header_values(&parts.headers)?;
    let scheme = parts.uri.scheme_str().unwrap_or("http").to_owned();
    let authority = parts
        .uri
        .authority()
        .map(|value| value.as_str().to_owned())
        .or_else(|| {
            parts
                .headers
                .get(axum::http::header::HOST)
                .and_then(|value| value.to_str().ok())
                .map(str::to_owned)
        })
        .unwrap_or_default();
    Ok(Val::Record(vec![
        ("method".into(), Val::String(parts.method.to_string())),
        ("scheme".into(), Val::String(scheme)),
        ("authority".into(), Val::String(authority)),
        ("path".into(), Val::String(parts.uri.path().to_owned())),
        (
            "query".into(),
            Val::String(parts.uri.query().unwrap_or_default().to_owned()),
        ),
        ("headers".into(), Val::List(headers)),
        (
            "body".into(),
            Val::List(body.iter().copied().map(Val::U8).collect()),
        ),
        (
            "remote-address".into(),
            Val::Option(Some(Box::new(Val::String(remote.to_string())))),
        ),
    ]))
}

fn header_values(headers: &HeaderMap) -> anyhow::Result<Vec<Val>> {
    anyhow::ensure!(headers.len() <= MAX_HEADERS, "too many HTTP headers");
    headers
        .iter()
        .map(|(name, value)| {
            let value = value.to_str()?;
            anyhow::ensure!(value.len() <= MAX_HEADER_BYTES, "HTTP header is too large");
            Ok(Val::Record(vec![
                ("name".into(), Val::String(name.as_str().to_owned())),
                ("value".into(), Val::String(value.to_owned())),
            ]))
        })
        .collect()
}

fn parse_handler_result(mut values: Vec<Val>) -> wasmtime::Result<Option<Response<Body>>> {
    if values.len() != 1 {
        return Err(wasmtime::format_err!(
            "HTTP handler returned {} values",
            values.len()
        ));
    }
    let Val::Result(result) = values.remove(0) else {
        return Err(wasmtime::format_err!(
            "HTTP handler returned a non-result value"
        ));
    };
    match result {
        Ok(Some(value)) => match *value {
            Val::Option(None) => Ok(None),
            Val::Option(Some(response)) => response_value(*response).map(Some),
            _ => Err(wasmtime::format_err!(
                "HTTP handler returned a non-option result"
            )),
        },
        Err(Some(error)) => match *error {
            Val::String(error) => Err(wasmtime::format_err!("{error}")),
            _ => Err(wasmtime::format_err!(
                "HTTP handler returned an invalid error"
            )),
        },
        Ok(None) | Err(None) => Err(wasmtime::format_err!(
            "HTTP handler returned an empty result"
        )),
    }
}

fn response_value(value: Val) -> wasmtime::Result<Response<Body>> {
    let fields = record(value, "HTTP response")?;
    let status = u16_field(&fields, "status")?;
    let headers = list_field(&fields, "headers")?;
    let body = bytes_field(&fields, "body")?;
    let mut builder = Response::builder().status(status);
    for value in headers {
        let header = record(value, "HTTP header")?;
        let name = string_field(&header, "name")?;
        let value = string_field(&header, "value")?;
        let name = HeaderName::from_bytes(name.as_bytes())
            .map_err(|error| wasmtime::format_err!("invalid response header: {error}"))?;
        let value = HeaderValue::from_str(&value)
            .map_err(|error| wasmtime::format_err!("invalid response header: {error}"))?;
        builder = builder.header(name, value);
    }
    builder
        .body(Body::from(body))
        .map_err(|error| wasmtime::format_err!("invalid HTTP response: {error}"))
}

fn record(value: Val, label: &str) -> wasmtime::Result<Vec<(String, Val)>> {
    match value {
        Val::Record(value) => Ok(value),
        _ => Err(wasmtime::format_err!("{label} is not a record")),
    }
}

fn field<'a>(fields: &'a [(String, Val)], name: &str) -> wasmtime::Result<&'a Val> {
    fields
        .iter()
        .find(|(candidate, _)| candidate == name)
        .map(|(_, value)| value)
        .ok_or_else(|| wasmtime::format_err!("missing HTTP field {name:?}"))
}

fn string_field(fields: &[(String, Val)], name: &str) -> wasmtime::Result<String> {
    match field(fields, name)? {
        Val::String(value) => Ok(value.clone()),
        _ => Err(wasmtime::format_err!("HTTP field {name:?} is not a string")),
    }
}

fn u16_field(fields: &[(String, Val)], name: &str) -> wasmtime::Result<u16> {
    match field(fields, name)? {
        Val::U16(value) => Ok(*value),
        _ => Err(wasmtime::format_err!("HTTP field {name:?} is not u16")),
    }
}

fn list_field(fields: &[(String, Val)], name: &str) -> wasmtime::Result<Vec<Val>> {
    match field(fields, name)? {
        Val::List(value) => Ok(value.clone()),
        _ => Err(wasmtime::format_err!("HTTP field {name:?} is not a list")),
    }
}

fn bytes_field(fields: &[(String, Val)], name: &str) -> wasmtime::Result<Vec<u8>> {
    list_field(fields, name)?
        .into_iter()
        .map(|value| match value {
            Val::U8(value) => Ok(value),
            _ => Err(wasmtime::format_err!("HTTP body contains a non-byte value")),
        })
        .collect()
}

fn plain(status: StatusCode, body: &'static str) -> Response<Body> {
    Response::builder()
        .status(status)
        .header("content-type", "text/plain; charset=utf-8")
        .header("cache-control", "no-store")
        .header("x-content-type-options", "nosniff")
        .body(Body::from(body))
        .expect("static HTTP response")
}

fn default_listen() -> SocketAddr {
    let port = std::env::var("PORT")
        .ok()
        .and_then(|value| value.parse().ok())
        .unwrap_or(3000);
    SocketAddr::from(([0, 0, 0, 0], port))
}
