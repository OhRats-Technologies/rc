package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/version"
)

const (
	// DefaultReleaseAPI is the public RC release endpoint.
	DefaultReleaseAPI = "https://api.github.com/repos/OhRats-Technologies/rc/releases/latest"

	// MaxBinary bounds one native executable.
	MaxBinary = 160 << 20
	// MaxComponent bounds one Wasm component.
	MaxComponent = 48 << 20
	// MaxBundle bounds one component archive.
	MaxBundle = 256 << 20
	// MaxManifest bounds the upgrade metadata document.
	MaxManifest = 1 << 20
	// MaxReleaseJSON bounds the release API response.
	MaxReleaseJSON = 4 << 20
)

// Asset is one immutable release asset.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	// Digest is the GitHub-provided "sha256:<64 hex>" value. It is required.
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// Release is the resolved metadata for one release.
type Release struct {
	Version string
	Assets  map[string]Asset
}

// Manifest is the rc-upgrade.json compatibility record.
type Manifest struct {
	Schema          int    `json:"schema"`
	MinimumUpgrader string `json:"minimumUpgrader"`
}

// PlatformName returns the release asset platform token.
func PlatformName() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

// CLIAssetName returns the native archive name for this platform.
func CLIAssetName() string {
	return "rc-" + PlatformName() + ".tar.gz"
}

// FetchRelease resolves the release API document.
func FetchRelease(ctx context.Context, api string) (*Release, error) {
	if strings.TrimSpace(api) == "" {
		return nil, errors.New("release API URL is required")
	}
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "rc-upgrade")
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release metadata: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("release metadata: HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var document struct {
		TagName string  `json:"tag_name"`
		Assets  []Asset `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, MaxReleaseJSON)).Decode(&document); err != nil {
		return nil, fmt.Errorf("decode release metadata: %w", err)
	}
	releaseVersion := strings.TrimPrefix(strings.TrimSpace(document.TagName), "v")
	if releaseVersion == "" {
		return nil, version.ErrEmptyVersion
	}
	if _, err := version.ParseSemver(releaseVersion); err != nil {
		return nil, fmt.Errorf("release tag: %w", err)
	}
	assets := make(map[string]Asset, len(document.Assets))
	for _, asset := range document.Assets {
		assets[asset.Name] = asset
	}
	return &Release{Version: releaseVersion, Assets: assets}, nil
}

// Asset returns a named asset.
func (r *Release) Asset(name string) (Asset, bool) {
	asset, ok := r.Assets[name]
	return asset, ok
}

// Download fetches an asset and verifies its pinned digest and size bound.
func Download(ctx context.Context, asset Asset, limit int64) ([]byte, error) {
	if asset.URL == "" {
		return nil, fmt.Errorf("asset %s has no download URL", asset.Name)
	}
	if _, err := ParseDigest(asset.Digest); err != nil {
		return nil, fmt.Errorf("asset %s: %w", asset.Name, err)
	}
	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "rc-upgrade")
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", asset.Name, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", asset.Name, res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", asset.Name, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("asset %s exceeds %d bytes", asset.Name, limit)
	}
	if asset.Size > 0 && int64(len(data)) != asset.Size {
		return nil, fmt.Errorf("asset %s size mismatch: got %d, want %d", asset.Name, len(data), asset.Size)
	}
	if err := VerifyDigest(data, asset.Digest); err != nil {
		return nil, fmt.Errorf("asset %s: %w", asset.Name, err)
	}
	return data, nil
}

// ParseDigest validates sha256:<64 lowercase hex>.
func ParseDigest(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") {
		return "", errors.New("digest must be sha256:<64 lowercase hexadecimal digits>")
	}
	hexPart := value[len("sha256:"):]
	if _, err := hex.DecodeString(hexPart); err != nil || strings.ToLower(hexPart) != hexPart {
		return "", errors.New("digest must be sha256:<64 lowercase hexadecimal digits>")
	}
	return value, nil
}

// Digest computes the sha256 digest string for bytes.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// VerifyDigest checks bytes against a pinned digest.
func VerifyDigest(data []byte, expected string) error {
	want, err := ParseDigest(expected)
	if err != nil {
		return err
	}
	if got := Digest(data); got != want {
		return errors.New("digest does not match the pinned value")
	}
	return nil
}

// ParseManifest validates rc-upgrade.json.
func ParseManifest(data []byte) (*Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse upgrade manifest: %w", err)
	}
	if manifest.Schema != 1 {
		return nil, fmt.Errorf("unsupported upgrade manifest schema %d", manifest.Schema)
	}
	if _, err := version.ParseSemver(manifest.MinimumUpgrader); err != nil {
		return nil, fmt.Errorf("upgrade manifest minimumUpgrader: %w", err)
	}
	return &manifest, nil
}

// EnforceUpgradeManifest refuses an upgrade the current binary cannot perform.
func EnforceUpgradeManifest(manifest *Manifest, current string) error {
	order, err := version.CompareStrings(current, manifest.MinimumUpgrader)
	if err != nil {
		return err
	}
	if order < 0 {
		return fmt.Errorf("this release requires upgrader %s or newer; install the release once with public/install.sh or public/install.ps1", manifest.MinimumUpgrader)
	}
	return nil
}
