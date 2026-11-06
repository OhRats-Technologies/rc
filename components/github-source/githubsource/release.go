package githubsource

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"wit_component/ohrats_rc_plugin_http_client"
	"wit_component/ohrats_rc_plugin_package_source"
)

const (
	componentMediaType = "application/wasm, application/octet-stream"
	releaseMediaType   = "application/vnd.github+json"

	// maxComponentBytes bounds a downloaded component at 48 MiB.
	maxComponentBytes = 48 * 1024 * 1024
	// maxReleaseBytes bounds a GitHub release document at 2 MiB.
	maxReleaseBytes = 2 * 1024 * 1024
)

type release struct {
	TagName string
	Assets  []asset
}

type asset struct {
	Name               string
	BrowserDownloadURL string
	Digest             *string
}

// RawComponent fetches a single ".wasm" file from a repository at a revision,
// defaulting to "main".
func RawComponent(value Spec) (ohrats_rc_plugin_package_source.PackageArtifact, error) {
	revision := "main"
	if value.Revision != nil {
		revision = *value.Revision
	}
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s",
		value.Owner, value.Repository, revision, value.Path)
	response, err := get(url, componentMediaType, maxComponentBytes)
	if err != nil {
		return ohrats_rc_plugin_package_source.PackageArtifact{}, err
	}
	name, err := value.PackageName()
	if err != nil {
		return ohrats_rc_plugin_package_source.PackageArtifact{}, err
	}
	return artifact(name, value.Display(revision), response, nil)
}

// ReleaseComponent resolves the named component from the latest release, or
// from the release tagged by the spec revision.
func ReleaseComponent(value Spec) (ohrats_rc_plugin_package_source.PackageArtifact, error) {
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest",
		value.Owner, value.Repository)
	if value.Revision != nil {
		endpoint = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s",
			value.Owner, value.Repository, *value.Revision)
	}
	response, err := get(endpoint, releaseMediaType, maxReleaseBytes)
	if err != nil {
		return ohrats_rc_plugin_package_source.PackageArtifact{}, err
	}
	published, err := parseRelease(response.Body)
	if err != nil {
		return ohrats_rc_plugin_package_source.PackageArtifact{}, err
	}
	name, err := value.PackageName()
	if err != nil {
		return ohrats_rc_plugin_package_source.PackageArtifact{}, err
	}
	selected, found := selectAsset(published.Assets, name)
	if !found {
		return ohrats_rc_plugin_package_source.PackageArtifact{}, fmt.Errorf(
			"GitHub release %s has no %s component", published.TagName, name)
	}
	download, err := get(selected.BrowserDownloadURL, componentMediaType, maxComponentBytes)
	if err != nil {
		return ohrats_rc_plugin_package_source.PackageArtifact{}, err
	}
	return artifact(name, value.Display(published.TagName), download, selected.Digest)
}

func selectAsset(assets []asset, packageName string) (asset, bool) {
	candidates := [2]string{packageName + ".wasm", "rc-component-" + packageName + ".wasm"}
	for _, candidate := range assets {
		for _, name := range candidates {
			if candidate.Name == name {
				return candidate, true
			}
		}
	}
	return asset{}, false
}

func parseRelease(body []byte) (release, error) {
	var wire struct {
		TagName *string `json:"tag_name"`
		Assets  *[]struct {
			Name               *string `json:"name"`
			BrowserDownloadURL *string `json:"browser_download_url"`
			Digest             *string `json:"digest"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return release{}, fmt.Errorf("invalid GitHub release response: %v", err)
	}
	if wire.TagName == nil {
		return release{}, invalidRelease("tag_name")
	}
	if wire.Assets == nil {
		return release{}, invalidRelease("assets")
	}
	published := release{TagName: *wire.TagName, Assets: make([]asset, 0, len(*wire.Assets))}
	for _, entry := range *wire.Assets {
		if entry.Name == nil {
			return release{}, invalidRelease("name")
		}
		if entry.BrowserDownloadURL == nil {
			return release{}, invalidRelease("browser_download_url")
		}
		published.Assets = append(published.Assets, asset{
			Name:               *entry.Name,
			BrowserDownloadURL: *entry.BrowserDownloadURL,
			Digest:             entry.Digest,
		})
	}
	return published, nil
}

func invalidRelease(field string) error {
	return fmt.Errorf("invalid GitHub release response: missing field `%s`", field)
}

func get(url string, accept string, maximum uint64) (ohrats_rc_plugin_http_client.Response, error) {
	response := ohrats_rc_plugin_http_client.Send(ohrats_rc_plugin_http_client.Request{
		Method: "GET",
		Url:    url,
		Headers: []ohrats_rc_plugin_http_client.Header{{
			Name:  "accept",
			Value: accept,
		}},
		Body:             []uint8{},
		MaxResponseBytes: maximum,
	})
	if response.IsErr() {
		return ohrats_rc_plugin_http_client.Response{}, errors.New(response.Err())
	}
	value := response.Ok()
	if value.Status >= 200 && value.Status < 300 {
		return value, nil
	}
	return ohrats_rc_plugin_http_client.Response{}, fmt.Errorf(
		"GitHub returned status %d for %s", value.Status, url)
}

func artifact(
	name string,
	source string,
	response ohrats_rc_plugin_http_client.Response,
	expected *string,
) (ohrats_rc_plugin_package_source.PackageArtifact, error) {
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(response.Body))
	if expected != nil && strings.HasPrefix(*expected, "sha256:") &&
		!equalASCIICaseInsensitive(digest, *expected) {
		return ohrats_rc_plugin_package_source.PackageArtifact{},
			errors.New("GitHub release asset digest mismatch")
	}
	return ohrats_rc_plugin_package_source.PackageArtifact{
		Name:   name,
		Source: source,
		Digest: digest,
		Bytes:  response.Body,
	}, nil
}

func equalASCIICaseInsensitive(left string, right string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := 0; index < len(left); index++ {
		if lowerASCII(left[index]) != lowerASCII(right[index]) {
			return false
		}
	}
	return true
}

func lowerASCII(value byte) byte {
	if value >= 'A' && value <= 'Z' {
		return value + ('a' - 'A')
	}
	return value
}
