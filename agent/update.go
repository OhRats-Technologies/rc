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
	"time"

	"github.com/OhRats-Technologies/rc/agent/releaseverify"
)

func replaceExecutable(serverURL string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	base := strings.TrimRight(serverURL, "/") + "/downloads/"
	manifest, err := downloadSignedManifest(base)
	if err != nil {
		return err
	}
	comparison, err := releaseverify.CompareVersions(manifest.Version, version)
	if err != nil {
		return err
	}
	if comparison < 0 {
		return fmt.Errorf("refusing signed downgrade from %s to %s", version, manifest.Version)
	}
	artifact, ok := releaseverify.ArtifactFor(manifest, runtime.GOOS, runtime.GOARCH)
	if !ok {
		return fmt.Errorf("release does not contain %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	resp, err := http.Get(base + artifact.Name)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: %s", resp.Status)
	}
	temp, err := os.CreateTemp(filepath.Dir(executable), ".ohrats-rc-update-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(temp, hash), io.LimitReader(resp.Body, 100<<20))
	if err != nil {
		temp.Close()
		return err
	}
	if written == 100<<20 {
		temp.Close()
		return fmt.Errorf("downloaded file is too large")
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); actual != artifact.SHA256 {
		return fmt.Errorf("release hash mismatch")
	}
	if err = os.Chmod(name, 0755); err != nil {
		return err
	}
	output, err := exec.Command(name, "version").CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "OhRats RC Node "+manifest.Version {
		return fmt.Errorf("downloaded file does not match signed release version")
	}
	return os.Rename(name, executable)
}

func downloadSignedManifest(base string) (releaseverify.Manifest, error) {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		manifestBytes, err := downloadSmall(base+"release.json", 64<<10)
		if err == nil {
			var signatureBytes []byte
			signatureBytes, err = downloadSmall(base+"release.json.sig", 4<<10)
			if err == nil {
				var manifest releaseverify.Manifest
				manifest, err = releaseverify.Verify(manifestBytes, signatureBytes)
				if err == nil {
					return manifest, nil
				}
			}
		}
		lastErr = err
		if attempt < 3 {
			time.Sleep(250 * time.Millisecond)
		}
	}
	return releaseverify.Manifest{}, fmt.Errorf("release manifest: %w", lastErr)
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
