package integration

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageManagerComponentDomain(t *testing.T) {
	root := repoRoot(t)
	members := profileComponents(t, filepath.Join(root, "profiles", "core.toml"))
	found := false
	for _, member := range members {
		if member == "package-manager" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("core profile does not include package-manager")
	}
	components := componentArtifacts(t, "package-manager")
	binary := buildBinary(t, "./cmd/rc", "rc")
	stdout, stderr, err := runCommand(t, root, nil, binary, "--component-dir", components, "commands")
	if err != nil {
		t.Fatalf("package-manager commands: %v\n%s%s", err, stdout, stderr)
	}
	for _, command := range []string{"add", "install", "list", "outdated", "remove", "update"} {
		if !strings.Contains(stdout, command) || !strings.Contains(stdout, "ohrats:package-manager") {
			t.Fatalf("package-manager catalog missing %q:\n%s", command, stdout)
		}
	}
}
