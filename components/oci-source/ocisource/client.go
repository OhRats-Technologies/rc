package ocisource

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"wit_component/ohrats_rc_plugin_http_client"
	"wit_component/ohrats_rc_plugin_package_source"
)

const manifestAccept = "application/vnd.oci.image.manifest.v1+json, " +
	"application/vnd.oci.artifact.manifest.v1+json, " +
	"application/vnd.docker.distribution.manifest.v2+json"

const (
	manifestLimit = 2 * 1024 * 1024
	layerLimit    = 48 * 1024 * 1024
	tokenLimit    = 1024 * 1024
)

// Pull resolves the pinned component layer of an OCI reference.
func Pull(value *Spec) (ohrats_rc_plugin_package_source.PackageArtifact, error) {
	var empty ohrats_rc_plugin_package_source.PackageArtifact
	response, token, err := authenticatedGet(value, value.ManifestURL(), manifestAccept, manifestLimit)
	if err != nil {
		return empty, err
	}
	manifestDigest := digest(response.Body)
	if declared := header(&response, "docker-content-digest"); declared != nil {
		manifestDigest = *declared
	}
	if err := ValidateDigest(manifestDigest); err != nil {
		return empty, err
	}
	if strings.HasPrefix(value.Reference, "sha256:") && !strings.EqualFold(value.Reference, manifestDigest) {
		return empty, errors.New("OCI manifest digest does not match requested digest")
	}
	layers, err := decodeLayers(response.Body)
	if err != nil {
		return empty, err
	}
	selected, err := selectLayer(layers)
	if err != nil {
		return empty, err
	}
	if err := ValidateDigest(selected.digest); err != nil {
		return empty, err
	}
	if selected.size > layerLimit {
		return empty, errors.New("OCI component layer exceeds 48 MiB")
	}
	blob, err := get(value.BlobURL(selected.digest), "application/wasm, application/octet-stream", layerLimit, token)
	if err != nil {
		return empty, err
	}
	if err := ensureSuccess(blob, "blob"); err != nil {
		return empty, err
	}
	actual := digest(blob.Body)
	if !strings.EqualFold(actual, selected.digest) {
		return empty, errors.New("OCI component layer digest mismatch")
	}
	name := value.PackageName()
	if title, found := selected.annotations["org.opencontainers.image.title"]; found {
		if trimmed, ok := strings.CutSuffix(title, ".wasm"); ok && validName(trimmed) {
			name = trimmed
		}
	}
	return ohrats_rc_plugin_package_source.PackageArtifact{
		Name:   name,
		Source: value.Source(manifestDigest),
		Digest: actual,
		Bytes:  blob.Body,
	}, nil
}

// authenticatedGet retries a request once with a bearer token when the registry
// answers 401, returning the successful response and the token used for blobs.
func authenticatedGet(
	value *Spec,
	url string,
	accept string,
	maximum uint64,
) (ohrats_rc_plugin_http_client.Response, *string, error) {
	var empty ohrats_rc_plugin_http_client.Response
	first, err := get(url, accept, maximum, nil)
	if err != nil {
		return empty, nil, err
	}
	if first.Status != 401 {
		if err := ensureSuccess(first, "manifest"); err != nil {
			return empty, nil, err
		}
		return *first, nil, nil
	}
	challenge := header(first, "www-authenticate")
	if challenge == nil {
		return empty, nil, errors.New("OCI registry returned 401 without an authentication challenge")
	}
	token, err := bearerToken(*challenge, value.Scope())
	if err != nil {
		return empty, nil, err
	}
	response, err := get(url, accept, maximum, token)
	if err != nil {
		return empty, nil, err
	}
	if err := ensureSuccess(response, "manifest"); err != nil {
		return empty, nil, err
	}
	return *response, token, nil
}

func get(url string, accept string, maximum uint64, token *string) (*ohrats_rc_plugin_http_client.Response, error) {
	headers := []ohrats_rc_plugin_http_client.Header{{Name: "accept", Value: accept}}
	if token != nil {
		headers = append(headers, ohrats_rc_plugin_http_client.Header{
			Name:  "authorization",
			Value: "Bearer " + *token,
		})
	}
	result := ohrats_rc_plugin_http_client.Send(ohrats_rc_plugin_http_client.Request{
		Method:           "GET",
		Url:              url,
		Headers:          headers,
		Body:             []uint8{},
		MaxResponseBytes: maximum,
	})
	if result.IsErr() {
		return nil, errors.New(result.Err())
	}
	response := result.Ok()
	return &response, nil
}

func ensureSuccess(response *ohrats_rc_plugin_http_client.Response, label string) error {
	if response.Status >= 200 && response.Status < 300 {
		return nil
	}
	return fmt.Errorf("OCI registry returned status %d for %s", response.Status, label)
}

func header(response *ohrats_rc_plugin_http_client.Response, name string) *string {
	for index := range response.Headers {
		candidate := &response.Headers[index]
		if strings.EqualFold(candidate.Name, name) {
			value := candidate.Value
			return &value
		}
	}
	return nil
}

func digest(bytes []byte) string {
	sum := sha256.Sum256(bytes)
	return "sha256:" + hex.EncodeToString(sum[:])
}
