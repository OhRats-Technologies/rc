package webshell

import (
	"strings"

	"wit_component/ohrats_rc_webui_types"
)

// componentVersion is substituted into public snapshots as the asset cache key.
const componentVersion = "0.2.0"

const (
	publicHeadPrefix = `<!doctype html><html lang="en" data-sidebar="open"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="`
	publicHeadMiddle = `"><meta name="color-scheme" content="light dark"><title>`
	sharedHeadLinks  = ` | RC</title><meta name="description" content="Secure remote control with a small native core and hot-swappable WebAssembly components."><link rel="icon" type="image/svg+xml" href="` + sharedVisualAssets + `/logo.092a1cece4d0.svg"><link rel="stylesheet" href="` + sharedVisualAssets + `/ohrats.eb38b77e6b5e.css"><link rel="stylesheet" href="` + sharedVisualAssets + `/states.8d99d4b0e704.css"><link rel="stylesheet" href="` + sharedVisualAssets + `/copy.e4c6bbb26b56.css"><link rel="preconnect" href="https://fonts.googleapis.com"><link rel="preconnect" href="https://fonts.gstatic.com" crossorigin><link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=Space+Mono:wght@400;700&display=swap">`
	themeScript      = `<script src="` + sharedVisualAssets + `/theme.b6e0fe408633.js"></script>`
)

// RenderPublic renders an unauthenticated document shell.
func RenderPublic(value ohrats_rc_webui_types.PublicDocument) string {
	scripts := scriptTags(value.Scripts)
	styles := styleTags(value.Styles)
	robots := "noindex,nofollow"
	if value.Indexable {
		robots = "index,follow"
	}
	var builder strings.Builder
	builder.WriteString(publicHeadPrefix)
	builder.WriteString(robots)
	builder.WriteString(publicHeadMiddle)
	builder.WriteString(escape(value.Title))
	builder.WriteString(sharedHeadLinks)
	builder.WriteString(`<link rel="stylesheet" href="`)
	builder.WriteString(stylesPath)
	builder.WriteString(`"><link rel="stylesheet" href="`)
	builder.WriteString(publicStylesPath)
	builder.WriteString(`">`)
	builder.WriteString(styles)
	builder.WriteString(value.ExtraHead)
	builder.WriteString(themeScript)
	builder.WriteString(`</head><body>`)
	builder.WriteString(value.Body)
	builder.WriteString(scripts)
	builder.WriteString(`</body></html>`)
	return builder.String()
}

// RenderAuthenticated renders the authenticated shell around trusted page body HTML.
func RenderAuthenticated(value ohrats_rc_webui_types.AuthenticatedDocument) string {
	panelSlot, hasPanel := panelSlotFor(value.Path)
	sidebarAdditions := contributionsHTML(ohrats_rc_webui_types.ExtensionSlot_Sidebar)
	if hasPanel {
		value.TrustedBody += contributionsHTML(panelSlot)
	}
	state := "open"
	if value.Sidebar == ohrats_rc_webui_types.SidebarState_Closed {
		state = "closed"
	}
	scripts := scriptTags(append([]string{sidebarScriptPath}, value.Scripts...))
	styles := styleTags(value.Styles)
	var builder strings.Builder
	builder.WriteString(`<!doctype html><html lang="en" data-sidebar="`)
	builder.WriteString(state)
	builder.WriteString(`"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex,nofollow"><meta name="color-scheme" content="light dark"><title>`)
	builder.WriteString(escape(value.Title))
	builder.WriteString(sharedHeadLinks)
	builder.WriteString(`<link rel="stylesheet" href="`)
	builder.WriteString(stylesPath)
	builder.WriteString(`">`)
	builder.WriteString(styles)
	builder.WriteString(themeScript)
	builder.WriteString(`</head><body class="authenticated"><div class="site-shell">`)
	builder.WriteString(renderSidebar(value, sidebarAdditions))
	builder.WriteString(`<main class="site-content">`)
	builder.WriteString(value.TrustedBody)
	builder.WriteString(`</main></div>`)
	builder.WriteString(scripts)
	builder.WriteString(`</body></html>`)
	return builder.String()
}

// RenderRegistered renders a page contributed by an installed component.
func RenderRegistered(title string, summary string, content string) string {
	body := `<main class="page-shell"><article class="page"><p class="eyebrow">RC</p><h1>` + escape(title) +
		`</h1><p>` + escape(summary) + `</p><pre>` + escape(content) + `</pre></article></main>`
	return RenderPublic(ohrats_rc_webui_types.PublicDocument{
		Title:   title,
		Body:    body,
		Scripts: []string{},
		Styles:  []string{},
	})
}

func panelSlotFor(path string) (ohrats_rc_webui_types.ExtensionSlot, bool) {
	if strings.HasPrefix(path, "/devices/") {
		return ohrats_rc_webui_types.ExtensionSlot_DevicePanel, true
	}
	if path == "/account" || strings.HasPrefix(path, "/settings") {
		return ohrats_rc_webui_types.ExtensionSlot_SettingsPanel, true
	}
	return 0, false
}

func contributionsHTML(slot ohrats_rc_webui_types.ExtensionSlot) string {
	var builder strings.Builder
	for _, contribution := range Contributions(slot) {
		builder.WriteString(contribution.TrustedHtml)
	}
	return builder.String()
}

func scriptTags(paths []string) string {
	var builder strings.Builder
	for _, path := range paths {
		builder.WriteString(`<script type="module" src="`)
		builder.WriteString(escape(path))
		builder.WriteString(`"></script>`)
	}
	return builder.String()
}

func styleTags(paths []string) string {
	var builder strings.Builder
	for _, path := range paths {
		builder.WriteString(`<link rel="stylesheet" href="`)
		builder.WriteString(escape(path))
		builder.WriteString(`">`)
	}
	return builder.String()
}

var htmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#39;",
)

// escape mirrors the Rust shell escaping order for markup and attributes.
func escape(value string) string {
	return htmlEscaper.Replace(value)
}
