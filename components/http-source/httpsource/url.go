// Package httpsource holds the URL and artifact-name rules used by the HTTP
// package source.
package httpsource

import (
	"errors"
	"fmt"
	"strings"
)

// maxArtifactNameBytes bounds the derived artifact name.
const maxArtifactNameBytes = 96

// NormalizeURL accepts an absolute http(s) URL or completes a spec against the
// requested scheme. A bare spec gains the scheme prefix, but a spec that
// already names a different scheme is rejected.
func NormalizeURL(scheme, spec string) (string, error) {
	value := spec
	if !strings.HasPrefix(spec, "http://") && !strings.HasPrefix(spec, "https://") {
		value = scheme + ":" + spec
	}
	if strings.HasPrefix(value, scheme+"://") {
		return value, nil
	}
	return "", fmt.Errorf("source URL does not use %s", scheme)
}

// ArtifactName derives the package name from the final (post-redirect) URL,
// dropping the query/fragment and the ".wasm" suffix.
func ArtifactName(rawURL string) (string, error) {
	path := rawURL
	if index := strings.IndexAny(path, "?#"); index >= 0 {
		path = path[:index]
	}
	filename := path
	if index := strings.LastIndexByte(path, '/'); index >= 0 {
		filename = path[index+1:]
	}
	if filename == "" {
		return "", errors.New("source URL has no artifact filename")
	}
	name := strings.TrimSuffix(filename, ".wasm")
	if ValidName(name) {
		return name, nil
	}
	return "", fmt.Errorf("invalid artifact filename %q", filename)
}

// ValidName reports whether value is usable as a component name.
func ValidName(value string) bool {
	if value == "" || len(value) > maxArtifactNameBytes {
		return false
	}
	for index := 0; index < len(value); index++ {
		byte := value[index]
		alphanumeric := byte >= '0' && byte <= '9' ||
			byte >= 'a' && byte <= 'z' ||
			byte >= 'A' && byte <= 'Z'
		if !alphanumeric && byte != '-' && byte != '_' && byte != '.' {
			return false
		}
	}
	return true
}
