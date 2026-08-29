use super::{ShellGuest, SlotsGuest, WebUiShell};
use crate::ohrats::rc_webui::types::{
    AuthenticatedDocument, Contribution, ExtensionSlot, Page, Principal, SidebarState,
};

fn as_caller<T>(caller: Option<&str>, action: impl FnOnce() -> T) -> T {
    super::TEST_CALLER.with(|current| current.replace(caller.map(str::to_owned)));
    let result = action();
    super::TEST_CALLER.with(|current| current.replace(None));
    result
}

fn contribution(id: &str, slot: ExtensionSlot, order: i32) -> Contribution {
    Contribution {
        id: id.into(),
        slot,
        order,
        trusted_html: format!("<section data-slot=\"{id}\"></section>"),
    }
}

fn page(title: &str) -> Page {
    Page {
        id: "owned-page".into(),
        title: title.into(),
        path: "/owned-page".into(),
        summary: "Owned page".into(),
        content: "trusted".into(),
    }
}

fn document(path: &str) -> AuthenticatedDocument {
    AuthenticatedDocument {
        title: "Test".into(),
        principal: Principal {
            user_id: "user".into(),
            display_name: "Test User".into(),
        },
        path: path.into(),
        sidebar: SidebarState::Open,
        navigation: Vec::new(),
        workspaces: Vec::new(),
        trusted_body: "<main data-page-body></main>".into(),
        scripts: Vec::new(),
        styles: Vec::new(),
    }
}

#[test]
fn contribution_lifecycle_is_owned_ordered_and_cleared() {
    assert!(
        as_caller(None, || WebUiShell::register_contribution(contribution(
            "kernel",
            ExtensionSlot::Sidebar,
            0
        )))
        .is_err()
    );
    as_caller(Some("ohrats:caller-a"), || {
        WebUiShell::register_contribution(contribution("later", ExtensionSlot::Sidebar, 20))
            .unwrap();
        WebUiShell::register_contribution(contribution("first", ExtensionSlot::Sidebar, 10))
            .unwrap();
    });
    assert_eq!(
        WebUiShell::contributions(ExtensionSlot::Sidebar)
            .iter()
            .map(|value| value.id.as_str())
            .collect::<Vec<_>>(),
        ["first", "later"]
    );
    assert!(
        as_caller(Some("ohrats:caller-b"), || {
            WebUiShell::register_contribution(contribution("first", ExtensionSlot::Sidebar, 5))
        })
        .is_err()
    );
    as_caller(Some("ohrats:caller-a"), || {
        WebUiShell::register_contribution(contribution("first", ExtensionSlot::Sidebar, 30))
    })
    .unwrap();
    assert_eq!(
        WebUiShell::contributions(ExtensionSlot::Sidebar)
            .iter()
            .map(|value| value.id.as_str())
            .collect::<Vec<_>>(),
        ["later", "first"]
    );
    assert!(
        as_caller(Some("ohrats:caller-b"), || WebUiShell::remove_contribution(
            "first".into()
        ))
        .is_err()
    );
    assert!(as_caller(None, || WebUiShell::remove_contribution("first".into())).is_err());
    assert!(
        as_caller(Some("ohrats:caller-a"), || WebUiShell::remove_contribution(
            "first".into()
        ))
        .unwrap()
    );
    <WebUiShell as super::Guest>::deactivate();
    assert!(WebUiShell::contributions(ExtensionSlot::Sidebar).is_empty());
}

#[test]
fn page_lifecycle_is_caller_owned_and_kernel_calls_are_rejected() {
    assert!(as_caller(None, || WebUiShell::register_page(page("Kernel"))).is_err());
    as_caller(Some("ohrats:caller-a"), || {
        WebUiShell::register_page(page("First"))
    })
    .unwrap();
    assert!(
        as_caller(Some("ohrats:caller-b"), || WebUiShell::register_page(page(
            "Stolen"
        )))
        .is_err()
    );
    as_caller(Some("ohrats:caller-a"), || {
        WebUiShell::register_page(page("Replaced"))
    })
    .unwrap();
    assert_eq!(WebUiShell::pages()[0].title, "Replaced");
    assert!(
        as_caller(Some("ohrats:caller-b"), || WebUiShell::remove_page(
            "owned-page".into()
        ))
        .is_err()
    );
    assert!(
        as_caller(Some("ohrats:caller-a"), || WebUiShell::remove_page(
            "owned-page".into()
        ))
        .unwrap()
    );
    as_caller(Some("ohrats:caller-a"), || {
        WebUiShell::register_page(page("Cleared"))
    })
    .unwrap();
    <WebUiShell as super::Guest>::deactivate();
    assert!(WebUiShell::pages().is_empty());
    assert!(as_caller(None, || WebUiShell::remove_page("missing".into())).is_err());
}

#[test]
fn contributions_render_only_in_their_selected_slot() {
    for (id, slot) in [
        ("sidebar-slot", ExtensionSlot::Sidebar),
        ("device-slot", ExtensionSlot::DevicePanel),
        ("settings-slot", ExtensionSlot::SettingsPanel),
    ] {
        as_caller(Some("ohrats:test"), || {
            WebUiShell::register_contribution(contribution(id, slot, 0))
        })
        .unwrap();
    }
    let devices = WebUiShell::render_authenticated(document("/devices"));
    assert!(devices.contains("data-slot=\"sidebar-slot\""));
    assert!(!devices.contains("data-slot=\"device-slot\""));
    let device = WebUiShell::render_authenticated(document("/devices/device-1"));
    assert!(device.contains("data-slot=\"device-slot\""));
    assert!(device.contains("data-slot=\"sidebar-slot\""));
    let settings = WebUiShell::render_authenticated(document("/account"));
    assert!(settings.contains("data-slot=\"settings-slot\""));
    assert!(settings.contains("data-slot=\"sidebar-slot\""));
    <WebUiShell as super::Guest>::deactivate();
}
