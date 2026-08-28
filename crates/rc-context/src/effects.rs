use std::{future::Future, pin::Pin};

type CleanupFuture = Pin<Box<dyn Future<Output = ()> + Send + 'static>>;
type Cleanup = Box<dyn FnOnce() -> CleanupFuture + Send + 'static>;

#[must_use = "effect scopes must be reverted or transferred to an active component"]
#[derive(Default)]
pub struct EffectScope {
    cleanups: Vec<Cleanup>,
}

impl EffectScope {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn defer(&mut self, cleanup: impl FnOnce() + Send + 'static) {
        self.defer_async(move || async move { cleanup() });
    }

    pub fn defer_async<F, Fut>(&mut self, cleanup: F)
    where
        F: FnOnce() -> Fut + Send + 'static,
        Fut: Future<Output = ()> + Send + 'static,
    {
        self.cleanups.push(Box::new(move || Box::pin(cleanup())));
    }

    pub fn append(&mut self, mut other: Self) {
        self.cleanups.append(&mut other.cleanups);
    }

    pub fn is_empty(&self) -> bool {
        self.cleanups.is_empty()
    }

    pub async fn revert(&mut self) {
        while let Some(cleanup) = self.cleanups.pop() {
            cleanup().await;
        }
    }
}
