const MAX_JOURNAL_BYTES: usize = 256 * 1024 * 1024;

pub(super) fn journal(used: usize, requested: usize) -> bool {
    used.checked_add(requested)
        .is_some_and(|total| total <= MAX_JOURNAL_BYTES)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rejects_without_overflow_or_eviction() {
        assert!(journal(MAX_JOURNAL_BYTES - 1, 1));
        assert!(!journal(MAX_JOURNAL_BYTES, 1));
        assert!(!journal(usize::MAX, 1));
    }
}
