use super::StreamValue;
use crate::{
    bindings::ohrats::rc_process::process_host::{
        ByteStream, HostByteStream, ReadResult, WriteResult,
    },
    host::HostState,
};
use std::io::{Read, Write};
use wasmtime::component::Resource;

impl HostByteStream for HostState {
    fn read(&mut self, stream: Resource<ByteStream>, max_bytes: u32) -> Result<ReadResult, String> {
        let maximum = usize::try_from(max_bytes).map_err(|error| error.to_string())?;
        if maximum == 0 {
            return Ok(ReadResult::Data(Vec::new()));
        }
        let value = self.stream_mut(&stream)?;
        let mut bytes = vec![0; maximum];
        let result = match value {
            StreamValue::Reader(file) => file.read(&mut bytes),
            #[cfg(unix)]
            StreamValue::Duplex(file) => file.read(&mut bytes),
            StreamValue::Writer(_) => return Err("stream is not readable".into()),
        };
        match result {
            Ok(0) => Ok(ReadResult::Eof),
            Ok(length) => {
                bytes.truncate(length);
                Ok(ReadResult::Data(bytes))
            }
            Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                Ok(ReadResult::WouldBlock)
            }
            Err(error) => Err(error.to_string()),
        }
    }

    fn write(
        &mut self,
        stream: Resource<ByteStream>,
        bytes: Vec<u8>,
    ) -> Result<WriteResult, String> {
        let value = self.stream_mut(&stream)?;
        let result = match value {
            StreamValue::Writer(file) => file.write(&bytes),
            #[cfg(unix)]
            StreamValue::Duplex(file) => file.write(&bytes),
            StreamValue::Reader(_) => return Err("stream is not writable".into()),
        };
        match result {
            Ok(length) => {
                Ok(WriteResult::Accepted(length.try_into().map_err(|_| {
                    "accepted byte count exceeds u32".to_owned()
                })?))
            }
            Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                Ok(WriteResult::WouldBlock)
            }
            Err(error) => Err(error.to_string()),
        }
    }

    fn close_write(&mut self, stream: Resource<ByteStream>) -> Result<(), String> {
        self.runtime_handles
            .process
            .streams
            .remove(&stream.rep())
            .ok_or_else(|| "unknown byte stream".to_owned())?;
        Ok(())
    }

    fn close(&mut self, stream: Resource<ByteStream>) {
        self.runtime_handles.process.streams.remove(&stream.rep());
    }

    fn drop(&mut self, stream: Resource<ByteStream>) -> wasmtime::Result<()> {
        self.runtime_handles.process.streams.remove(&stream.rep());
        Ok(())
    }
}

impl HostState {
    fn stream_mut(&mut self, stream: &Resource<ByteStream>) -> Result<&mut StreamValue, String> {
        self.runtime_handles
            .process
            .streams
            .get_mut(&stream.rep())
            .ok_or_else(|| "unknown byte stream".into())
    }
}
