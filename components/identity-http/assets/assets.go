// Package assets embeds the identity browser script so the component can serve
// it without filesystem access.
package assets

import _ "embed"

//go:embed auth.js
var AuthJS []byte
