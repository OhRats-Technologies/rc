wit_bindgen::generate!({
    path: "../../wit",
    world: "webui-shell",
    generate_all,
});

mod document;
mod http;
mod pages;

include!(concat!(env!("OUT_DIR"), "/assets.rs"));

use exports::{
    ohrats::rc_http::handler::Guest as HttpGuest, ohrats::rc_webui::slots::Guest as SlotsGuest,
};
use ohrats::{
    rc_http::types::{Request, Response},
    rc_plugin::types::{Command, Service},
    rc_webui::types::Page,
};
use std::{cell::RefCell, collections::BTreeMap};

thread_local! {
    pub(crate) static PAGES: RefCell<BTreeMap<String, Page>> = const { RefCell::new(BTreeMap::new()) };
}

struct WebUiShell;

impl Guest for WebUiShell {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:webui-shell".into(),
            version: "0.1.0".into(),
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
            ],
            requires: Vec::new(),
            commands: vec![Command {
                name: "ui-pages".into(),
                summary: "List active WebUI page contributions".into(),
                usage: "rc ui-pages".into(),
            }],
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {
        PAGES.with(|pages| pages.borrow_mut().clear());
    }

    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
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
        PAGES.with(|pages| {
            pages.borrow_mut().insert(value.id.clone(), value);
        });
        Ok(())
    }

    fn remove_page(id: String) -> Result<bool, String> {
        validate_id(&id)?;
        Ok(PAGES.with(|pages| pages.borrow_mut().remove(&id).is_some()))
    }

    fn pages() -> Vec<Page> {
        PAGES.with(|pages| pages.borrow().values().cloned().collect())
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

export!(WebUiShell);
