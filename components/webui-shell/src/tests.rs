use super::{ShellGuest, SlotsGuest, WebUiShell};
use crate::ohrats::rc_webui::types::{
    AuthenticatedDocument, Contribution, ExtensionSlot, Principal, SidebarState,
};

fn contribution(id: &str, owner: &str, slot: ExtensionSlot, order: i32) -> Contribution {
    Contribution {
        id: id.into(),
        owner_id: owner.into(),
        slot,
        order,
        trusted_html: format!("<section data-slot=\"{id}\"></section>"),
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
    WebUiShell::register_contribution(contribution(
        "later",
        "ohrats:test",
        ExtensionSlot::Sidebar,
        20,
    ))
    .unwrap();
    WebUiShell::register_contribution(contribution(
        "first",
        "ohrats:test",
        ExtensionSlot::Sidebar,
        10,
    ))
    .unwrap();
    assert_eq!(
        WebUiShell::contributions(ExtensionSlot::Sidebar)
            .iter()
            .map(|value| value.id.as_str())
            .collect::<Vec<_>>(),
        ["first", "later"]
    );
    assert!(WebUiShell::remove_contribution("other:owner".into(), "first".into()).is_err());
    assert!(WebUiShell::remove_contribution("ohrats:test".into(), "first".into()).unwrap());
    <WebUiShell as super::Guest>::deactivate();
    assert!(WebUiShell::contributions(ExtensionSlot::Sidebar).is_empty());
}

#[test]
fn contributions_render_only_in_their_selected_slot() {
    for (id, slot) in [
        ("sidebar-slot", ExtensionSlot::Sidebar),
        ("device-slot", ExtensionSlot::DevicePanel),
        ("settings-slot", ExtensionSlot::SettingsPanel),
    ] {
        WebUiShell::register_contribution(contribution(id, "ohrats:test", slot, 0)).unwrap();
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
