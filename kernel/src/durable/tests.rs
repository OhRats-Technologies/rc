use super::{commit, get, revision, scan};
use crate::{
    bindings::ohrats::rc_storage::types::{Change, CommitError, Deletion, Entry},
    database::Database,
};

fn put(bucket: &str, key: &[u8], value: &[u8]) -> Change {
    Change::Put(Entry {
        bucket: bucket.into(),
        key: key.to_vec(),
        value: value.to_vec(),
    })
}

#[test]
fn isolates_namespaces_and_persists_values() -> anyhow::Result<()> {
    let directory = tempfile::tempdir()?;
    let path = directory.path().join("kernel.sqlite3");
    let database = Database::open(&path)?;
    assert_eq!(
        commit(&database, "alpha", 0, vec![put("items", b"key", b"value")])
            .map_err(|_| anyhow::anyhow!("initial commit failed"))?,
        1
    );
    assert_eq!(
        get(&database, "alpha", "items", b"key")?,
        Some(b"value".to_vec())
    );
    assert_eq!(get(&database, "beta", "items", b"key")?, None);
    drop(database);
    let reopened = Database::open(&path)?;
    assert_eq!(
        get(&reopened, "alpha", "items", b"key")?,
        Some(b"value".to_vec())
    );
    Ok(())
}

#[test]
fn rejects_stale_revisions_without_partial_writes() -> anyhow::Result<()> {
    let directory = tempfile::tempdir()?;
    let database = Database::open(&directory.path().join("kernel.sqlite3"))?;
    assert_eq!(
        commit(&database, "alpha", 0, vec![put("items", b"first", b"one")])
            .map_err(|_| anyhow::anyhow!("initial commit failed"))?,
        1
    );
    assert!(matches!(
        commit(&database, "alpha", 0, vec![put("items", b"second", b"two")]),
        Err(CommitError::Conflict(1))
    ));
    assert_eq!(revision(&database, "alpha")?, 1);
    assert_eq!(get(&database, "alpha", "items", b"second")?, None);
    Ok(())
}

#[test]
fn scans_prefixes_with_an_exclusive_cursor() -> anyhow::Result<()> {
    let directory = tempfile::tempdir()?;
    let database = Database::open(&directory.path().join("kernel.sqlite3"))?;
    assert_eq!(
        commit(
            &database,
            "alpha",
            0,
            vec![
                put("items", b"aa", b"one"),
                put("items", b"ab", b"two"),
                put("items", b"ba", b"three"),
            ],
        )
        .map_err(|_| anyhow::anyhow!("prefix fixture commit failed"))?,
        1
    );
    let first = scan(&database, "alpha", "items", b"a", None, 1)?;
    assert_eq!(first.entries[0].key, b"aa");
    assert_eq!(first.next_key, Some(b"aa".to_vec()));
    let second = scan(
        &database,
        "alpha",
        "items",
        b"a",
        first.next_key.as_deref(),
        1,
    )?;
    assert_eq!(second.entries[0].key, b"ab");
    assert_eq!(second.next_key, None);
    Ok(())
}

#[test]
fn validates_the_whole_commit_before_writing() -> anyhow::Result<()> {
    let directory = tempfile::tempdir()?;
    let database = Database::open(&directory.path().join("kernel.sqlite3"))?;
    let result = commit(
        &database,
        "alpha",
        0,
        vec![
            put("items", b"valid", b"one"),
            Change::Delete(Deletion {
                bucket: "items".into(),
                key: Vec::new(),
            }),
        ],
    );
    assert!(matches!(result, Err(CommitError::Failure(_))));
    assert_eq!(revision(&database, "alpha")?, 0);
    assert_eq!(get(&database, "alpha", "items", b"valid")?, None);
    Ok(())
}
