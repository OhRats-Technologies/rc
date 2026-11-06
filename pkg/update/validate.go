package update

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/platform"
)

func validateBinary(ctx context.Context, path, expected string) error {
	output, err := runBounded(ctx, path, "version")
	if err != nil {
		return fmt.Errorf("staged RC binary failed its health check: %w", err)
	}
	if strings.TrimSpace(output) != "RC "+expected {
		return fmt.Errorf("staged RC binary reports %q, want RC %s", strings.TrimSpace(output), expected)
	}
	return nil
}

func validateProfile(ctx context.Context, binary, componentDir string) error {
	output, err := runBounded(ctx, binary, "policy-check", "--component-dir", componentDir)
	if err != nil {
		return fmt.Errorf("core profile failed validation: %w", err)
	}
	_ = output
	return nil
}

func runBounded(ctx context.Context, binary string, args ...string) (string, error) {
	timeout, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(timeout, binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{buffer: &stdout, limit: 64 << 10}
	cmd.Stderr = &limitedWriter{buffer: &stderr, limit: 64 << 10}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

type limitedWriter struct {
	buffer *bytes.Buffer
	limit  int
}

func (w *limitedWriter) Write(data []byte) (int, error) {
	remaining := w.limit - w.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			_, _ = w.buffer.Write(data[:remaining])
		} else {
			_, _ = w.buffer.Write(data)
		}
	}
	return len(data), nil
}

func runtimeComplete(binaryDir, componentDir, installedVersion string) bool {
	path := filepath.Join(binaryDir, platform.ExecutableName("rc"))
	if _, err := os.Stat(path); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output, err := runBounded(ctx, path, "version")
	if err != nil || strings.TrimSpace(output) != "RC "+installedVersion {
		return false
	}
	entries, err := os.ReadDir(componentDir)
	if err != nil {
		return false
	}
	seen := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".wasm") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".wasm")
		if _, err := os.Stat(filepath.Join(componentDir, name+".core")); err != nil {
			continue
		}
		seen++
	}
	if seen == 0 {
		return false
	}
	_, err = runBounded(ctx, path, "policy-check", "--component-dir", componentDir)
	return err == nil
}

func writeStagedComponents(dir string, profile *Profile) error {
	names := make([]string, 0, len(profile.Components))
	for name := range profile.Components {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name+".wasm"), profile.Components[name], 0o600); err != nil {
			return err
		}
	}
	return nil
}

func reexecCurrent() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return replaceProcess(executable)
}
