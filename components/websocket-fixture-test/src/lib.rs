wit_bindgen::generate!({ path: "../../wit", world: "websocket-test", generate_all });

use ohrats::rc_plugin::types::{Command, Requirement, Selection};
use ohrats::rc_websocket::{
    handler,
    types::{CloseFrame, Header, Inbound, Message, OpenRequest, Outbound},
};

struct Fixture;

impl Guest for Fixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:websocket-fixture-test".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: vec![Requirement {
                name: "ohrats:rc-websocket/handler".into(),
                version: "^0.1".into(),
                selection: Selection::Keyed,
            }],
            commands: vec![Command {
                name: "websocket-contract-test".into(),
                summary: "Exercise the WebSocket contract".into(),
                usage: "rc websocket-contract-test".into(),
            }],
        }
    }
    fn activate() -> Result<(), String> {
        Ok(())
    }
    fn deactivate() {}
    fn invoke(command: String, _: Vec<String>) -> Result<u32, String> {
        if command != "websocket-contract-test" {
            return Err("unsupported command".into());
        }
        verify()?;
        Ok(0)
    }
}

fn verify() -> Result<(), String> {
    let opened = handler::open(&OpenRequest {
        route: "/fixture/socket".into(),
        query: "device=fixture".into(),
        headers: vec![Header {
            name: "sec-websocket-protocol".into(),
            value: "fixture.v1".into(),
        }],
        principal: Some("fixture-user".into()),
    })?
    .ok_or("fixture route declined")?;
    if opened.limits.max_message_bytes == 0 {
        return Err("invalid limits".into());
    }
    opened
        .session
        .inbound(&Inbound::Message(Message::Ping(vec![1, 2])))?;
    let pong = opened.session.poll(2)?;
    if !matches!(pong.frames.as_slice(), [Outbound::Message(Message::Pong(value))] if value == &[1, 2])
    {
        return Err("ping/pong semantics failed".into());
    }
    opened
        .session
        .inbound(&Inbound::Message(Message::Binary(vec![3, 4])))?;
    opened
        .session
        .inbound(&Inbound::Message(Message::Text("metadata".into())))?;
    opened
        .session
        .inbound(&Inbound::PeerClose(Some(CloseFrame {
            code: 1000,
            reason: "done".into(),
        })))?;
    let close = opened.session.poll(4)?;
    if !matches!(close.frames.as_slice(), [Outbound::Close(frame)] if frame.code == 1000) {
        return Err("close semantics failed".into());
    }
    opened.session.close("fixture complete");
    Ok(())
}

export!(Fixture);
