package update

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/platform"
)

func installComponents(dir string, profile *Profile, force bool) (int, error) {
	if err := ensurePrivateDir(dir); err != nil {
		return 0, err
	}
	names := sortedNames(profile.Components)
	installed := 0
	for _, name := range names {
		target := filepath.Join(dir, name+".wasm")
		marker := filepath.Join(dir, name+".core")
		if !force {
			if _, err := os.Stat(target); err == nil {
				if _, markerErr := os.Stat(marker); markerErr != nil {
					// A component without a .core marker is a local override.
					continue
				}
			}
		}
		digest := profile.Digests[name]
		if digest == "" {
			digest = Digest(profile.Components[name])
		}
		if err := writeFileAtomic(target, profile.Components[name], 0o600); err != nil {
			return installed, err
		}
		if err := writeFileAtomic(marker, []byte(digest+"\n"), 0o600); err != nil {
			return installed, err
		}
		installed++
	}
	if force {
		if err := removeStaleCoreComponents(dir, profile.Components); err != nil {
			return installed, err
		}
	}
	if profile.Lock != "" {
		if err := writeFileAtomic(filepath.Join(dir, "profile.lock"), []byte(profile.Lock), 0o600); err != nil {
			return installed, err
		}
	}
	return installed, nil
}

func removeStaleCoreComponents(dir string, keep map[string][]byte) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".wasm") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".wasm")
		if _, ok := keep[name]; ok {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, name+".core")); err != nil {
			continue // local override
		}
		_ = os.Remove(filepath.Join(dir, entry.Name()))
		_ = os.Remove(filepath.Join(dir, name+".core"))
	}
	return nil
}

func sortedNames(values map[string][]byte) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// RuntimeComplete reports whether the installed binary and core components
// validate. It is used by doctor and upgrade no-op detection.
func RuntimeComplete() bool {
	binDir, err := platform.BinaryDir()
	if err != nil {
		return false
	}
	componentDir, err := platform.ComponentDir()
	if err != nil {
		return false
	}
	return runtimeComplete(binDir, componentDir, currentBinaryVersion(binDir))
}

func currentBinaryVersion(binDir string) string {
	output, err := runBounded(context.Background(), filepath.Join(binDir, platform.ExecutableName("rc")), "version")
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(output), "RC ")
}
