package integration

import (
	"strings"
	"testing"
)

func TestCLIDispatchesComponentCommand(t *testing.T) {
	root := repoRoot(t)
	binary := buildBinary(t, "./cmd/rc", "rc")
	components := componentArtifacts(t, "fixture-provider")

	stdout, stderr, err := runCommand(t, root, nil, binary, "--component-dir", components, "commands")
	if err != nil {
		t.Fatalf("commands: %v\n%s%s", err, stdout, stderr)
	}
	for _, want := range []string{"hello", "provider-caller", "fixture-provider"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("commands output missing %q:\n%s", want, stdout)
		}
	}

	stdout, stderr, err = runCommand(t, root, nil, binary, "--component-dir", components, "hello", "Bob")
	if err != nil {
		t.Fatalf("component command: %v\n%s%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "hello, Bob") {
		t.Fatalf("component output = %q", stdout)
	}

	stdout, stderr, err = runCommand(t, root, nil, binary, "--component-dir", components, "provider-caller")
	if err != nil {
		t.Fatalf("provider-caller: %v\n%s%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "none") {
		t.Fatalf("provider-caller output = %q", stdout)
	}
}
