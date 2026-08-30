#[derive(Clone, Copy, PartialEq, Eq)]
pub enum Kind {
    Attached,
    Managed,
    Scheduled,
}

pub struct Leases {
    kind: Kind,
    reattach_grace_ms: u32,
    terminate_grace_ms: u32,
    attachment: Option<String>,
    detached_deadline: Option<u64>,
    max_deadline: Option<u64>,
    terminate_deadline: Option<u64>,
}

impl Leases {
    pub fn new(
        now: u64,
        kind: Kind,
        reattach_grace_ms: u32,
        terminate_grace_ms: u32,
        max_runtime_ms: Option<u64>,
    ) -> Self {
        Self {
            kind,
            reattach_grace_ms,
            terminate_grace_ms,
            attachment: None,
            detached_deadline: (kind == Kind::Attached)
                .then_some(now.saturating_add(u64::from(reattach_grace_ms))),
            max_deadline: max_runtime_ms.map(|value| now.saturating_add(value)),
            terminate_deadline: None,
        }
    }

    pub fn attach(&mut self, controller: String) -> Result<(), String> {
        if self.kind != Kind::Attached {
            return Err("execution is not attachable".into());
        }
        self.attachment = Some(controller);
        self.detached_deadline = None;
        Ok(())
    }

    pub fn detach(&mut self, controller: &str, now: u64) {
        if self.attachment.as_deref() == Some(controller) {
            self.attachment = None;
            self.detached_deadline = Some(now.saturating_add(u64::from(self.reattach_grace_ms)));
        }
    }

    pub fn terminate(&mut self, now: u64) {
        self.terminate_deadline = Some(now.saturating_add(u64::from(self.terminate_grace_ms)));
    }

    pub fn expired(&mut self, now: u64) -> bool {
        let expired = self.max_deadline.is_some_and(|value| now >= value)
            || self.detached_deadline.is_some_and(|value| now >= value)
            || self.terminate_deadline.is_some_and(|value| now >= value);
        if expired {
            self.max_deadline = None;
            self.detached_deadline = None;
            self.terminate_deadline = None;
        }
        expired
    }
}

#[cfg(test)]
mod tests;
