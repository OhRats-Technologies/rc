use std::collections::HashSet;

#[derive(Default)]
pub struct Registry {
    seen: HashSet<String>,
    active: [usize; 3],
}

impl Registry {
    pub fn claim(&mut self, id: &str) -> Result<(), String> {
        if id.is_empty() {
            return Err("execution id is empty".into());
        }
        if self.seen.contains(id) {
            return Err("execution id was already claimed".into());
        }
        self.seen.insert(id.into());
        Ok(())
    }

    pub fn started(&mut self, kind: crate::leases::Kind) -> [usize; 3] {
        self.active[index(kind)] += 1;
        self.active
    }

    pub fn finished(&mut self, kind: crate::leases::Kind) -> [usize; 3] {
        self.active[index(kind)] = self.active[index(kind)].saturating_sub(1);
        self.active
    }
}

fn index(kind: crate::leases::Kind) -> usize {
    match kind {
        crate::leases::Kind::Attached => 0,
        crate::leases::Kind::Managed => 1,
        crate::leases::Kind::Scheduled => 2,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn duplicate_id_is_never_reclaimed() {
        let mut registry = Registry::default();
        registry.claim("execution-1").unwrap();
        assert_eq!(
            registry.claim("execution-1"),
            Err("execution id was already claimed".into())
        );
    }

    #[test]
    fn empty_id_is_rejected_before_native_spawn() {
        assert_eq!(
            Registry::default().claim(""),
            Err("execution id is empty".into())
        );
    }

    #[test]
    fn fresh_ids_are_not_rejected_after_a_fixed_number_of_executions() {
        let mut registry = Registry::default();
        for index in 0..65_537 {
            registry.claim(&format!("execution-{index}")).unwrap();
        }
        assert!(registry.claim("execution-0").is_err());
    }

    #[test]
    fn active_counts_are_lifetime_specific_and_saturating() {
        let mut registry = Registry::default();
        assert_eq!(registry.started(crate::leases::Kind::Managed), [0, 1, 0]);
        assert_eq!(registry.started(crate::leases::Kind::Attached), [1, 1, 0]);
        assert_eq!(registry.finished(crate::leases::Kind::Managed), [1, 0, 0]);
        assert_eq!(registry.finished(crate::leases::Kind::Managed), [1, 0, 0]);
    }
}
