package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/OhRats-Technologies/rc/pkg/update"
)

func tarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		header := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(files[name])), Typeflag: tar.TypeReg}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(files[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func buildVersionedBinary(t *testing.T, version string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "rc")
	stdout, stderr, err := runCommand(t, repoRoot(t), nil, "go", "build",
		"-ldflags", "-X github.com/OhRats-Technologies/rc/pkg/version.Version="+version,
		"-o", out, "./cmd/rc")
	if err != nil {
		t.Fatalf("build rc %s: %v\n%s%s", version, err, stdout, stderr)
	}
	return out
}

func profileBundle(t *testing.T, components map[string][]byte) []byte {
	t.Helper()
	names := make([]string, 0, len(components))
	for name := range components {
		names = append(names, name)
	}
	sort.Strings(names)
	files := make(map[string][]byte, len(components)+1)
	var lock strings.Builder
	lock.WriteString("schema 1\nprofile ohrats:core\n")
	for _, name := range names {
		data := components[name]
		files["components/"+name+".wasm"] = data
		fmt.Fprintf(&lock, "component %s %s\n", name, update.Digest(data))
	}
	files["profile.lock"] = []byte(lock.String())
	return tarGz(t, files)
}

type releaseServer struct {
	mu       sync.Mutex
	server   *httptest.Server
	assets   map[string][]byte
	override map[string]string
}

func newReleaseServer(t *testing.T, version string, assets map[string][]byte) *releaseServer {
	t.Helper()
	release := &releaseServer{assets: assets, override: make(map[string]string)}
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		release.mu.Lock()
		assets := make(map[string][]byte, len(release.assets))
		for name, data := range release.assets {
			assets[name] = data
		}
		overrides := make(map[string]string, len(release.override))
		for name, digest := range release.override {
			overrides[name] = digest
		}
		release.mu.Unlock()
		type asset struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
			Hash string `json:"digest"`
		}
		document := struct {
			TagName string  `json:"tag_name"`
			Assets  []asset `json:"assets"`
		}{TagName: "v" + version}
		names := make([]string, 0, len(assets))
		for name := range assets {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			data := assets[name]
			digest := update.Digest(data)
			if override, ok := overrides[name]; ok {
				digest = override
			}
			document.Assets = append(document.Assets, asset{
				Name: name,
				URL:  release.server.URL + "/assets/" + name,
				Size: int64(len(data)),
				Hash: digest,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(document)
	})
	mux.HandleFunc("/assets/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/assets/")
		release.mu.Lock()
		data, ok := release.assets[name]
		release.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	})
	release.server = httptest.NewUnstartedServer(mux)
	release.server.Start()
	t.Cleanup(release.server.Close)
	return release
}

func upgradeFixture(t *testing.T) (releaseAssets map[string][]byte, binary string, components map[string][]byte) {
	t.Helper()
	binary = buildVersionedBinary(t, "0.99.0")
	componentDir := componentArtifacts(t, "crypto-control")
	data, err := os.ReadFile(filepath.Join(componentDir, "crypto-control.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	components = map[string][]byte{"crypto-control": data}
	releaseAssets = map[string][]byte{
		"rc-" + update.PlatformName() + ".tar.gz": tarGz(t, map[string][]byte{"rc": mustRead(t, binary)}),
		"rc-core-profile.tar.gz":                  profileBundle(t, components),
		"rc-upgrade.json":                         []byte(`{"schema":1,"minimumUpgrader":"0.20.2"}`),
	}
	return releaseAssets, binary, components
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func upgradeEnv(t *testing.T, binDir, componentDir string) []string {
	return []string{
		"RC_INSTALL_BIN_DIR=" + binDir,
		"RC_COMPONENT_DIR=" + componentDir,
		"RC_STATE_DIR=" + t.TempDir(),
		"HOME=" + t.TempDir(),
	}
}

func TestCLIUpgradeInstallsPinnedRelease(t *testing.T) {
	root := repoRoot(t)
	assets, _, _ := upgradeFixture(t)
	server := newReleaseServer(t, "0.99.0", assets)
	binary := buildBinary(t, "./cmd/rc", "rc")
	binDir, componentDir := t.TempDir(), t.TempDir()

	stdout, stderr, err := runCommand(t, root, upgradeEnv(t, binDir, componentDir), binary,
		"upgrade", "--release-api", server.server.URL+"/latest")
	if err != nil {
		t.Fatalf("upgrade: %v\n%s%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "upgraded to 0.99.0") {
		t.Fatalf("unexpected output: %s", stdout)
	}
	installed := filepath.Join(binDir, "rc")
	versionOut, versionErr, err := runCommand(t, root, nil, installed, "version")
	if err != nil || strings.TrimSpace(versionOut) != "RC 0.99.0" {
		t.Fatalf("installed binary: %q %v %s", versionOut, err, versionErr)
	}
	for _, name := range []string{"crypto-control.wasm", "crypto-control.core", "profile.lock"} {
		if _, err := os.Stat(filepath.Join(componentDir, name)); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
	}

	// The upgraded binary sees the same release and reports a no-op.
	stdout, stderr, err = runCommand(t, root, upgradeEnv(t, binDir, componentDir), installed,
		"upgrade", "--release-api", server.server.URL+"/latest")
	if err != nil {
		t.Fatalf("second upgrade: %v\n%s%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "already up to date") {
		t.Fatalf("expected no-op, got: %s", stdout)
	}
}

func TestCLIUpgradeRejectsDowngrade(t *testing.T) {
	root := repoRoot(t)
	assets, _, _ := upgradeFixture(t)
	server := newReleaseServer(t, "0.0.1", assets)
	binary := buildBinary(t, "./cmd/rc", "rc")
	stdout, stderr, err := runCommand(t, root, upgradeEnv(t, t.TempDir(), t.TempDir()), binary,
		"upgrade", "--release-api", server.server.URL+"/latest")
	if err == nil || !strings.Contains(stderr, "refusing downgrade") {
		t.Fatalf("expected downgrade rejection, got %v\n%s%s", err, stdout, stderr)
	}
}

func TestCLIUpgradeEnforcesMinimumUpgrader(t *testing.T) {
	root := repoRoot(t)
	assets, _, _ := upgradeFixture(t)
	assets["rc-upgrade.json"] = []byte(`{"schema":1,"minimumUpgrader":"99.0.0"}`)
	server := newReleaseServer(t, "0.99.0", assets)
	binary := buildBinary(t, "./cmd/rc", "rc")
	_, stderr, err := runCommand(t, root, upgradeEnv(t, t.TempDir(), t.TempDir()), binary,
		"upgrade", "--release-api", server.server.URL+"/latest")
	if err == nil || !strings.Contains(stderr, "requires upgrader") {
		t.Fatalf("expected minimum upgrader rejection, got %v: %s", err, stderr)
	}
}

func TestCLIUpgradeRejectsDigestMismatch(t *testing.T) {
	root := repoRoot(t)
	assets, _, _ := upgradeFixture(t)
	assets["rc-upgrade.json"] = []byte(`{"schema":1,"minimumUpgrader":"0.20.2"}`)
	server := newReleaseServer(t, "0.99.0", assets)
	server.mu.Lock()
	server.override["rc-core-profile.tar.gz"] = "sha256:" + strings.Repeat("0", 64)
	server.mu.Unlock()
	binary := buildBinary(t, "./cmd/rc", "rc")
	_, stderr, err := runCommand(t, root, upgradeEnv(t, t.TempDir(), t.TempDir()), binary,
		"upgrade", "--release-api", server.server.URL+"/latest")
	if err == nil || !strings.Contains(stderr, "digest") {
		t.Fatalf("expected digest rejection, got %v: %s", err, stderr)
	}
}

func TestUpgradePreservesLocalComponentOverride(t *testing.T) {
	root := repoRoot(t)
	assets, _, components := upgradeFixture(t)
	server := newReleaseServer(t, "0.99.0", assets)
	binary := buildBinary(t, "./cmd/rc", "rc")
	binDir, componentDir := t.TempDir(), t.TempDir()
	local := []byte("local override")
	if err := os.WriteFile(filepath.Join(componentDir, "crypto-control.wasm"), local, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := runCommand(t, root, upgradeEnv(t, binDir, componentDir), binary,
		"upgrade", "--release-api", server.server.URL+"/latest")
	if err != nil {
		t.Fatalf("upgrade: %v\n%s%s", err, stdout, stderr)
	}
	got, err := os.ReadFile(filepath.Join(componentDir, "crypto-control.wasm"))
	if err != nil || !bytes.Equal(got, local) {
		t.Fatalf("local override replaced: %q, %v", got, err)
	}
	if len(components) != 1 {
		t.Fatal("fixture changed")
	}
}
