// Package webapp serves the authenticated account and device pages.
package webapp

import (
	"strings"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_http_types"
	"wit_component/ohrats_rc_session_lookup"
	sessionTypes "wit_component/ohrats_rc_session_types"
	"wit_component/ohrats_rc_webui_shell"
	"wit_component/ohrats_rc_webui_types"
)

const (
	accountPath = "/account"
	devicesPath = "/devices"
)

// Handle routes authenticated WebUI page requests.
func Handle(request ohrats_rc_http_types.Request) witTypes.Result[witTypes.Option[ohrats_rc_http_types.Response], string] {
	if request.Method != "GET" && request.Method != "HEAD" {
		return noneResponse()
	}
	if request.Path != accountPath && request.Path != devicesPath {
		return noneResponse()
	}
	cookie := cookieHeader(request)
	session := ohrats_rc_session_lookup.Find(cookie)
	if session.IsErr() {
		return witTypes.Err[witTypes.Option[ohrats_rc_http_types.Response], string](session.Err())
	}
	option := session.Ok()
	var found *sessionTypes.Session
	if option.IsSome() {
		value := option.Some()
		found = &value
	}
	return route(request.Path, request.Method == "HEAD", cookie, found)
}

func route(path string, head bool, cookie string, session *sessionTypes.Session) witTypes.Result[witTypes.Option[ohrats_rc_http_types.Response], string] {
	if session == nil {
		location := "/login?next=" + path
		return someResponse(ohrats_rc_http_types.Response{
			Status: 303,
			Headers: []ohrats_rc_http_types.Header{
				header("location", location),
				header("cache-control", "no-store"),
			},
			Body: []byte{},
		})
	}
	title := "Devices"
	body := `<div class="page" data-live-page="devices"><header class="page-header"><div><h1>Devices</h1></div></header><div class="data-list" id="device-list"><p class="empty-state">Device data is unavailable until a device domain provider is active.</p></div></div>`
	if path == accountPath {
		title = "Account"
		body = `<div class="page"><header class="page-header account-header"><div><div class="page-title-row"><h1>` +
			escape(session.Principal.DisplayName) +
			`</h1></div></div></header><section class="content-section"><p class="empty-state">Account settings are unavailable until an identity settings provider is active.</p></section></div>`
	}
	html := ohrats_rc_webui_shell.RenderAuthenticated(ohrats_rc_webui_types.AuthenticatedDocument{
		Title: title,
		Principal: ohrats_rc_webui_types.Principal{
			UserId:      session.Principal.UserId,
			DisplayName: session.Principal.DisplayName,
		},
		Path:        path,
		Sidebar:     sidebarState(cookie),
		Navigation:  navigation(),
		Workspaces:  []ohrats_rc_webui_types.WorkspaceSummary{},
		TrustedBody: body,
		Scripts:     []string{},
		Styles:      []string{},
	})
	return someResponse(htmlResponse(html, head))
}

func cookieHeader(request ohrats_rc_http_types.Request) string {
	for _, header := range request.Headers {
		if strings.EqualFold(header.Name, "cookie") {
			return header.Value
		}
	}
	return ""
}

func sidebarState(cookie string) ohrats_rc_webui_types.SidebarState {
	for _, part := range strings.Split(cookie, ";") {
		if strings.TrimSpace(part) == "rc_sidebar=closed" {
			return ohrats_rc_webui_types.SidebarState_Closed
		}
	}
	return ohrats_rc_webui_types.SidebarState_Open
}

func navigation() []ohrats_rc_webui_types.NavigationEntry {
	entries := []ohrats_rc_webui_types.NavigationEntry{
		{Id: "devices", Label: "Devices", Path: devicesPath, Icon: "icon-devices"},
		{Id: "api", Label: "API", Path: "/api", Icon: "icon-api"},
		{Id: "mcp", Label: "MCP", Path: "/integrations/mcp", Icon: "icon-api"},
	}
	return entries
}

func htmlResponse(body string, head bool) ohrats_rc_http_types.Response {
	value := ohrats_rc_http_types.Response{
		Status:  200,
		Headers: securityHeaders("text/html; charset=utf-8"),
	}
	if head {
		value.Body = []byte{}
	} else {
		value.Body = []byte(body)
	}
	return value
}

func securityHeaders(contentType string) []ohrats_rc_http_types.Header {
	return []ohrats_rc_http_types.Header{
		header("content-type", contentType),
		header("cache-control", "no-store"),
		header("x-content-type-options", "nosniff"),
		header("x-frame-options", "DENY"),
	}
}

func header(name string, value string) ohrats_rc_http_types.Header {
	return ohrats_rc_http_types.Header{Name: name, Value: value}
}

func noneResponse() witTypes.Result[witTypes.Option[ohrats_rc_http_types.Response], string] {
	return witTypes.Ok[witTypes.Option[ohrats_rc_http_types.Response], string](
		witTypes.None[ohrats_rc_http_types.Response](),
	)
}

func someResponse(value ohrats_rc_http_types.Response) witTypes.Result[witTypes.Option[ohrats_rc_http_types.Response], string] {
	return witTypes.Ok[witTypes.Option[ohrats_rc_http_types.Response], string](
		witTypes.Some(value),
	)
}

var htmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#39;",
)

func escape(value string) string {
	return htmlEscaper.Replace(value)
}
