use crate::ohrats::rc_storage::{
    durable_store,
    types::{Change, CommitError, Entry},
};
use sha2::{Digest as _, Sha256};
use std::collections::BTreeMap;

const BUCKET: &str = "execution-replay-v1";
const CHUNK_BYTES: usize = 4_096;
const CHUNKS: usize = 256;
const HASHES: u8 = 5;
const RETRIES: usize = 12;

pub(crate) fn claim(id: &str) -> Result<(), String> {
    if id.is_empty() {
        return Err("execution id is empty".into());
    }
    let positions = positions(id);
    for _ in 0..RETRIES {
        let revision = durable_store::revision()?;
        let mut chunks = load(&positions)?;
        if positions
            .iter()
            .all(|(chunk, bit)| chunks[chunk][bit / 8] & (1 << (bit % 8)) != 0)
        {
            return Err(format!("execution id {id:?} was already claimed"));
        }
        for (chunk, bit) in &positions {
            chunks.get_mut(chunk).expect("replay chunk exists")[bit / 8] |= 1 << (bit % 8);
        }
        let changes = chunks
            .into_iter()
            .map(|(chunk, value)| {
                Change::Put(Entry {
                    bucket: BUCKET.into(),
                    key: (chunk as u16).to_be_bytes().to_vec(),
                    value,
                })
            })
            .collect::<Vec<_>>();
        match durable_store::commit(revision, &changes) {
            Ok(_) => return Ok(()),
            Err(CommitError::Conflict(_)) => continue,
            Err(CommitError::Failure(error)) => return Err(error),
        }
    }
    Err("execution replay state changed too frequently".into())
}

fn load(positions: &[(usize, usize)]) -> Result<BTreeMap<usize, Vec<u8>>, String> {
    let mut result = BTreeMap::new();
    for (chunk, _) in positions {
        if result.contains_key(chunk) {
            continue;
        }
        let value = durable_store::get(BUCKET, &(*chunk as u16).to_be_bytes())?
            .unwrap_or_else(|| vec![0; CHUNK_BYTES]);
        if value.len() != CHUNK_BYTES {
            return Err("execution replay state is corrupt".into());
        }
        result.insert(*chunk, value);
    }
    Ok(result)
}

fn positions(id: &str) -> Vec<(usize, usize)> {
    let total_bits = CHUNKS * CHUNK_BYTES * 8;
    (0..HASHES)
        .map(|domain| {
            let digest = Sha256::digest([id.as_bytes(), &[domain]].concat());
            let index =
                (u64::from_be_bytes(digest[..8].try_into().unwrap()) % total_bits as u64) as usize;
            (index / (CHUNK_BYTES * 8), index % (CHUNK_BYTES * 8))
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn replay_positions_are_bounded_and_domain_separated() {
        let values = positions("execution-1");
        assert_eq!(values.len(), HASHES as usize);
        assert!(
            values
                .iter()
                .all(|(chunk, bit)| { *chunk < CHUNKS && *bit < CHUNK_BYTES * 8 })
        );
        assert_ne!(values, positions("execution-2"));
    }
}
