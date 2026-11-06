// Package githubsource implements the GitHub package source: spec parsing plus
// raw-file and release-asset resolution.
package githubsource

import (
	"errors"
	"fmt"
	"strings"
)

const (
	maxTokenBytes    = 100
	maxRevisionBytes = 128
	maxPathBytes     = 512
)

// Spec is a parsed GitHub package reference of the form
// OWNER/REPOSITORY[//PATH][#REVISION].
type Spec struct {
	Owner      string
	Repository string
	Path       string
	Revision   *string
}

// Parse validates a GitHub package reference.
func Parse(value string) (Spec, error) {
	body := value
	var revision *string
	if index := strings.IndexByte(value, '#'); index >= 0 {
		tail := value[index+1:]
		revision = &tail
		body = value[:index]
	}
	repository := body
	path := ""
	if index := strings.Index(body, "//"); index >= 0 {
		repository = body[:index]
		path = body[index+2:]
	}
	parts := strings.Split(repository, "/")
	owner := parts[0]
	name := ""
	if len(parts) > 1 {
		name = parts[1]
	}
	if len(parts) > 2 || !namespaceToken(owner) || !namespaceToken(name) {
		return Spec{}, fmt.Errorf("invalid GitHub repository %q", name)
	}
	if path != "" && !validPath(path) {
		return Spec{}, fmt.Errorf("invalid GitHub package path %q", path)
	}
	if revision != nil && !validRevision(*revision) {
		return Spec{}, errors.New("invalid GitHub revision")
	}
	return Spec{
		Owner:      owner,
		Repository: name,
		Path:       strings.Trim(path, "/"),
		Revision:   revision,
	}, nil
}

// PackageName derives the component name from the last path segment, falling
// back to the repository name.
func (value Spec) PackageName() (string, error) {
	name := value.Path
	if index := strings.LastIndexByte(name, '/'); index >= 0 {
		name = name[index+1:]
	}
	if name == "" {
		name = value.Repository
	}
	name = strings.TrimSuffix(name, ".wasm")
	if namespaceToken(name) {
		return name, nil
	}
	return "", fmt.Errorf("invalid GitHub package name %q", name)
}

// Display renders the artifact source for a concrete revision.
func (value Spec) Display(revision string) string {
	path := ""
	if value.Path != "" {
		path = "//" + value.Path
	}
	return fmt.Sprintf("github:%s/%s%s#%s", value.Owner, value.Repository, path, revision)
}

func namespaceToken(value string) bool {
	if value == "" || len(value) > maxTokenBytes {
		return false
	}
	for index := 0; index < len(value); index++ {
		if !safeSegmentByte(value[index]) {
			return false
		}
	}
	return true
}

func validRevision(value string) bool {
	if value == "" || len(value) > maxRevisionBytes {
		return false
	}
	for index := 0; index < len(value); index++ {
		byte := value[index]
		if !safeSegmentByte(byte) && byte != '/' {
			return false
		}
	}
	return true
}

func validPath(value string) bool {
	if len(value) > maxPathBytes {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return false
		}
		if part != "" && !namespaceToken(part) {
			return false
		}
	}
	return true
}

func safeSegmentByte(value byte) bool {
	alphanumeric := value >= '0' && value <= '9' ||
		value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z'
	return alphanumeric || value == '-' || value == '_' || value == '.'
}
