use super::*;
use std::sync::atomic::{AtomicUsize, Ordering};
static NEXT: AtomicUsize = AtomicUsize::new(0);

struct Fixture(PathBuf);
impl Fixture {
    fn new() -> Self {
        let path = std::env::temp_dir().join(format!(
            "rc-service-log-{}-{}-{}",
            std::process::id(),
            micros(),
            NEXT.fetch_add(1, Ordering::Relaxed)
        ));
        fs::create_dir(&path).unwrap();
        Self(path)
    }
}
impl Drop for Fixture {
    fn drop(&mut self) {
        let _ = fs::remove_dir_all(&self.0);
    }
}

#[test]
fn tail_follow_and_restart_preserve_order() {
    let root = Fixture::new();
    let mut log = Journal::new(&root.0).unwrap();
    for n in 0..5 {
        log.append(&format!("line {n}")).unwrap();
    }
    let tail = recent(&root.0, "", 2).unwrap();
    assert_eq!(
        tail.iter().map(|e| e.message.as_str()).collect::<Vec<_>>(),
        ["line 3", "line 4"]
    );
    let cursor = &tail[1].cursor;
    assert!(recent(&root.0, cursor, 2).unwrap().is_empty());
    drop(log);
    let mut log = Journal::new(&root.0).unwrap();
    for n in 5..10 {
        log.append(&format!("line {n}")).unwrap();
    }
    let next = recent(&root.0, cursor, 2).unwrap();
    assert_eq!(
        next.iter().map(|e| e.message.as_str()).collect::<Vec<_>>(),
        ["line 5", "line 6"]
    );
    assert_eq!(
        recent(&root.0, &next[1].cursor, 2).unwrap()[0].message,
        "line 7"
    );
}

#[test]
fn rotation_is_bounded_and_follow_crosses_files() {
    let root = Fixture::new();
    let mut log = Journal::new(&root.0).unwrap();
    let text = "x".repeat(MAX_MESSAGE);
    while fs::metadata(root.0.join("service.log")).map_or(0, |m| m.len()) < CAPACITY {
        log.append(&text).unwrap();
    }
    let cursor = recent(&root.0, "", 1).unwrap().remove(0).cursor;
    log.append("after rotation").unwrap();
    assert_eq!(
        recent(&root.0, &cursor, 10).unwrap()[0].message,
        "after rotation"
    );
    assert!(fs::metadata(root.0.join("service.log.1")).unwrap().len() < CAPACITY + 20000);
    for _ in 0..550 {
        log.append(&text).unwrap();
    }
    assert_eq!(fs::read_dir(&root.0).unwrap().count(), 2);
    assert_eq!(recent(&root.0, &cursor, 3).unwrap().len(), 3);
}

#[test]
fn partial_records_are_ignored_and_messages_cannot_inject_records() {
    let root = Fixture::new();
    let mut log = Journal::new(&root.0).unwrap();
    log.append("hello\n\t\u{1b}world 🐀").unwrap();
    let entries = recent(&root.0, "", 20).unwrap();
    assert_eq!(entries.len(), 1);
    assert_eq!(entries[0].message, "hello   world 🐀");
    OpenOptions::new()
        .append(true)
        .open(root.0.join("service.log"))
        .unwrap()
        .write_all(b"partial\t123\t\xf0\x9f")
        .unwrap();
    assert_eq!(recent(&root.0, "", 20).unwrap().len(), 1);
    assert!(recent(&root.0, "", 0).is_err());
    drop(log);
    Journal::new(&root.0).unwrap().append("recovered").unwrap();
    let entries = recent(&root.0, "", 20).unwrap();
    assert_eq!(entries.len(), 2);
    assert_eq!(entries[1].message, "recovered");
}
