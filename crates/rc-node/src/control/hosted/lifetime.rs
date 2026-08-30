pub(super) fn bounded_runtime_ms(
    requested_seconds: Option<u64>,
    grant_expires_at: i64,
    now_ms: i64,
) -> Option<u64> {
    let requested = requested_seconds.map(|seconds| seconds.saturating_mul(1_000));
    let grant = (grant_expires_at != 0)
        .then(|| u64::try_from(grant_expires_at.saturating_sub(now_ms)).unwrap_or_default());
    match (requested, grant) {
        (Some(requested), Some(grant)) => Some(requested.min(grant)),
        (requested, grant) => requested.or(grant),
    }
}

#[cfg(test)]
mod tests {
    use super::bounded_runtime_ms;

    #[test]
    fn managed_runtime_cannot_outlive_its_grant() {
        assert_eq!(bounded_runtime_ms(Some(60), 11_000, 1_000), Some(10_000));
        assert_eq!(bounded_runtime_ms(Some(5), 11_000, 1_000), Some(5_000));
        assert_eq!(bounded_runtime_ms(None, 11_000, 1_000), Some(10_000));
        assert_eq!(bounded_runtime_ms(Some(5), 0, 1_000), Some(5_000));
        assert_eq!(bounded_runtime_ms(None, 0, 1_000), None);
        assert_eq!(bounded_runtime_ms(None, 999, 1_000), Some(0));
    }
}
