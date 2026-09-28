//! Bounded local service journal. No remote process streams belong in this log.
use std::{
    fs::{self, File, OpenOptions},
    io::{self, Read, Write},
    path::{Path, PathBuf},
    time::{SystemTime, UNIX_EPOCH},
};

const CAPACITY: u64 = 1024 * 1024;
pub const MAX_MESSAGE: usize = 4096;

#[derive(Debug)]
pub struct Entry {
    pub cursor: String,
    pub timestamp: String,
    pub message: String,
}

pub struct Journal {
    directory: PathBuf,
    sequence: u64,
    generation: String,
}

impl Journal {
    pub fn new(directory: &Path) -> io::Result<Self> {
        fs::create_dir_all(directory)?;
        // A killed writer may leave its last record incomplete. Discard it
        // before appending, so the next generation starts at a record boundary.
        let current = directory.join("service.log");
        if current.exists() {
            let bytes = fs::read(&current)?;
            if !bytes.is_empty() && bytes.last() != Some(&b'\n') {
                let length = bytes.iter().rposition(|b| *b == b'\n').map_or(0, |i| i + 1);
                OpenOptions::new()
                    .write(true)
                    .open(current)?
                    .set_len(length as u64)?;
            }
        }
        Ok(Self {
            directory: directory.into(),
            sequence: 0,
            generation: format!("{}-{}", micros(), std::process::id()),
        })
    }

    pub fn append(&mut self, message: &str) -> io::Result<()> {
        let current = self.directory.join("service.log");
        if fs::metadata(&current).is_ok_and(|m| m.len() >= CAPACITY) {
            let previous = self.directory.join("service.log.1");
            match fs::remove_file(&previous) {
                Ok(()) => {}
                Err(e) if e.kind() == io::ErrorKind::NotFound => {}
                Err(e) => return Err(e),
            }
            fs::rename(&current, previous)?;
        }
        self.sequence += 1;
        let message: String = message
            .chars()
            .take(MAX_MESSAGE)
            .map(|c| if c.is_control() { ' ' } else { c })
            .collect();
        let line = format!(
            "{}-{}\t{}\t{}\n",
            self.generation,
            self.sequence,
            micros(),
            message
        );
        let mut file = OpenOptions::new().create(true).append(true).open(current)?;
        file.write_all(line.as_bytes())?;
        file.flush()
    }
}

fn micros() -> u128 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_micros()
}

pub fn recent(directory: &Path, cursor: &str, limit: usize) -> io::Result<Vec<Entry>> {
    if !(1..=100).contains(&limit) {
        return Err(io::Error::other("invalid log limit"));
    }
    // Rotation may happen between these opens. Retry if the cursor was in the
    // current file just before it became the previous file.
    for attempt in 0..2 {
        let mut entries = Vec::new();
        let mut seen = std::collections::HashSet::new();
        let open = |name: &str| -> io::Result<Option<File>> {
            Ok(Some(match File::open(directory.join(name)) {
                Ok(file) => file,
                Err(e) if e.kind() == io::ErrorKind::NotFound => return Ok(None),
                Err(e) => return Err(e),
            }))
        };
        let current = open("service.log")?;
        let previous = open("service.log.1")?;
        for file in [previous, current].into_iter().flatten() {
            let mut bytes = Vec::new();
            file.take(CAPACITY + (MAX_MESSAGE * 4 + 256) as u64)
                .read_to_end(&mut bytes)?;
            let text = String::from_utf8_lossy(&bytes);
            // Ignore a final partial record while the writer is appending.
            for line in text
                .split_inclusive('\n')
                .filter(|line| line.ends_with('\n'))
            {
                let mut fields = line.trim_end_matches('\n').splitn(3, '\t');
                if let (Some(cursor), Some(timestamp), Some(message)) =
                    (fields.next(), fields.next(), fields.next())
                {
                    if !seen.insert(cursor.to_owned()) {
                        continue;
                    }
                    entries.push(Entry {
                        cursor: cursor.into(),
                        timestamp: timestamp.into(),
                        message: message.into(),
                    });
                }
            }
        }
        let start = if cursor.is_empty() {
            entries.len().saturating_sub(limit)
        } else if let Some(index) = entries.iter().position(|entry| entry.cursor == cursor) {
            index + 1
        } else if attempt == 0 {
            continue;
        } else {
            0
        }; // An older cursor was rotated out: resume at the oldest retained record.
        return Ok(entries.into_iter().skip(start).take(limit).collect());
    }
    unreachable!()
}

#[cfg(test)]
#[path = "service_log/tests.rs"]
mod tests;
