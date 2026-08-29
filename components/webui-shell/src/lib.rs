wit_bindgen::generate!({
    path: "../../wit",
    world: "webui-shell",
    generate_all,
});

mod config;
mod document;
mod http;
mod pages;
mod sidebar;

include!(concat!(env!("OUT_DIR"), "/assets.rs"));

use exports::{
    ohrats::rc_http::handler::Guest as HttpGuest,
    ohrats::rc_webui::{shell::Guest as ShellGuest, slots::Guest as SlotsGuest},
};
#[cfg(not(test))]
use ohrats::rc_plugin::call_context;
use ohrats::{
    rc_http::types::{Request, Response},
    rc_plugin::types::{Command, Service},
    rc_webui::types::{AuthenticatedDocument, Contribution, ExtensionSlot, Page, PublicDocument},
};
use std::{cell::RefCell, collections::BTreeMap};

#[derive(Clone)]
pub(crate) struct Owned<T> {
    owner: String,
    value: T,
}

thread_local! {
    pub(crate) static PAGES: RefCell<BTreeMap<String, Owned<Page>>> = const { RefCell::new(BTreeMap::new()) };
    static CONTRIBUTIONS: RefCell<BTreeMap<String, Owned<Contribution>>> = const { RefCell::new(BTreeMap::new()) };
    #[cfg(test)]
    static TEST_CALLER: RefCell<Option<String>> = const { RefCell::new(None) };
}

struct WebUiShell;

impl Guest for WebUiShell {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:webui-shell".into(),
            version: "0.2.0".into(),
            provides: vec![
                Service {
                    name: "ohrats:rc-http/handler".into(),
                    version: "0.1.0".into(),
                    priority: 100,
                    keys: Vec::new(),
                },
                Service {
                    name: "ohrats:rc-webui/slots".into(),
                    version: "0.1.0".into(),
                    priority: 100,
                    keys: Vec::new(),
                },
                Service {
                    name: "ohrats:rc-webui/shell".into(),
                    version: "0.1.0".into(),
                    priority: 100,
                    keys: Vec::new(),
                },
            ],
            requires: Vec::new(),
            commands: vec![
                Command {
                    name: "ui-pages".into(),
                    summary: "List active WebUI page contributions".into(),
                    usage: "rc ui-pages".into(),
                },
                Command {
                    name: "webui-config".into(),
                    summary: "Read or change WebUI deployment configuration".into(),
                    usage: "rc webui-config [public-signup BOOL|public-url URL|auto]".into(),
                },
            ],
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {
        PAGES.with(|pages| pages.borrow_mut().clear());
        CONTRIBUTIONS.with(|values| values.borrow_mut().clear());
    }

    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        if command == "webui-config" {
            return config::invoke(&args);
        }
        if command != "ui-pages" || !args.is_empty() {
            return Err("usage: rc ui-pages".into());
        }
        for page in Self::pages() {
            println!("{}\t{}\t{}", page.id, page.path, page.title);
            println!("  {}", page.summary);
        }
        Ok(0)
    }
}

impl HttpGuest for WebUiShell {
    fn handle(value: Request) -> Result<Option<Response>, String> {
        http::handle(value)
    }
}

impl SlotsGuest for WebUiShell {
    fn register_page(value: Page) -> Result<(), String> {
        validate_page(&value)?;
        let owner = caller()?;
        PAGES.with(|pages| {
            insert_owned(
                &mut pages.borrow_mut(),
                owner,
                value.id.clone(),
                value,
                "page",
            )
        })
    }

    fn remove_page(id: String) -> Result<bool, String> {
        validate_id(&id)?;
        let owner = caller()?;
        PAGES.with(|pages| remove_owned(&mut pages.borrow_mut(), &owner, &id, "page"))
    }

    fn pages() -> Vec<Page> {
        PAGES.with(|pages| {
            pages
                .borrow()
                .values()
                .map(|entry| entry.value.clone())
                .collect()
        })
    }

    fn register_contribution(value: Contribution) -> Result<(), String> {
        validate_id(&value.id)?;
        if value.trusted_html.len() > 16 * 1024 {
            return Err("WebUI contribution is too large".into());
        }
        let owner = caller()?;
        CONTRIBUTIONS.with(|values| {
            insert_owned(
                &mut values.borrow_mut(),
                owner,
                value.id.clone(),
                value,
                "contribution",
            )
        })
    }

    fn remove_contribution(id: String) -> Result<bool, String> {
        validate_id(&id)?;
        let owner = caller()?;
        CONTRIBUTIONS
            .with(|values| remove_owned(&mut values.borrow_mut(), &owner, &id, "contribution"))
    }

    fn contributions(slot: ExtensionSlot) -> Vec<Contribution> {
        let mut values = CONTRIBUTIONS.with(|all| {
            all.borrow()
                .values()
                .map(|entry| &entry.value)
                .filter(|value| value.slot == slot)
                .cloned()
                .collect::<Vec<_>>()
        });
        values.sort_by(|left, right| (left.order, &left.id).cmp(&(right.order, &right.id)));
        values
    }
}

fn validate_page(value: &Page) -> Result<(), String> {
    validate_id(&value.id)?;
    if value.title.is_empty() || value.title.len() > 96 {
        return Err("WebUI page title must contain 1 to 96 bytes".into());
    }
    if !value.path.starts_with('/') || value.path.len() > 256 || value.path.contains("..") {
        return Err("invalid WebUI page path".into());
    }
    if value.summary.len() > 256 || value.content.len() > 16 * 1024 {
        return Err("WebUI page contribution is too large".into());
    }
    Ok(())
}

fn validate_id(value: &str) -> Result<(), String> {
    if !value.is_empty()
        && value.len() <= 96
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'-' | b'_'))
    {
        Ok(())
    } else {
        Err(format!("invalid WebUI page id {value:?}"))
    }
}

fn caller() -> Result<String, String> {
    #[cfg(test)]
    let value = TEST_CALLER.with(|caller| caller.borrow().clone());
    #[cfg(not(test))]
    let value = call_context::caller_component_id();
    value.ok_or_else(|| "WebUI registration requires a component caller".into())
}

fn insert_owned<T>(
    values: &mut BTreeMap<String, Owned<T>>,
    owner: String,
    id: String,
    value: T,
    kind: &str,
) -> Result<(), String> {
    if values.get(&id).is_some_and(|entry| entry.owner != owner) {
        return Err(format!("WebUI {kind} ID belongs to another component"));
    }
    values.insert(id, Owned { owner, value });
    Ok(())
}

fn remove_owned<T>(
    values: &mut BTreeMap<String, Owned<T>>,
    owner: &str,
    id: &str,
    kind: &str,
) -> Result<bool, String> {
    if values.get(id).is_some_and(|entry| entry.owner != owner) {
        return Err(format!("WebUI {kind} belongs to another component"));
    }
    Ok(values.remove(id).is_some())
}

impl ShellGuest for WebUiShell {
    fn render_public(value: PublicDocument) -> String {
        document::public(value)
    }

    fn render_authenticated(mut value: AuthenticatedDocument) -> String {
        let panel_slot = if value.path.starts_with("/devices/") {
            Some(ExtensionSlot::DevicePanel)
        } else if value.path == "/account" || value.path.starts_with("/settings") {
            Some(ExtensionSlot::SettingsPanel)
        } else {
            None
        };
        let sidebar_additions = Self::contributions(ExtensionSlot::Sidebar)
            .into_iter()
            .map(|entry| entry.trusted_html)
            .collect::<String>();
        if let Some(slot) = panel_slot {
            for contribution in Self::contributions(slot) {
                value.trusted_body.push_str(&contribution.trusted_html);
            }
        }
        document::authenticated(value, &sidebar_additions)
    }
}

export!(WebUiShell);

#[cfg(test)]
mod tests;
