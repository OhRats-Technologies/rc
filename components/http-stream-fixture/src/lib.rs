wit_bindgen::generate!({
    path: "../../wit",
    world: "http-stream-provider",
    generate_all,
});

use exports::ohrats::rc_http::stream_handler::{
    CloseReason, Guest as StreamGuest, OpenedResponse, PollResult,
};
use ohrats::{
    rc_http::types::{Header, Request},
    rc_plugin::types::Service,
};
use std::sync::{LazyLock, Mutex};

static STATE: LazyLock<Mutex<State>> = LazyLock::new(|| Mutex::new(State::default()));

#[cfg(rc_stream_replacement)]
const GENERATION: &str = "replacement";
#[cfg(not(rc_stream_replacement))]
const GENERATION: &str = "old";

#[derive(Default)]
struct State {
    next: u64,
    sessions: Vec<(String, String, usize)>,
    closed: usize,
}

struct Fixture;

impl Guest for Fixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:http-stream-fixture".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-http/stream-handler".into(),
                version: "0.1.0".into(),
                priority: 300,
                keys: Vec::new(),
            }],
            requires: Vec::new(),
            commands: Vec::new(),
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }
    fn deactivate() {
        ohrats::rc_plugin::host::log(
            ohrats::rc_plugin::host::LogLevel::Info,
            &format!("http stream generation {GENERATION} deactivated"),
        );
    }
    fn invoke(_: String, _: Vec<String>) -> Result<u32, String> {
        Err("no commands".into())
    }
}

impl StreamGuest for Fixture {
    fn open(request: Request) -> Result<Option<OpenedResponse>, String> {
        let kind = match request.path.as_str() {
            "/events" | "/generation" | "/drain" | "/slow" | "/endless" | "/total"
            | "/oversized" | "/failure" | "/closed" => request.path,
            _ => return Ok(None),
        };
        let mut state = STATE.lock().map_err(|_| "state poisoned")?;
        state.next += 1;
        let id = state.next.to_string();
        state.sessions.push((id.clone(), kind, 0));
        Ok(Some(OpenedResponse {
            status: 200,
            headers: vec![Header {
                name: "content-type".into(),
                value: "text/event-stream".into(),
            }],
            session_id: id,
        }))
    }

    fn poll(id: String) -> Result<PollResult, String> {
        let mut state = STATE.lock().map_err(|_| "state poisoned")?;
        let closed = state.closed;
        let (_, kind, step) = state
            .sessions
            .iter_mut()
            .find(|value| value.0 == id)
            .ok_or("unknown session")?;
        let current = *step;
        *step += 1;
        match (kind.as_str(), current) {
            ("/generation", 0) => chunk(&format!("data: {GENERATION}\n\n")),
            ("/generation", _) => Ok(PollResult::Done),
            ("/drain", 0) => Ok(PollResult::Pending(1_000)),
            ("/drain", 1) => chunk("data: drained\n\n"),
            ("/drain", _) => Ok(PollResult::Done),
            ("/events", 0) => chunk("id: 1\ndata: first\n\n"),
            ("/events", 1) => Ok(PollResult::Pending(40)),
            ("/events", 2) => chunk(": heartbeat\n\n"),
            ("/events", 3) => chunk("id: 2\ndata: second\n\n"),
            ("/events", _) => Ok(PollResult::Done),
            ("/slow", 0) => Ok(PollResult::Pending(250)),
            ("/slow", 1) => chunk("data: slow\n\n"),
            ("/slow", _) => Ok(PollResult::Done),
            ("/endless", _) => Ok(PollResult::Pending(25)),
            ("/total", _) => Ok(PollResult::Chunk(vec![b't'; 32])),
            ("/oversized", 0) => Ok(PollResult::Chunk(vec![b'x'; 65_537])),
            ("/oversized", _) => Ok(PollResult::Done),
            ("/failure", 0) => chunk("data: before-error\n\n"),
            ("/failure", _) => Err("intentional poll failure".into()),
            ("/closed", 0) => chunk(&format!("data: {closed}\n\n")),
            ("/closed", _) => Ok(PollResult::Done),
            _ => Ok(PollResult::Done),
        }
    }

    fn close(id: String, _: CloseReason) {
        if let Ok(mut state) = STATE.lock() {
            state.sessions.retain(|value| value.0 != id);
            state.closed += 1;
        }
    }
}

fn chunk(value: &str) -> Result<PollResult, String> {
    Ok(PollResult::Chunk(value.as_bytes().to_vec()))
}

export!(Fixture);
