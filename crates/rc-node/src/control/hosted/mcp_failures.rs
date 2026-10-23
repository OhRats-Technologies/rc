use std::{
    collections::VecDeque,
    time::{Duration, Instant},
};

const RETENTION: Duration = Duration::from_secs(300);
const MAX_FAILURES: usize = 256;

// Keep only fixed diagnostic categories, never runtime error strings that may
// contain commands, paths, output, or credentials. These are failed starts,
// not live executions; discarding old diagnostics never evicts live work.
#[derive(Clone, Copy, Debug, thiserror::Error)]
pub(crate) enum StartFailure {
    #[error("Node could not start process: execution registry capacity reached")]
    Capacity,
    #[error("Node could not start process: execution runtime is unavailable")]
    Unavailable,
    #[error("Node could not start process: execution ID was already claimed")]
    Claimed,
    #[error("Node could not start process: process policy rejected startup")]
    Policy,
    #[error("Node could not start process: execution runtime rejected startup")]
    Runtime,
}

impl StartFailure {
    pub(super) fn runtime(error: &std::io::Error) -> Self {
        let message = error.to_string();
        if message.contains("registry capacity reached") {
            Self::Capacity
        } else if message == "execution runtime is unavailable" {
            Self::Unavailable
        } else if message.contains("was already claimed") {
            Self::Claimed
        } else {
            Self::Runtime
        }
    }
}

struct Entry {
    process_id: String,
    user_id: String,
    grant_id: String,
    failure: StartFailure,
    expires: Instant,
}

#[derive(Default)]
pub(crate) struct StartFailures(VecDeque<Entry>);

impl StartFailures {
    pub(super) fn record(
        &mut self,
        process_id: &str,
        user_id: &str,
        grant_id: &str,
        failure: StartFailure,
        now: Instant,
    ) {
        self.0.retain(|entry| entry.expires > now);
        while self.0.len() >= MAX_FAILURES {
            self.0.pop_front();
        }
        self.0.push_back(Entry {
            process_id: process_id.into(),
            user_id: user_id.into(),
            grant_id: grant_id.into(),
            failure,
            expires: now + RETENTION,
        });
    }

    pub(super) fn get(
        &mut self,
        process_id: &str,
        user_id: &str,
        grant_id: &str,
        now: Instant,
    ) -> Option<StartFailure> {
        self.0.retain(|entry| entry.expires > now);
        self.0
            .iter()
            .find(|entry| {
                entry.process_id == process_id
                    && entry.user_id == user_id
                    && entry.grant_id == grant_id
            })
            .map(|entry| entry.failure)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn startup_failure_is_bound_to_user_grant_and_retention() {
        let now = Instant::now();
        let mut failures = StartFailures::default();
        failures.record("process", "owner", "grant", StartFailure::Capacity, now);
        assert!(failures.get("process", "other", "grant", now).is_none());
        assert!(failures.get("process", "owner", "other", now).is_none());
        assert!(matches!(
            failures.get("process", "owner", "grant", now),
            Some(StartFailure::Capacity)
        ));
        assert!(
            failures
                .get("process", "owner", "grant", now + RETENTION)
                .is_none()
        );
    }

    #[test]
    fn only_fixed_error_categories_are_retained() {
        let secret = std::io::Error::other("private command and credentials");
        assert_eq!(
            StartFailure::runtime(&secret).to_string(),
            "Node could not start process: execution runtime rejected startup"
        );
        let capacity = std::io::Error::other("execution registry capacity reached");
        assert!(matches!(
            StartFailure::runtime(&capacity),
            StartFailure::Capacity
        ));
    }

    #[test]
    fn only_failed_start_metadata_is_bounded() {
        let now = Instant::now();
        let mut failures = StartFailures::default();
        for index in 0..=MAX_FAILURES {
            failures.record(
                &index.to_string(),
                "owner",
                "grant",
                StartFailure::Runtime,
                now,
            );
        }
        assert_eq!(failures.0.len(), MAX_FAILURES);
        assert!(failures.get("0", "owner", "grant", now).is_none());
        assert!(
            failures
                .get(&MAX_FAILURES.to_string(), "owner", "grant", now)
                .is_some()
        );
    }
}
