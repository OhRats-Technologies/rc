//! Bounded framing for ordered Node DataChannels with a 64 KiB SCTP limit.
//! A sender must serialize all fragments of one message before another message.
use serde::{Deserialize, Serialize};

const WIRE_LIMIT: usize = 48 * 1024;
const FRAGMENT_BYTES: usize = 8 * 1024;

#[derive(Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
struct Fragment {
    #[serde(rename = "rcFragment")]
    text: String,
    last: bool,
}

pub fn encode(message: &str) -> Result<Vec<String>, String> {
    if message.len() > crate::NODE_CONTROL_MESSAGE_LIMIT {
        return Err("Node control message exceeds transport frame capacity".into());
    }
    if message.len() <= WIRE_LIMIT {
        return Ok(vec![message.to_owned()]);
    }
    let mut result = Vec::new();
    let mut start = 0;
    while start < message.len() {
        let mut end = (start + FRAGMENT_BYTES).min(message.len());
        while !message.is_char_boundary(end) {
            end -= 1;
        }
        result.push(
            serde_json::to_string(&Fragment {
                text: message[start..end].into(),
                last: end == message.len(),
            })
            .map_err(|error| error.to_string())?,
        );
        start = end;
    }
    Ok(result)
}

#[derive(Default)]
pub struct Decoder {
    pending: String,
}

impl Decoder {
    pub fn accept(&mut self, bytes: &[u8]) -> Result<Option<Vec<u8>>, String> {
        if bytes.len() > WIRE_LIMIT {
            self.pending.clear();
            return Err("Node wire frame exceeds capacity".into());
        }
        let value: serde_json::Value = serde_json::from_slice(bytes).map_err(|e| e.to_string())?;
        if value.get("rcFragment").is_none() {
            if !self.pending.is_empty() {
                self.pending.clear();
                return Err("interleaved Node fragments".into());
            }
            return Ok(Some(bytes.to_vec()));
        }
        let fragment: Fragment = serde_json::from_value(value).map_err(|e| e.to_string())?;
        if self.pending.len().saturating_add(fragment.text.len())
            > crate::NODE_CONTROL_MESSAGE_LIMIT
        {
            self.pending.clear();
            return Err("Node reassembled frame exceeds capacity".into());
        }
        self.pending.push_str(&fragment.text);
        Ok(fragment
            .last
            .then(|| std::mem::take(&mut self.pending).into_bytes()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn large_unicode_frames_round_trip_without_interleaving_or_overflow() {
        let message = serde_json::json!({"data": "雪\\\"".repeat(30_000)}).to_string();
        let frames = encode(&message).unwrap();
        assert!(frames.len() > 1);
        let mut decoder = Decoder::default();
        for (index, frame) in frames.iter().enumerate() {
            assert!(frame.len() < WIRE_LIMIT);
            let result = decoder.accept(frame.as_bytes()).unwrap();
            if index + 1 == frames.len() {
                assert_eq!(result.unwrap(), message.as_bytes());
            } else {
                assert!(result.is_none());
            }
        }
        decoder.accept(frames[0].as_bytes()).unwrap();
        assert!(decoder.accept(b"{}").is_err());
        assert!(encode(&"x".repeat(crate::NODE_CONTROL_MESSAGE_LIMIT + 1)).is_err());
    }
}
