package webshell

import (
	"crypto/sha256"
	"fmt"

	"wit_component/assets"
)

// SharedVisualAssets is the immutable content-fingerprinted asset origin.
const sharedVisualAssets = "https://assets.ohrats.party/assets"

var (
	stylesPath        = assetPath(assets.StylesCSS, "css")
	publicStylesPath  = assetPath(assets.PublicStylesCSS, "css")
	copyScriptPath    = assetPath(assets.CopyScriptJS, "js")
	sidebarScriptPath = assetPath(assets.SidebarScriptJS, "js")
	socialCardPath    = assetPath(assets.SocialCardPNG, "png")
)

// assetPath mirrors the Rust build script: content digest prefix plus extension.
func assetPath(body []byte, extension string) string {
	digest := sha256.Sum256(body)
	return fmt.Sprintf("/assets/rc.%x.%s", digest[:8], extension)
}

func assetFor(path string) (string, []byte, bool) {
	switch path {
	case stylesPath:
		return "text/css; charset=utf-8", assets.StylesCSS, true
	case publicStylesPath:
		return "text/css; charset=utf-8", assets.PublicStylesCSS, true
	case copyScriptPath:
		return "text/javascript; charset=utf-8", assets.CopyScriptJS, true
	case sidebarScriptPath:
		return "text/javascript; charset=utf-8", assets.SidebarScriptJS, true
	case socialCardPath:
		return "image/png", assets.SocialCardPNG, true
	default:
		return "", nil, false
	}
}
