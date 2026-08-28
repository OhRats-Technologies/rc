use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;

const DESIRED: &str = "rc.toml";
const LOCK: &str = "rc.lock";

#[derive(Default, Deserialize, Serialize)]
pub struct DesiredState {
    pub schema: u32,
    pub components: BTreeMap<String, DesiredComponent>,
}

#[derive(Deserialize, Serialize)]
pub struct DesiredComponent {
    pub spec: String,
}

#[derive(Default, Deserialize, Serialize)]
pub struct LockState {
    pub schema: u32,
    pub component: Vec<LockedComponent>,
}

#[derive(Clone, Deserialize, Serialize)]
pub struct LockedComponent {
    pub name: String,
    pub id: String,
    pub version: String,
    pub spec: String,
    pub resolved_source: String,
    pub digest: String,
}

impl DesiredState {
    pub fn load() -> Result<Self, String> {
        let value: Self = load(DESIRED)?.unwrap_or_default();
        validate_schema(value.schema, DESIRED)?;
        Ok(value)
    }

    pub fn save(&mut self) -> Result<(), String> {
        self.schema = 1;
        save(DESIRED, self)
    }
}

impl LockState {
    pub fn load() -> Result<Self, String> {
        let value: Self = load(LOCK)?.unwrap_or_default();
        validate_schema(value.schema, LOCK)?;
        Ok(value)
    }

    pub fn save(&mut self) -> Result<(), String> {
        self.schema = 1;
        self.component
            .sort_by(|left, right| left.name.cmp(&right.name));
        save(LOCK, self)
    }

    pub fn replace(&mut self, value: LockedComponent) {
        self.component.retain(|item| item.name != value.name);
        self.component.push(value);
    }

    pub fn remove(&mut self, name: &str) {
        self.component.retain(|item| item.name != name);
    }

    pub fn find(&self, name: &str) -> Option<&LockedComponent> {
        self.component.iter().find(|item| item.name == name)
    }
}

fn load<T: for<'de> Deserialize<'de>>(name: &str) -> Result<Option<T>, String> {
    let Some(bytes) = crate::ohrats::rc_plugin::state_store::read(name)? else {
        return Ok(None);
    };
    let text = String::from_utf8(bytes).map_err(|error| error.to_string())?;
    toml::from_str(&text)
        .map(Some)
        .map_err(|error| format!("invalid {name}: {error}"))
}

fn save(name: &str, value: &impl Serialize) -> Result<(), String> {
    let text = toml::to_string_pretty(value).map_err(|error| error.to_string())?;
    crate::ohrats::rc_plugin::state_store::write(name, text.as_bytes())
}

fn validate_schema(schema: u32, name: &str) -> Result<(), String> {
    if matches!(schema, 0 | 1) {
        Ok(())
    } else {
        Err(format!("unsupported {name} schema {schema}"))
    }
}
