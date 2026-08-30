use crate::exports::ohrats::rc_process::runtime::OutputChunk;
use crate::ohrats::rc_process::types::StreamKind;
use std::collections::VecDeque;

pub(crate) struct Journal {
    chunks: VecDeque<OutputChunk>,
    next: u64,
    truncated: u64,
    limit: usize,
    bytes: usize,
}

impl Journal {
    pub(crate) fn new(limit: u32) -> Self {
        Self {
            chunks: VecDeque::new(),
            next: 0,
            truncated: 0,
            limit: limit as usize,
            bytes: 0,
        }
    }

    pub(crate) fn push(&mut self, stream: StreamKind, mut bytes: Vec<u8>) {
        if bytes.is_empty() {
            return;
        }
        let original = bytes.len() as u64;
        let mut cursor = self.next;
        self.next = self.next.saturating_add(original);
        if bytes.len() > self.limit {
            let skipped = bytes.len() - self.limit;
            bytes.drain(..skipped);
            cursor = cursor.saturating_add(skipped as u64);
        }
        self.bytes += bytes.len();
        if let Some(last) = self.chunks.back_mut()
            && last.kind == stream
            && last.cursor.saturating_add(last.bytes.len() as u64) == cursor
        {
            last.bytes.extend(bytes);
        } else {
            self.chunks.push_back(OutputChunk {
                kind: stream,
                cursor,
                bytes,
            });
        }
        while self.bytes > self.limit {
            let remove = self.bytes - self.limit;
            let Some(front) = self.chunks.front_mut() else {
                break;
            };
            if remove >= front.bytes.len() {
                self.bytes -= front.bytes.len();
                self.chunks.pop_front();
            } else {
                front.bytes.drain(..remove);
                front.cursor = front.cursor.saturating_add(remove as u64);
                self.bytes -= remove;
            }
        }
        self.truncated = self.chunks.front().map_or(self.next, |value| value.cursor);
    }

    pub(crate) fn read(&self, cursor: u64, max: usize) -> (Vec<OutputChunk>, u64, bool) {
        let mut cursor = cursor.max(self.truncated);
        let mut budget = max;
        let mut result = Vec::new();
        for chunk in &self.chunks {
            let end = chunk.cursor + chunk.bytes.len() as u64;
            if end <= cursor {
                continue;
            }
            let offset = cursor.saturating_sub(chunk.cursor) as usize;
            let count = (chunk.bytes.len() - offset).min(budget);
            if count == 0 {
                break;
            }
            result.push(OutputChunk {
                kind: chunk.kind,
                cursor,
                bytes: chunk.bytes[offset..offset + count].to_vec(),
            });
            cursor += count as u64;
            budget -= count;
            if budget == 0 {
                break;
            }
        }
        (result, cursor, cursor < self.next)
    }

    pub(crate) fn truncated(&self) -> u64 {
        self.truncated
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn truncates_partial_chunks_and_preserves_stream_boundaries() {
        let mut journal = Journal::new(6);
        journal.push(StreamKind::Stdout, b"abc".to_vec());
        journal.push(StreamKind::Stdout, b"de".to_vec());
        journal.push(StreamKind::Stderr, b"XYZ".to_vec());
        let (chunks, cursor, more) = journal.read(0, 4);
        assert_eq!(journal.truncated(), 2);
        assert_eq!(chunks[0].bytes, b"cde");
        assert_eq!(chunks[1].bytes, b"X");
        assert_eq!(cursor, 6);
        assert!(more);
    }

    #[test]
    fn oversized_append_keeps_the_tail_with_absolute_cursor() {
        let mut journal = Journal::new(3);
        journal.push(StreamKind::Stdout, b"abcdef".to_vec());
        let (chunks, cursor, more) = journal.read(0, 10);
        assert_eq!(journal.truncated(), 3);
        assert_eq!(chunks[0].cursor, 3);
        assert_eq!(chunks[0].bytes, b"def");
        assert_eq!(cursor, 6);
        assert!(!more);
    }
}
