use std::path::Path;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ComponentState {
    Waiting,
    Active,
    Failed,
}

pub struct ComponentStatus<'a> {
    pub id: &'a str,
    pub version: String,
    pub digest: &'a str,
    pub path: &'a Path,
    pub state: ComponentState,
    pub error: Option<&'a str>,
}
