use super::{
    bytes_field, limit::StreamLimiter, list_field, plain, record, string_field, u16_field,
};
use crate::service::PinnedProvider;
use axum::{
    body::Body,
    http::{HeaderName, HeaderValue, Response, StatusCode},
};
use std::{
    env,
    time::{Duration, Instant},
};
use tokio::sync::mpsc;
use tokio_stream::wrappers::ReceiverStream;
use wasmtime::component::Val;

const SERVICE: &str = "ohrats:rc-http/stream-handler";
const MAX_CHUNK: usize = 64 * 1024;
const MAX_TOTAL: usize = 16 * 1024 * 1024;
const MAX_IDLE: Duration = Duration::from_secs(30);
const MAX_SESSION: Duration = Duration::from_secs(300);
const MAX_POLL_DELAY: Duration = Duration::from_secs(5);
const MIN_POLL_DELAY: Duration = Duration::from_millis(1);
const DISCONNECT_CHECK: Duration = Duration::from_millis(25);

#[derive(Clone, Copy)]
struct Limits {
    chunk: usize,
    total: usize,
    idle: Duration,
    session: Duration,
}

impl Limits {
    fn configured() -> Self {
        Self {
            chunk: setting("RC_HTTP_STREAM_MAX_CHUNK", MAX_CHUNK),
            total: setting("RC_HTTP_STREAM_MAX_TOTAL", MAX_TOTAL),
            idle: Duration::from_millis(setting(
                "RC_HTTP_STREAM_MAX_IDLE_MS",
                MAX_IDLE.as_millis() as usize,
            ) as u64),
            session: Duration::from_millis(setting(
                "RC_HTTP_STREAM_MAX_SESSION_MS",
                MAX_SESSION.as_millis() as usize,
            ) as u64),
        }
    }
}

fn setting(name: &str, default: usize) -> usize {
    env::var(name)
        .ok()
        .and_then(|value| value.parse().ok())
        .filter(|value| *value > 0)
        .unwrap_or(default)
}

pub(super) fn open(
    providers: Vec<PinnedProvider>,
    request: &Val,
    limiter: &StreamLimiter,
) -> anyhow::Result<Option<Response<Body>>> {
    let mut first_error = None;
    for provider in providers {
        match provider
            .call(SERVICE, "open", std::slice::from_ref(request))
            .and_then(parse_open)
        {
            Ok(Some(opened)) => return opened.response(provider, limiter).map(Some),
            Ok(None) => {}
            Err(error) if first_error.is_none() => {
                first_error = Some(anyhow::anyhow!("{}: {error:#}", provider.component_id()));
            }
            Err(_) => {}
        }
    }
    first_error.map_or(Ok(None), Err)
}

struct Opened {
    status: u16,
    headers: Vec<Val>,
    session_id: String,
}

impl Opened {
    fn response(
        self,
        provider: PinnedProvider,
        limiter: &StreamLimiter,
    ) -> anyhow::Result<Response<Body>> {
        if self.session_id.is_empty() {
            close(&provider, self.session_id, "provider-error");
            anyhow::bail!("empty stream session id");
        }
        let Some(permit) = limiter.acquire() else {
            close(&provider, self.session_id, "limit-exceeded");
            return Ok(plain(
                StatusCode::SERVICE_UNAVAILABLE,
                "stream capacity exceeded",
            ));
        };
        let builder = match response_builder(self.status, self.headers) {
            Ok(builder) => builder,
            Err(error) => {
                close(&provider, self.session_id, "provider-error");
                return Err(error);
            }
        };
        let (sender, receiver) = mpsc::channel(1);
        let response = match builder.body(Body::from_stream(ReceiverStream::new(receiver))) {
            Ok(response) => response,
            Err(error) => {
                close(&provider, self.session_id, "provider-error");
                return Err(error.into());
            }
        };
        let session_id = self.session_id;
        let limits = Limits::configured();
        tokio::task::spawn_blocking(move || {
            produce(provider, session_id, sender, limits);
            drop(permit);
        });
        Ok(response)
    }
}

fn response_builder(
    status: u16,
    headers: Vec<Val>,
) -> anyhow::Result<axum::http::response::Builder> {
    anyhow::ensure!(
        headers.len() <= super::MAX_HEADERS,
        "too many HTTP response headers"
    );
    let mut builder = Response::builder().status(status);
    for value in headers {
        let header = record(value, "HTTP stream header")?;
        let name = string_field(&header, "name")?;
        let value = string_field(&header, "value")?;
        anyhow::ensure!(
            value.len() <= super::MAX_HEADER_BYTES,
            "HTTP response header is too large"
        );
        builder = builder.header(
            HeaderName::from_bytes(name.as_bytes())?,
            HeaderValue::from_str(&value)?,
        );
    }
    Ok(builder)
}

fn produce(
    provider: PinnedProvider,
    session_id: String,
    sender: mpsc::Sender<Result<Vec<u8>, String>>,
    limits: Limits,
) {
    let started = Instant::now();
    let mut active = Instant::now();
    let mut total: usize = 0;
    let reason = loop {
        if sender.is_closed() {
            break "client-disconnected";
        }
        if started.elapsed() > limits.session || active.elapsed() > limits.idle {
            break "limit-exceeded";
        }
        match provider
            .call(SERVICE, "poll", &[Val::String(session_id.clone())])
            .and_then(parse_poll)
        {
            Ok(Poll::Chunk(bytes)) => {
                if bytes.len() > limits.chunk || total.saturating_add(bytes.len()) > limits.total {
                    let _ = sender.blocking_send(Err("stream response limit exceeded".into()));
                    break "limit-exceeded";
                }
                total += bytes.len();
                active = Instant::now();
                if sender.blocking_send(Ok(bytes)).is_err() {
                    break "client-disconnected";
                }
            }
            Ok(Poll::Pending(delay)) => {
                if wait_pending(delay, &sender) {
                    break "client-disconnected";
                }
            }
            Ok(Poll::Done) => break "completed",
            Err(error) => {
                let _ = sender.blocking_send(Err(format!("stream provider failed: {error:#}")));
                break "provider-error";
            }
        }
    };
    close(&provider, session_id, reason);
}

fn wait_pending(delay: Duration, sender: &mpsc::Sender<Result<Vec<u8>, String>>) -> bool {
    let delay = delay.clamp(MIN_POLL_DELAY, MAX_POLL_DELAY);
    let started = Instant::now();
    while started.elapsed() < delay {
        if sender.is_closed() {
            return true;
        }
        std::thread::sleep(
            delay
                .saturating_sub(started.elapsed())
                .min(DISCONNECT_CHECK),
        );
    }
    sender.is_closed()
}

fn close(provider: &PinnedProvider, session_id: String, reason: &str) {
    let _ = provider.call(
        SERVICE,
        "close",
        &[Val::String(session_id), Val::Enum(reason.into())],
    );
}

enum Poll {
    Chunk(Vec<u8>),
    Pending(Duration),
    Done,
}

fn parse_open(mut values: Vec<Val>) -> wasmtime::Result<Option<Opened>> {
    let value = result_value(&mut values, "open")?;
    match value {
        Val::Option(None) => Ok(None),
        Val::Option(Some(value)) => {
            let fields = record(*value, "opened HTTP stream")?;
            Ok(Some(Opened {
                status: u16_field(&fields, "status")?,
                headers: list_field(&fields, "headers")?,
                session_id: string_field(&fields, "session-id")?,
            }))
        }
        _ => Err(wasmtime::format_err!("stream open returned a non-option")),
    }
}

fn parse_poll(mut values: Vec<Val>) -> wasmtime::Result<Poll> {
    match result_value(&mut values, "poll")? {
        Val::Variant(name, value) if name == "chunk" => Ok(Poll::Chunk(bytes_field(
            &[(
                "value".into(),
                *value.ok_or_else(|| wasmtime::format_err!("missing chunk"))?,
            )],
            "value",
        )?)),
        Val::Variant(name, value) if name == "pending" => {
            match *value.ok_or_else(|| wasmtime::format_err!("missing delay"))? {
                Val::U32(ms) => Ok(Poll::Pending(Duration::from_millis(ms.into()))),
                _ => Err(wasmtime::format_err!("invalid poll delay")),
            }
        }
        Val::Variant(name, None) if name == "done" => Ok(Poll::Done),
        _ => Err(wasmtime::format_err!("invalid stream poll result")),
    }
}

fn result_value(values: &mut Vec<Val>, function: &str) -> wasmtime::Result<Val> {
    if values.len() != 1 {
        return Err(wasmtime::format_err!(
            "stream {function} returned wrong arity"
        ));
    }
    match values.remove(0) {
        Val::Result(Ok(Some(value))) => Ok(*value),
        Val::Result(Err(Some(error))) => match *error {
            Val::String(error) => Err(wasmtime::format_err!("{error}")),
            _ => Err(wasmtime::format_err!("invalid stream error")),
        },
        _ => Err(wasmtime::format_err!(
            "stream {function} returned invalid result"
        )),
    }
}
