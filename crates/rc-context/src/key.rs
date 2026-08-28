use std::{any::TypeId, fmt};

#[derive(Clone, Copy, PartialEq, Eq, Hash)]
pub struct ServiceKey {
    type_id: TypeId,
    name: &'static str,
}

impl ServiceKey {
    pub fn of<T: Send + Sync + 'static>() -> Self {
        Self::named::<T>(std::any::type_name::<T>())
    }

    pub fn named<T: Send + Sync + 'static>(name: &'static str) -> Self {
        Self {
            type_id: TypeId::of::<T>(),
            name,
        }
    }

    pub fn name(self) -> &'static str {
        self.name
    }
}

impl fmt::Debug for ServiceKey {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("ServiceKey")
            .field("name", &self.name)
            .finish_non_exhaustive()
    }
}
