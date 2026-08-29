use std::sync::{
    Arc,
    atomic::{AtomicUsize, Ordering},
};

const DEFAULT_ACTIVE_STREAMS: usize = 128;
const MIN_ACTIVE_STREAMS: usize = 1;
const MAX_ACTIVE_STREAMS: usize = 4_096;

#[derive(Clone)]
pub(super) struct StreamLimiter {
    inner: Arc<Inner>,
}

struct Inner {
    active: AtomicUsize,
    maximum: usize,
}

impl StreamLimiter {
    pub(super) fn configured() -> anyhow::Result<Self> {
        let maximum = match std::env::var("RC_HTTP_STREAM_MAX_ACTIVE") {
            Ok(value) => value
                .parse()
                .map_err(|_| anyhow::anyhow!("RC_HTTP_STREAM_MAX_ACTIVE must be an integer"))?,
            Err(std::env::VarError::NotPresent) => DEFAULT_ACTIVE_STREAMS,
            Err(error) => return Err(error.into()),
        };
        anyhow::ensure!(
            (MIN_ACTIVE_STREAMS..=MAX_ACTIVE_STREAMS).contains(&maximum),
            "RC_HTTP_STREAM_MAX_ACTIVE must be between {MIN_ACTIVE_STREAMS} and {MAX_ACTIVE_STREAMS}"
        );
        Ok(Self::new(maximum))
    }

    fn new(maximum: usize) -> Self {
        Self {
            inner: Arc::new(Inner {
                active: AtomicUsize::new(0),
                maximum,
            }),
        }
    }

    pub(super) fn acquire(&self) -> Option<StreamPermit> {
        self.inner
            .active
            .fetch_update(Ordering::AcqRel, Ordering::Acquire, |active| {
                (active < self.inner.maximum).then_some(active + 1)
            })
            .ok()?;
        Some(StreamPermit(self.inner.clone()))
    }
}

pub(super) struct StreamPermit(Arc<Inner>);

impl Drop for StreamPermit {
    fn drop(&mut self) {
        let previous = self.0.active.fetch_sub(1, Ordering::AcqRel);
        debug_assert!(previous > 0);
    }
}

#[cfg(test)]
mod tests {
    use super::StreamLimiter;

    #[test]
    fn caps_active_permits_and_reuses_released_slots() {
        let limiter = StreamLimiter::new(2);
        let first = limiter.acquire().expect("first permit");
        let second = limiter.acquire().expect("second permit");
        assert!(limiter.acquire().is_none());
        drop(first);
        let replacement = limiter.acquire().expect("replacement permit");
        assert!(limiter.acquire().is_none());
        drop((second, replacement));
        assert!(limiter.acquire().is_some());
    }
}
