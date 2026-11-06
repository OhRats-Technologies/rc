// Package assets embeds the WebUI shell styles, scripts, image, and public page
// snapshots so the component can serve them without filesystem access.
package assets

import _ "embed"

//go:embed styles.css
var StylesCSS []byte

//go:embed public.css
var PublicStylesCSS []byte

//go:embed copy.js
var CopyScriptJS []byte

//go:embed sidebar.js
var SidebarScriptJS []byte

//go:embed social-card.png
var SocialCardPNG []byte

//go:embed public_snapshots/landing-open.html
var LandingOpen string

//go:embed public_snapshots/landing-closed.html
var LandingClosed string

//go:embed public_snapshots/quickstart-open.html
var QuickstartOpen string

//go:embed public_snapshots/quickstart-closed.html
var QuickstartClosed string

//go:embed public_snapshots/principles-open.html
var PrinciplesOpen string

//go:embed public_snapshots/principles-closed.html
var PrinciplesClosed string

//go:embed public_snapshots/security-open.html
var SecurityOpen string

//go:embed public_snapshots/security-closed.html
var SecurityClosed string

//go:embed public_snapshots/authentication-open.html
var AuthenticationOpen string

//go:embed public_snapshots/authentication-closed.html
var AuthenticationClosed string

//go:embed public_snapshots/cli-open.html
var CLIOpen string

//go:embed public_snapshots/cli-closed.html
var CLIClosed string

//go:embed public_snapshots/mcp-open.html
var MCPOpen string

//go:embed public_snapshots/mcp-closed.html
var MCPClosed string

//go:embed public_snapshots/api-open.html
var APIOpen string

//go:embed public_snapshots/api-closed.html
var APIClosed string
