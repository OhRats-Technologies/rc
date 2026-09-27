use std::{
    fs::File,
    io::{self, Read},
};

pub(super) struct PtyReader(pub File);

impl Read for PtyReader {
    fn read(&mut self, bytes: &mut [u8]) -> io::Result<usize> {
        match self.0.read(bytes) {
            // Linux PTY masters report EIO after the final slave closes.
            Err(error) if error.raw_os_error() == Some(libc::EIO) => Ok(0),
            result => result,
        }
    }
}
