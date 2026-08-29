wit_bindgen::generate!({ path: "../../wit", world: "websocket-provider", generate_all });

use exports::ohrats::rc_websocket::handler::{
    Guest as HandlerGuest, GuestSession, OpenedSession, Session,
};
use ohrats::rc_net::byte_stream_host::{self, Connection, Endpoint, StreamLimits};
use ohrats::rc_plugin::types::Service;
use ohrats::rc_websocket::types::{
    CloseFrame, Inbound, Message, OpenRequest, Outbound, OutboundBatch, SessionLimits,
};
use std::cell::RefCell;

struct Fixture;

impl Guest for Fixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:websocket-fixture-provider".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-websocket/handler".into(),
                version: "0.1.0".into(),
                priority: 0,
                keys: vec!["/fixture/socket".into()],
            }],
            requires: Vec::new(),
            commands: Vec::new(),
        }
    }
    fn activate() -> Result<(), String> {
        Ok(())
    }
    fn deactivate() {}
    fn invoke(_: String, _: Vec<String>) -> Result<u32, String> {
        Err("fixture has no commands".into())
    }
}

struct SessionState {
    stream: Connection,
    pending_write: Vec<u8>,
    queued: Vec<Outbound>,
    closed: bool,
}

struct FixtureSession(RefCell<SessionState>);

impl HandlerGuest for Fixture {
    type Session = FixtureSession;

    fn open(request: OpenRequest) -> Result<Option<OpenedSession>, String> {
        if request.route != "/fixture/socket" {
            return Ok(None);
        }
        // Compile-only public SSH mapping fixture: a deployment may grant exactly this
        // configured loopback stock-sshd endpoint. The internal RC bridge remains a
        // process/control-service integration, not a TCP connection through this adapter.
        let stream = byte_stream_host::connect(
            &Endpoint {
                host: "127.0.0.1".into(),
                port: 22,
            },
            StreamLimits {
                connect_timeout_ms: 1_000,
                idle_timeout_ms: 30_000,
                max_lifetime_ms: 300_000,
                max_pending_write_bytes: 65_536,
            },
        )?;
        Ok(Some(OpenedSession {
            session: Session::new(FixtureSession(RefCell::new(SessionState {
                stream,
                pending_write: Vec::new(),
                queued: Vec::new(),
                closed: false,
            }))),
            limits: SessionLimits {
                max_message_bytes: 65_536,
                max_queued_outbound_bytes: 131_072,
                idle_timeout_ms: 30_000,
                max_lifetime_ms: 300_000,
            },
        }))
    }
}

impl GuestSession for FixtureSession {
    fn inbound(&self, event: Inbound) -> Result<(), String> {
        let mut state = self.0.borrow_mut();
        if state.closed {
            return Err("session closed".into());
        }
        match event {
            Inbound::Message(Message::Binary(bytes)) => {
                if state.pending_write.len() + bytes.len() > 65_536 {
                    return Err("fixture write queue full".into());
                }
                state.pending_write.extend(bytes);
                flush_write(&mut state)?;
            }
            Inbound::Message(Message::Ping(bytes)) => {
                state.queued.push(Outbound::Message(Message::Pong(bytes)))
            }
            Inbound::Message(Message::Text(_)) | Inbound::Message(Message::Pong(_)) => {}
            Inbound::PeerClose(frame) => {
                state.stream.close_write()?;
                state
                    .queued
                    .push(Outbound::Close(frame.unwrap_or(CloseFrame {
                        code: 1000,
                        reason: "peer closed".into(),
                    })));
            }
            Inbound::Disconnected(reason) => {
                state.stream.close(&reason);
                state.closed = true;
            }
        }
        Ok(())
    }

    fn poll(&self, max_bytes: u64) -> Result<OutboundBatch, String> {
        let mut state = self.0.borrow_mut();
        if state.closed || max_bytes == 0 {
            return Ok(OutboundBatch {
                frames: Vec::new(),
                more: false,
            });
        }
        flush_write(&mut state)?;
        if state.queued.is_empty() {
            match state.stream.read(max_bytes)? {
                byte_stream_host::ReadResult::Data(bytes) => {
                    state.queued.push(Outbound::Message(Message::Binary(bytes)))
                }
                byte_stream_host::ReadResult::WouldBlock => {}
                byte_stream_host::ReadResult::Eof => {
                    state.queued.push(Outbound::Close(CloseFrame {
                        code: 1000,
                        reason: "upstream eof".into(),
                    }))
                }
            }
        }
        let frames = if state
            .queued
            .first()
            .is_some_and(|frame| payload_len(frame) <= max_bytes)
        {
            vec![state.queued.remove(0)]
        } else {
            Vec::new()
        };
        Ok(OutboundBatch {
            frames,
            more: !state.queued.is_empty(),
        })
    }

    fn close(&self, reason: String) {
        let mut state = self.0.borrow_mut();
        if !state.closed {
            state.stream.close(&reason);
            state.closed = true;
            state.queued.clear();
        }
    }
}

fn flush_write(state: &mut SessionState) -> Result<(), String> {
    if state.pending_write.is_empty() {
        return Ok(());
    }
    match state.stream.write(&state.pending_write)? {
        byte_stream_host::WriteResult::Accepted(count) => {
            let count = usize::try_from(count).map_err(|_| "write overflow")?;
            if count == 0 || count > state.pending_write.len() {
                return Err("invalid write progress".into());
            }
            state.pending_write.drain(..count);
        }
        byte_stream_host::WriteResult::WouldBlock => {}
    }
    Ok(())
}

fn payload_len(frame: &Outbound) -> u64 {
    match frame {
        Outbound::Message(Message::Text(value)) => value.len() as u64,
        Outbound::Message(Message::Binary(value))
        | Outbound::Message(Message::Ping(value))
        | Outbound::Message(Message::Pong(value)) => value.len() as u64,
        Outbound::Close(value) => value.reason.len() as u64,
    }
}

export!(Fixture);
