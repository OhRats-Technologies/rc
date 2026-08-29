use crate::bindings::Descriptor;
use crate::bindings::ohrats::rc_plugin::types::{Command, Requirement, Selection, Service};
use anyhow::Context as _;
use semver::{Version, VersionReq};
use std::collections::{BTreeMap, BTreeSet};
use wasmtime::{Engine, component::types::ComponentItem};

#[derive(Debug, Clone)]
pub struct ValidatedService {
    pub name: String,
    pub version: Version,
    pub priority: i32,
    pub interface: String,
    pub functions: Vec<String>,
    pub keys: Vec<String>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SelectionMode {
    Single,
    Keyed,
}

#[derive(Debug, Clone)]
pub struct ValidatedRequirement {
    pub name: String,
    pub version: VersionReq,
    pub interface: String,
    pub functions: Vec<String>,
    pub selection: SelectionMode,
}

#[derive(Debug, Clone)]
pub struct ValidatedCommand {
    pub name: String,
    pub summary: String,
    pub usage: String,
}

#[derive(Debug, Clone)]
pub struct ValidatedDescriptor {
    pub id: String,
    pub version: Version,
    pub provides: Vec<ValidatedService>,
    pub requires: Vec<ValidatedRequirement>,
    pub commands: Vec<ValidatedCommand>,
}

pub fn validate(
    value: Descriptor,
    component: &wasmtime::component::Component,
    engine: &Engine,
) -> anyhow::Result<ValidatedDescriptor> {
    validate_name(&value.id, "component id")?;
    let version = Version::parse(&value.version).context("invalid component version")?;
    let (imports, exports) = typed_interfaces(component, engine);
    let provides = value
        .provides
        .into_iter()
        .map(|service| validate_service(service, &exports))
        .collect::<anyhow::Result<Vec<_>>>()?;
    let requires = value
        .requires
        .into_iter()
        .map(|requirement| validate_requirement(requirement, &imports))
        .collect::<anyhow::Result<Vec<_>>>()?;
    let commands = value
        .commands
        .into_iter()
        .map(validate_command)
        .collect::<anyhow::Result<Vec<_>>>()?;
    ensure_unique(provides.iter().map(|value| value.name.as_str()), "service")?;
    ensure_unique(
        requires.iter().map(|value| value.name.as_str()),
        "requirement",
    )?;
    ensure_unique(commands.iter().map(|value| value.name.as_str()), "command")?;
    ensure_declared(
        &imports,
        requires.iter().map(|value| value.interface.as_str()),
        "import",
    )?;
    ensure_declared(
        &exports,
        provides.iter().map(|value| value.interface.as_str()),
        "export",
    )?;
    Ok(ValidatedDescriptor {
        id: value.id,
        version,
        provides,
        requires,
        commands,
    })
}

fn validate_service(
    value: Service,
    interfaces: &BTreeMap<String, Vec<String>>,
) -> anyhow::Result<ValidatedService> {
    validate_name(&value.name, "service name")?;
    let version = Version::parse(&value.version).context("invalid service version")?;
    let interface = format!("{}@{}", value.name, version);
    let functions = interfaces
        .get(&interface)
        .with_context(|| format!("service {interface} is not exported by the component"))?
        .clone();
    let mut keys = value.keys;
    keys.sort();
    keys.dedup();
    anyhow::ensure!(
        keys.iter().all(|key| valid_key(key)),
        "service {} has an invalid selection key",
        value.name
    );
    Ok(ValidatedService {
        name: value.name,
        version,
        priority: value.priority,
        interface,
        functions,
        keys,
    })
}

fn validate_requirement(
    value: Requirement,
    interfaces: &BTreeMap<String, Vec<String>>,
) -> anyhow::Result<ValidatedRequirement> {
    validate_name(&value.name, "requirement name")?;
    let version = VersionReq::parse(&value.version).context("invalid service requirement")?;
    let (interface, functions) = interfaces
        .iter()
        .find(|(name, _)| interface_matches(name, &value.name, &version))
        .with_context(|| {
            format!(
                "required service {} {} is not imported",
                value.name, version
            )
        })?;
    Ok(ValidatedRequirement {
        name: value.name,
        version,
        interface: interface.clone(),
        functions: functions.clone(),
        selection: match value.selection {
            Selection::Single => SelectionMode::Single,
            Selection::Keyed => SelectionMode::Keyed,
        },
    })
}

fn typed_interfaces(
    component: &wasmtime::component::Component,
    engine: &Engine,
) -> (BTreeMap<String, Vec<String>>, BTreeMap<String, Vec<String>>) {
    let ty = component.component_type();
    let imports = collect_interfaces(ty.imports(engine), engine, true);
    let exports = collect_interfaces(ty.exports(engine), engine, false);
    (imports, exports)
}

fn collect_interfaces<'a>(
    items: impl Iterator<Item = (&'a str, wasmtime::component::types::ComponentExtern<'a>)>,
    engine: &Engine,
    imports: bool,
) -> BTreeMap<String, Vec<String>> {
    items
        .filter_map(|(name, item)| match item.ty {
            ComponentItem::ComponentInstance(instance) if domain_interface(name, imports) => {
                let functions = instance
                    .exports(engine)
                    .filter_map(|(name, item)| {
                        matches!(item.ty, ComponentItem::ComponentFunc(_)).then(|| name.to_owned())
                    })
                    .collect::<Vec<_>>();
                (!functions.is_empty()).then(|| (name.to_owned(), functions))
            }
            _ => None,
        })
        .collect()
}

fn domain_interface(name: &str, imports: bool) -> bool {
    name.starts_with("ohrats:") && !(imports && kernel_interface(name))
}

fn kernel_interface(name: &str) -> bool {
    [
        "ohrats:rc-plugin/host@",
        "ohrats:rc-plugin/call-context@",
        "ohrats:rc-plugin/component-store@",
        "ohrats:rc-plugin/artifact-cache@",
        "ohrats:rc-plugin/state-store@",
        "ohrats:rc-plugin/local-files@",
        "ohrats:rc-plugin/catalog-store@",
        "ohrats:rc-plugin/service-registry@",
        "ohrats:rc-plugin/http-client@",
        "ohrats:rc-storage/durable-store@",
        "ohrats:rc-updater/artifact-source@",
        "ohrats:rc-updater/native-replacement@",
    ]
    .iter()
    .any(|prefix| name.starts_with(prefix))
}

fn interface_matches(interface: &str, name: &str, requirement: &VersionReq) -> bool {
    interface
        .strip_prefix(name)
        .and_then(|suffix| suffix.strip_prefix('@'))
        .and_then(|version| Version::parse(version).ok())
        .is_some_and(|version| requirement.matches(&version))
}

fn ensure_declared<'a>(
    actual: &BTreeMap<String, Vec<String>>,
    declared: impl IntoIterator<Item = &'a str>,
    label: &str,
) -> anyhow::Result<()> {
    let declared = declared.into_iter().collect::<BTreeSet<_>>();
    for interface in actual.keys() {
        anyhow::ensure!(
            declared.contains(interface.as_str()),
            "undeclared typed {label} {interface}"
        );
    }
    Ok(())
}

fn validate_command(value: Command) -> anyhow::Result<ValidatedCommand> {
    anyhow::ensure!(
        !value.name.is_empty()
            && value.name.len() <= 64
            && value
                .name
                .bytes()
                .all(|byte| { byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'-' }),
        "invalid command name {:?}",
        value.name
    );
    anyhow::ensure!(!value.summary.trim().is_empty(), "command summary is empty");
    anyhow::ensure!(!value.usage.trim().is_empty(), "command usage is empty");
    Ok(ValidatedCommand {
        name: value.name,
        summary: value.summary,
        usage: value.usage,
    })
}

fn validate_name(value: &str, label: &str) -> anyhow::Result<()> {
    anyhow::ensure!(
        !value.is_empty()
            && value.len() <= 128
            && value.bytes().all(|byte| {
                byte.is_ascii_alphanumeric() || matches!(byte, b':' | b'/' | b'.' | b'-' | b'_')
            }),
        "invalid {label} {value:?}"
    );
    Ok(())
}

fn valid_key(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 64
        && value.bytes().all(|byte| {
            byte.is_ascii_lowercase() || byte.is_ascii_digit() || matches!(byte, b'-' | b'_')
        })
}

fn ensure_unique<'a>(values: impl IntoIterator<Item = &'a str>, label: &str) -> anyhow::Result<()> {
    let mut seen = BTreeSet::new();
    for value in values {
        anyhow::ensure!(seen.insert(value), "duplicate {label} {value:?}");
    }
    Ok(())
}
