package webshell

import (
	"strings"

	"wit_component/assets"
)

type publicPage int

const (
	pageLanding publicPage = iota
	pageQuickstart
	pagePrinciples
	pageSecurity
	pageAuthentication
	pageCLI
	pageMCP
	pageAPI
)

type renderedPage struct {
	status       uint16
	cacheControl string
	body         string
}

// renderPage resolves a public or registered page for the request path.
func renderPage(path string, publicSignup bool, publicURL string) (renderedPage, bool) {
	page, ok := lookupPublicPage(path)
	if !ok {
		return registeredPage(path)
	}
	return renderedPage{
		status:       200,
		cacheControl: "public, max-age=0, must-revalidate",
		body:         renderTemplate(page, publicSignup, publicURL),
	}, true
}

func lookupPublicPage(path string) (publicPage, bool) {
	switch path {
	case "/":
		return pageLanding, true
	case "/docs":
		return pageQuickstart, true
	case "/docs/principles":
		return pagePrinciples, true
	case "/docs/security":
		return pageSecurity, true
	case "/docs/authentication":
		return pageAuthentication, true
	case "/docs/cli":
		return pageCLI, true
	case "/docs/mcp":
		return pageMCP, true
	case "/docs/api":
		return pageAPI, true
	default:
		return pageLanding, false
	}
}

func renderTemplate(page publicPage, publicSignup bool, publicURL string) string {
	template := snapshot(page, publicSignup)
	windowsInstall := strings.ReplaceAll(strings.TrimRight(publicURL, "/"), "'", "''")
	windowsInstall = "& ([scriptblock]::Create((Invoke-RestMethod -Uri '" + windowsInstall + "/install.ps1')))"
	rendered := strings.ReplaceAll(template, "__WINDOWS_INSTALL_COMMAND__", escape(windowsInstall))
	rendered = strings.ReplaceAll(rendered, "__PUBLIC_URL__", escape(strings.TrimRight(publicURL, "/")))
	rendered = strings.ReplaceAll(rendered, "__ASSET_VERSION__", componentVersion)
	rendered = strings.ReplaceAll(rendered, "/assets/styles.css", stylesPath)
	rendered = strings.ReplaceAll(rendered, "/assets/public.css", publicStylesPath)
	rendered = strings.ReplaceAll(rendered, "/assets/copy.js", copyScriptPath)
	rendered = strings.ReplaceAll(rendered, "/assets/social-card.png", socialCardPath)
	return currentize(page, rendered)
}

func snapshot(page publicPage, publicSignup bool) string {
	switch page {
	case pageLanding:
		if publicSignup {
			return assets.LandingOpen
		}
		return assets.LandingClosed
	case pageQuickstart:
		if publicSignup {
			return assets.QuickstartOpen
		}
		return assets.QuickstartClosed
	case pagePrinciples:
		if publicSignup {
			return assets.PrinciplesOpen
		}
		return assets.PrinciplesClosed
	case pageSecurity:
		if publicSignup {
			return assets.SecurityOpen
		}
		return assets.SecurityClosed
	case pageAuthentication:
		if publicSignup {
			return assets.AuthenticationOpen
		}
		return assets.AuthenticationClosed
	case pageCLI:
		if publicSignup {
			return assets.CLIOpen
		}
		return assets.CLIClosed
	case pageMCP:
		if publicSignup {
			return assets.MCPOpen
		}
		return assets.MCPClosed
	default:
		if publicSignup {
			return assets.APIOpen
		}
		return assets.APIClosed
	}
}

func currentize(page publicPage, html string) string {
	switch page {
	case pageCLI:
		return strings.ReplaceAll(html,
			"<tr><td><code>rc update</code></td><td>Check the latest GitHub Node release. If newer, verify/install it and restart the service if installed; otherwise leave the binary and service untouched.</td></tr>",
			"<tr><td><code>rc</code> / <code>rc --help</code></td><td>Show grouped native, kernel, and active component commands.</td></tr><tr><td><code>rc add SPEC</code> / <code>rc remove NAME</code></td><td>Install or remove a managed WebAssembly component.</td></tr><tr><td><code>rc install</code> / <code>rc list</code></td><td>Restore the locked component set or inspect installed components.</td></tr><tr><td><code>rc outdated [NAME...]</code></td><td>Show available managed-component updates.</td></tr><tr><td><code>rc update [NAME...]</code></td><td>Update managed WebAssembly components without replacing the native RC platform.</td></tr><tr><td><code>rc upgrade</code></td><td>Upgrade the native RC platform and refresh its core component bundle.</td></tr>")
	case pagePrinciples:
		html = strings.ReplaceAll(html,
			"Node releases are isolated from RC deploys",
			"Native releases are isolated from component updates")
		return strings.ReplaceAll(html,
			"GitHub Actions builds and publishes version-tagged Node/CLI releases independently of the RC control-plane deployment.</p><p>The installer and updater download GitHub Release assets directly, verify the selected artifact SHA-256 and reported version, and refuse downgrades.",
			"GitHub Actions publishes native RC and kernel artifacts independently from portable WebAssembly components.</p><p><code>rc update</code> changes managed components. <code>rc upgrade</code> verifies GitHub release digests, refreshes the native platform and core component bundle, and refuses native downgrades.")
	case pageSecurity:
		return strings.ReplaceAll(html,
			"GitHub Releases are the Node release trust boundary. The updater reads the published release manifest, verifies the selected artifact SHA-256, verifies the downloaded binary&#x27;s reported version, and refuses downgrades.</p><p>Node releases are published independently of RC runtime deployments, so a normal control-plane deploy does not rebuild or replace Node binaries.",
			"GitHub Releases are the native RC release trust boundary. <code>rc upgrade</code> verifies the selected RC, kernel, and core-component artifact digests, validates the downloaded executables and component graph, and refuses native downgrades.</p><p>Managed WebAssembly components update independently through <code>rc update</code>; a component-only update does not replace the native RC binary or kernel.")
	default:
		return html
	}
}

func registeredPage(path string) (renderedPage, bool) {
	for _, page := range Pages() {
		if page.Path == path {
			return renderedPage{
				status:       200,
				cacheControl: "no-store",
				body:         RenderRegistered(page.Title, page.Summary, page.Content),
			}, true
		}
	}
	return renderedPage{}, false
}
