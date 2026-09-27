//! Adapt blocking OS pipes to the process host's nonblocking stream contract.
use std::{
    io::{self, Read, Write},
    sync::mpsc::{self, Receiver, SyncSender, TryRecvError, TrySendError},
};

#[cfg(windows)]
pub(super) fn adapt(value: super::StreamValue) -> super::StreamValue {
    use super::StreamValue;
    match value {
        StreamValue::Reader(reader) => StreamValue::Reader(Box::new(Reader::new(reader))),
        StreamValue::Writer(writer) => StreamValue::Writer(Box::new(Writer::new(writer))),
    }
}

const CHUNK: usize = 16 * 1024;
const QUEUE: usize = 4;

pub(super) struct Reader {
    receiver: Receiver<io::Result<Vec<u8>>>,
    pending: std::io::Cursor<Vec<u8>>,
}

impl Reader {
    pub(super) fn new(mut source: Box<dyn Read + Send>) -> Self {
        let (sender, receiver) = mpsc::sync_channel(QUEUE);
        std::thread::spawn(move || {
            loop {
                let mut bytes = vec![0; CHUNK];
                let result = match source.read(&mut bytes) {
                    Ok(0) => break,
                    Ok(count) => {
                        bytes.truncate(count);
                        Ok(bytes)
                    }
                    Err(error) if error.kind() == io::ErrorKind::Interrupted => continue,
                    Err(error) => Err(error),
                };
                let failed = result.is_err();
                if sender.send(result).is_err() || failed {
                    break;
                }
            }
        });
        Self {
            receiver,
            pending: std::io::Cursor::new(Vec::new()),
        }
    }
}

impl Read for Reader {
    fn read(&mut self, bytes: &mut [u8]) -> io::Result<usize> {
        if bytes.is_empty() {
            return Ok(0);
        }
        if self.pending.position() as usize == self.pending.get_ref().len() {
            match self.receiver.try_recv() {
                Ok(result) => self.pending = std::io::Cursor::new(result?),
                Err(TryRecvError::Empty) => return Err(io::ErrorKind::WouldBlock.into()),
                Err(TryRecvError::Disconnected) => return Ok(0),
            }
        }
        self.pending.read(bytes)
    }
}

pub(super) struct Writer {
    sender: SyncSender<Vec<u8>>,
    error: Receiver<io::Error>,
}

impl Writer {
    pub(super) fn new(mut destination: Box<dyn Write + Send>) -> Self {
        let (sender, receiver) = mpsc::sync_channel::<Vec<u8>>(QUEUE);
        let (errors, error) = mpsc::channel();
        std::thread::spawn(move || {
            while let Ok(bytes) = receiver.recv() {
                if let Err(error) = destination.write_all(&bytes) {
                    let _ = errors.send(error);
                    break;
                }
            }
        });
        Self { sender, error }
    }
}

impl Write for Writer {
    fn write(&mut self, bytes: &[u8]) -> io::Result<usize> {
        if let Ok(error) = self.error.try_recv() {
            return Err(error);
        }
        if bytes.is_empty() {
            return Ok(0);
        }
        let count = bytes.len().min(CHUNK);
        match self.sender.try_send(bytes[..count].to_vec()) {
            Ok(()) => Ok(count),
            Err(TrySendError::Full(_)) => Err(io::ErrorKind::WouldBlock.into()),
            Err(TrySendError::Disconnected(_)) => Err(io::ErrorKind::BrokenPipe.into()),
        }
    }

    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::{sync::Arc, time::Duration};

    struct DelayedReader(Receiver<Vec<u8>>);
    impl Read for DelayedReader {
        fn read(&mut self, bytes: &mut [u8]) -> io::Result<usize> {
            match self.0.recv() {
                Ok(value) => {
                    bytes[..value.len()].copy_from_slice(&value);
                    Ok(value.len())
                }
                Err(_) => Ok(0),
            }
        }
    }

    #[test]
    fn idle_reader_returns_without_blocking_then_preserves_bytes_and_eof() {
        let (sender, receiver) = mpsc::channel();
        let mut reader = Reader::new(Box::new(DelayedReader(receiver)));
        let mut bytes = [0; 2];
        assert_eq!(
            reader.read(&mut bytes).unwrap_err().kind(),
            io::ErrorKind::WouldBlock
        );
        sender.send(vec![0, 255, 13]).unwrap();
        drop(sender);
        let mut output = Vec::new();
        for _ in 0..1000 {
            match reader.read(&mut bytes) {
                Ok(0) => {
                    assert_eq!(output, [0, 255, 13]);
                    return;
                }
                Ok(count) => output.extend_from_slice(&bytes[..count]),
                Err(error) if error.kind() == io::ErrorKind::WouldBlock => {
                    std::thread::sleep(Duration::from_millis(1))
                }
                Err(error) => panic!("{error}"),
            }
        }
        panic!("reader did not deliver EOF");
    }

    struct DelayedWriter {
        gate: Receiver<()>,
        output: Arc<std::sync::Mutex<Vec<u8>>>,
    }
    impl Write for DelayedWriter {
        fn write(&mut self, bytes: &[u8]) -> io::Result<usize> {
            self.gate.recv().unwrap();
            self.output.lock().unwrap().extend_from_slice(bytes);
            Ok(bytes.len())
        }
        fn flush(&mut self) -> io::Result<()> {
            Ok(())
        }
    }

    #[test]
    fn blocked_writer_has_bounded_backpressure_and_drains_before_eof() {
        let (gate, receiver) = mpsc::channel();
        let output = Arc::new(std::sync::Mutex::new(Vec::new()));
        let mut writer = Writer::new(Box::new(DelayedWriter {
            gate: receiver,
            output: output.clone(),
        }));
        let chunk = vec![255; CHUNK * 2];
        let mut accepted = 0;
        loop {
            match writer.write(&chunk) {
                Ok(count) => {
                    assert_eq!(count, CHUNK);
                    accepted += count;
                }
                Err(error) => {
                    assert_eq!(error.kind(), io::ErrorKind::WouldBlock);
                    break;
                }
            }
        }
        assert!(accepted <= (QUEUE + 1) * CHUNK);
        drop(writer);
        for _ in 0..accepted / CHUNK {
            gate.send(()).unwrap();
        }
        for _ in 0..1000 {
            if output.lock().unwrap().len() == accepted {
                return;
            }
            std::thread::sleep(Duration::from_millis(1));
        }
        panic!("accepted writes were not drained");
    }
}
