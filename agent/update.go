package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/OhRats-Technologies/rc/agent/releaseinfo"
)

var releaseBaseURL = "https://github.com/OhRats-Technologies/rc/releases/latest/download/"
var taggedReleaseBaseURL = "https://github.com/OhRats-Technologies/rc/releases/download/"

func replaceExecutable() (bool, error) {
	executable, err := os.Executable()
	if err != nil {
		return false, err
	}
	manifest, err := downloadReleaseManifest()
	if err != nil {
		return false, err
	}
	comparison, err := releaseinfo.CompareVersions(manifest.Version, version)
	if err != nil {
		return false, err
	}
	if comparison < 0 {
		return false, fmt.Errorf("refusing downgrade from %s to %s", version, manifest.Version)
	}
	if comparison == 0 {
		return false, nil
	}
	artifact, ok := releaseinfo.ArtifactFor(manifest, runtime.GOOS, runtime.GOARCH)
	if !ok {
		return false, fmt.Errorf("release does not contain %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	resp, err := http.Get(fmt.Sprintf("%sv%s/%s", taggedReleaseBaseURL, manifest.Version, artifact.Name))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("download: %s", resp.Status)
	}
	temp, err := os.CreateTemp(filepath.Dir(executable), ".rc-update-*")
	if err != nil {
		return false, err
	}
	name := temp.Name()
	defer os.Remove(name)
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(temp, hash), io.LimitReader(resp.Body, 100<<20))
	if err != nil {
		temp.Close()
		return false, err
	}
	if written == 100<<20 {
		temp.Close()
		return false, fmt.Errorf("downloaded file is too large")
	}
	if err = temp.Close(); err != nil {
		return false, err
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); actual != artifact.SHA256 {
		return false, fmt.Errorf("release hash mismatch")
	}
	if err = os.Chmod(name, 0755); err != nil {
		return false, err
	}
	output, err := exec.Command(name, "version").CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "RC "+manifest.Version {
		return false, fmt.Errorf("downloaded file does not match release version")
	}
	if err := os.Rename(name, executable); err != nil {
		return false, err
	}
	return true, nil
}

func downloadReleaseManifest() (releaseinfo.Manifest, error) {
	data, err := downloadSmall(releaseBaseURL+"release.json", 64<<10)
	if err != nil {
		return releaseinfo.Manifest{}, fmt.Errorf("release manifest: %w", err)
	}
	manifest, err := releaseinfo.Parse(data)
	if err != nil {
		return releaseinfo.Manifest{}, fmt.Errorf("release manifest: %w", err)
	}
	return manifest, nil
}

func downloadSmall(url string, limit int64) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", filepath.Base(url), resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download %s is too large", filepath.Base(url))
	}
	return data, nil
}

func syscallExecCurrent() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(executable, os.Args, os.Environ())
}
