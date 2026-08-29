use crate::ohrats::rc_http::types::{Header, Request, Response};
use crate::ohrats::rc_session::{lookup, types::Session};
use crate::ohrats::rc_webui::{
    shell,
    types::{AuthenticatedDocument, NavigationEntry, Principal, SidebarState},
};

pub fn handle(request: Request) -> Result<Option<Response>, String> {
    if request.method != "GET" && request.method != "HEAD" {
        return Ok(None);
    }
    if request.path != "/account" && request.path != "/devices" {
        return Ok(None);
    }
    let cookie = request
        .headers
        .iter()
        .find(|header| header.name.eq_ignore_ascii_case("cookie"))
        .map(|header| header.value.clone())
        .unwrap_or_default();
    route(
        &request.path,
        request.method == "HEAD",
        &cookie,
        lookup::find(&cookie)?,
    )
}

fn route(
    path: &str,
    head: bool,
    cookie: &str,
    session: Option<Session>,
) -> Result<Option<Response>, String> {
    let Some(session) = session else {
        return Ok(Some(redirect(&format!("/login?next={path}"))));
    };
    let title = if path == "/account" {
        "Account"
    } else {
        "Devices"
    };
    let body = if path == "/account" {
        format!(
            "<div class=\"page\"><header class=\"page-header account-header\"><div><div class=\"page-title-row\"><h1>{}</h1></div></div></header><section class=\"content-section\"><p class=\"empty-state\">Account settings are unavailable until an identity settings provider is active.</p></section></div>",
            escape(&session.principal.display_name)
        )
    } else {
        "<div class=\"page\" data-live-page=\"devices\"><header class=\"page-header\"><div><h1>Devices</h1></div></header><div class=\"data-list\" id=\"device-list\"><p class=\"empty-state\">Device data is unavailable until a device domain provider is active.</p></div></div>".into()
    };
    let sidebar = sidebar_cookie(cookie);
    let html = shell::render_authenticated(&AuthenticatedDocument {
        title: title.into(),
        principal: Principal {
            user_id: session.principal.user_id,
            display_name: session.principal.display_name,
        },
        path: path.into(),
        sidebar,
        navigation: navigation(),
        workspaces: Vec::new(),
        trusted_body: body,
        scripts: Vec::new(),
        styles: Vec::new(),
    });
    Ok(Some(html_response(html, head)))
}

fn sidebar_cookie(cookie: &str) -> SidebarState {
    if cookie
        .split(';')
        .map(str::trim)
        .any(|part| part == "rc_sidebar=closed")
    {
        SidebarState::Closed
    } else {
        SidebarState::Open
    }
}

fn navigation() -> Vec<NavigationEntry> {
    [
        ("devices", "Devices", "/devices", "icon-devices"),
        ("api", "API", "/api", "icon-api"),
        ("mcp", "MCP", "/integrations/mcp", "icon-api"),
    ]
    .into_iter()
    .map(|(id, label, path, icon)| NavigationEntry {
        id: id.into(),
        label: label.into(),
        path: path.into(),
        icon: icon.into(),
    })
    .collect()
}

fn html_response(body: String, head: bool) -> Response {
    Response {
        status: 200,
        headers: security_headers("text/html; charset=utf-8"),
        body: if head { Vec::new() } else { body.into_bytes() },
    }
}

fn redirect(location: &str) -> Response {
    Response {
        status: 303,
        headers: vec![
            header("location", location),
            header("cache-control", "no-store"),
        ],
        body: Vec::new(),
    }
}

fn security_headers(content_type: &str) -> Vec<Header> {
    vec![
        header("content-type", content_type),
        header("cache-control", "no-store"),
        header("x-content-type-options", "nosniff"),
        header("x-frame-options", "DENY"),
    ]
}

fn header(name: &str, value: &str) -> Header {
    Header {
        name: name.into(),
        value: value.into(),
    }
}
fn escape(value: &str) -> String {
    value
        .replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
        .replace('\'', "&#39;")
}

#[cfg(test)]
mod tests {
    use super::{escape, redirect, sidebar_cookie};
    use crate::ohrats::rc_webui::types::SidebarState;
    #[test]
    fn signed_out_routes_redirect_to_login_and_preserve_destination() {
        let response = redirect("/login?next=/devices");
        assert_eq!(response.status, 303);
        assert_eq!(response.headers[0].value, "/login?next=/devices");
    }
    #[test]
    fn principal_names_are_escaped() {
        assert_eq!(escape("A < B"), "A &lt; B");
    }
    #[test]
    fn sidebar_state_comes_from_its_cookie() {
        assert_eq!(
            sidebar_cookie("theme=dark; rc_sidebar=closed"),
            SidebarState::Closed
        );
        assert_eq!(sidebar_cookie("theme=dark"), SidebarState::Open);
    }
}
