// Package version exposes the single RC release version and semantic ordering.
package version

// Version is the RC release version. It is a var so tests and release builds
// can override it with -ldflags -X.
var Version = "0.20.4"
