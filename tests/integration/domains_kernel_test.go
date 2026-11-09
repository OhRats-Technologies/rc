package integration

import (
	"runtime"
	"strings"
	"testing"
)

func kernelDomainEnv(t *testing.T) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	return dir, []string{"RC_DATA_DIR=" + dir + "/data", "RC_STATE_DIR=" + dir + "/state"}
}

func runDomainCommand(t *testing.T, binary, components string, env []string, args ...string) (string, string, error) {
	t.Helper()
	full := append([]string{"--component-dir", components}, args...)
	return runCommand(t, repoRoot(t), env, binary, full...)
}

// TestKernelHostIdentityAndSessionDomains invokes the identity fixture through
// the real kernel host: users, passkeys, ceremonies, sessions, and admin proof.
func TestKernelHostIdentityAndSessionDomains(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native component host imports are unavailable on Windows")
	}
	binary := buildBinary(t, "./cmd/rc", "rc")
	components := componentArtifacts(t, "identity-fixture")
	_, env := kernelDomainEnv(t)

	stdout, stderr, err := runDomainCommand(t, binary, components, env, "identity-seed", "kernel-identity")
	if err != nil {
		t.Fatalf("identity-seed: %v\n%s%s", err, stdout, stderr)
	}
	token := strings.TrimSpace(stdout)
	if token == "" {
		t.Fatalf("identity-seed produced no session token: %q", stdout)
	}
	stdout, stderr, err = runDomainCommand(t, binary, components, env, "identity-verify", "kernel-identity", token)
	if err != nil || !strings.Contains(stdout, "identity state: ok") {
		t.Fatalf("identity-verify: %v\n%s%s", err, stdout, stderr)
	}
}

// TestKernelHostAPICredentialDomain invokes the API credential fixture through
// the kernel host, including step-up proof, replay, expiry, revocation, and CLI.
func TestKernelHostAPICredentialDomain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native component host imports are unavailable on Windows")
	}
	binary := buildBinary(t, "./cmd/rc", "rc")
	components := componentArtifacts(t, "api-credential-fixture")

	_, env := kernelDomainEnv(t)
	stdout, stderr, err := runDomainCommand(t, binary, components, env, "api-credentials-seed", "kernel-api")
	if err != nil {
		t.Fatalf("api-credentials-seed: %v\n%s%s", err, stdout, stderr)
	}
	stdout, stderr, err = runDomainCommand(t, binary, components, env, "api-credentials-verify", "kernel-api")
	if err != nil || !strings.Contains(stdout, "api credential state: ok") {
		t.Fatalf("api-credentials-verify: %v\n%s%s", err, stdout, stderr)
	}
}

func TestKernelHostAPICredentialRace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native component host imports are unavailable on Windows")
	}
	binary := buildBinary(t, "./cmd/rc", "rc")
	components := componentArtifacts(t, "api-credential-fixture")
	_, env := kernelDomainEnv(t)

	if stdout, stderr, err := runDomainCommand(t, binary, components, env, "api-credentials-seed", "kernel-race"); err != nil {
		t.Fatalf("api-credentials-seed: %v\n%s%s", err, stdout, stderr)
	}
	stdout, stderr, err := runDomainCommand(t, binary, components, env, "api-credentials-race", "kernel-race")
	if err != nil {
		t.Fatalf("api-credentials-race: %v\n%s%s", err, stdout, stderr)
	}
}

// TestKernelHostEventsDomain invokes events append/query/feed/retention through
// the kernel host with durable state shared across separate CLI processes.
func TestKernelHostEventsDomain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native component host imports are unavailable on Windows")
	}
	binary := buildBinary(t, "./cmd/rc", "rc")
	components := componentArtifacts(t, "events-fixture")
	_, env := kernelDomainEnv(t)

	stdout, stderr, err := runDomainCommand(t, binary, components, env, "events-seed", "kernel-events")
	if err != nil {
		t.Fatalf("events-seed: %v\n%s%s", err, stdout, stderr)
	}
	stdout, stderr, err = runDomainCommand(t, binary, components, env, "events-verify", "kernel-events")
	if err != nil || !strings.Contains(stdout, "events state: ok") {
		t.Fatalf("events-verify: %v\n%s%s", err, stdout, stderr)
	}
}

// TestKernelHostMeshAndKeyDomains invokes the deterministic mesh policy and
// protected key-custody fixtures through the kernel host.
func TestKernelHostMeshAndKeyDomains(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native component host imports are unavailable on Windows")
	}
	binary := buildBinary(t, "./cmd/rc", "rc")
	components := componentArtifacts(t, "mesh-policy-fixture", "key-custody-fixture")
	_, env := kernelDomainEnv(t)

	stdout, stderr, err := runDomainCommand(t, binary, components, env, "mesh-policy-verify")
	if err != nil || !strings.Contains(stdout, "mesh policy fixture: ok") {
		t.Fatalf("mesh-policy-verify: %v\n%s%s", err, stdout, stderr)
	}
	stdout, stderr, err = runDomainCommand(t, binary, components, env, "key-custody-public", "kernel-slot")
	if err != nil || strings.TrimSpace(stdout) == "" {
		t.Fatalf("key-custody-public: %v\n%s%s", err, stdout, stderr)
	}
	stdout, stderr, err = runDomainCommand(t, binary, components, env, "key-custody-verify", "kernel-slot")
	if err != nil || !strings.Contains(stdout, "key custody fixture: ok") {
		t.Fatalf("key-custody-verify: %v\n%s%s", err, stdout, stderr)
	}
}

// TestKernelHostPackageManagerCatalog invokes the package-manager component
// through the kernel host and verifies its managed command catalog.
func TestKernelHostPackageManagerCatalog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native component host imports are unavailable on Windows")
	}
	binary := buildBinary(t, "./cmd/rc", "rc")
	components := componentArtifacts(t, "package-manager")
	_, env := kernelDomainEnv(t)

	stdout, stderr, err := runDomainCommand(t, binary, components, env, "commands")
	if err != nil {
		t.Fatalf("package-manager commands: %v\n%s%s", err, stdout, stderr)
	}
	for _, command := range []string{"add", "install", "list", "outdated", "remove", "update"} {
		if !strings.Contains(stdout, command) || !strings.Contains(stdout, "ohrats:package-manager") {
			t.Fatalf("package-manager catalog missing %q:\n%s", command, stdout)
		}
	}
	stdout, stderr, err = runDomainCommand(t, binary, components, env, "list")
	if err != nil {
		t.Fatalf("package-manager list: %v\n%s%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "NAME") {
		t.Fatalf("package-manager list output = %q", stdout)
	}
}
