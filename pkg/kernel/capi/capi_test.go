//go:build cgo && !windows

package capi

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bytecodealliance/wasmtime-go/v50"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repository root")
		}
		dir = parent
	}
}

func fixtureWasm(t *testing.T, name string) []byte {
	t.Helper()
	root := repoRoot(t)
	artifact := filepath.Join(root, "dist", "components", name+".wasm")
	if _, err := os.Stat(artifact); err != nil {
		command := exec.Command("sh", "scripts/build-component.sh", "components/"+name)
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("build fixture: %v\n%s", err, output)
		}
	}
	data, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestComponentHostDescriptorActivateInvoke(t *testing.T) {
	config := wasmtime.NewConfig()
	config.SetWasmComponentModel(true)
	engine := wasmtime.NewEngineWithConfig(config)
	defer engine.Close()

	host, err := New(engine, fixtureWasm(t, "fixture-collision"))
	if err != nil {
		t.Fatalf("instantiate fixture: %v", err)
	}
	defer host.Close()
	descriptor, err := host.Descriptor()
	if err != nil {
		t.Fatalf("descriptor: %v", err)
	}
	if descriptor.ID != "ohrats:fixture-collision" || descriptor.Version != "1.0.0" {
		t.Fatalf("unexpected descriptor: %+v", descriptor)
	}
	if len(descriptor.Commands) != 1 || descriptor.Commands[0].Name != "hello" {
		t.Fatalf("unexpected commands: %+v", descriptor.Commands)
	}
	if err := host.Activate(); err != nil {
		t.Fatalf("activate: %v", err)
	}
	code, message, err := host.Invoke("hello", []string{"world"})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if code != 0 || message != "" {
		t.Fatalf("invoke = %d, %q", code, message)
	}
}

func TestComponentHostPluginImports(t *testing.T) {
	config := wasmtime.NewConfig()
	config.SetWasmComponentModel(true)
	engine := wasmtime.NewEngineWithConfig(config)
	defer engine.Close()

	host, err := New(engine, fixtureWasm(t, "fixture-provider"))
	if err != nil {
		t.Fatalf("instantiate fixture-provider: %v", err)
	}
	defer host.Close()

	descriptor, err := host.Descriptor()
	if err != nil {
		t.Fatalf("descriptor: %v", err)
	}
	if descriptor.ID != "ohrats:fixture-provider" || descriptor.Version != "1.0.0" {
		t.Fatalf("unexpected descriptor: %+v", descriptor)
	}
	if len(descriptor.Provides) != 1 || descriptor.Provides[0].Name != "ohrats:rc-plugin/greeter" {
		t.Fatalf("unexpected provides: %+v", descriptor.Provides)
	}
	if err := host.Activate(); err != nil {
		t.Fatalf("activate: %v", err)
	}
	code, message, err := host.Invoke("provider-caller", nil)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if code != 0 || message != "" {
		t.Fatalf("invoke = %d, %q", code, message)
	}
}

func TestComponentHostStorage(t *testing.T) {
	config := wasmtime.NewConfig()
	config.SetWasmComponentModel(true)
	engine := wasmtime.NewEngineWithConfig(config)
	defer engine.Close()

	host, err := New(engine, fixtureWasm(t, "storage-fixture"))
	if err != nil {
		t.Fatalf("instantiate storage-fixture: %v", err)
	}
	defer host.Close()
	if err := host.Activate(); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if code, message, err := host.Invoke("kv-set", []string{"bucket", "key", "value"}); err != nil || code != 0 {
		t.Fatalf("kv-set = %d, %q, %v", code, message, err)
	}
	if code, message, err := host.Invoke("kv-get", []string{"bucket", "key"}); err != nil || code != 0 {
		t.Fatalf("kv-get = %d, %q, %v", code, message, err)
	}
	if code, message, err := host.Invoke("kv-list", []string{"bucket"}); err != nil || code != 0 {
		t.Fatalf("kv-list = %d, %q, %v", code, message, err)
	}
	if code, message, err := host.Invoke("kv-conflict", nil); err != nil || code != 0 {
		t.Fatalf("kv-conflict = %d, %q, %v", code, message, err)
	}
}

func TestComponentHostProcess(t *testing.T) {
	config := wasmtime.NewConfig()
	config.SetWasmComponentModel(true)
	engine := wasmtime.NewEngineWithConfig(config)
	defer engine.Close()

	host, err := New(engine, fixtureWasm(t, "fixture-collision"))
	if err != nil {
		t.Fatalf("instantiate fixture: %v", err)
	}
	defer host.Close()
	output, err := host.RunProcessProbe()
	if err != nil {
		t.Fatalf("process probe: %v", err)
	}
	if output != "hi" {
		t.Fatalf("process output = %q", output)
	}
}

func TestComponentHostExtras(t *testing.T) {
	config := wasmtime.NewConfig()
	config.SetWasmComponentModel(true)
	engine := wasmtime.NewEngineWithConfig(config)
	defer engine.Close()

	host, err := New(engine, fixtureWasm(t, "fixture-collision"))
	if err != nil {
		t.Fatalf("instantiate fixture: %v", err)
	}
	defer host.Close()
	result, err := host.RunExtrasProbe()
	if err != nil {
		t.Fatalf("extras probe: %v", err)
	}
	if result != "ok" {
		t.Fatalf("extras probe = %q", result)
	}
}

func TestComponentHostExecutionRuntime(t *testing.T) {
	config := wasmtime.NewConfig()
	config.SetWasmComponentModel(true)
	engine := wasmtime.NewEngineWithConfig(config)
	defer engine.Close()

	path := t.TempDir() + "/components.db"
	host, err := NewWithStorage(engine, fixtureWasm(t, "execution-runtime"), path)
	if err != nil {
		t.Fatalf("instantiate execution-runtime: %v", err)
	}
	defer host.Close()
	output, err := host.RunExecutionRuntimeProbe()
	if err != nil {
		t.Fatalf("execution probe: %v", err)
	}
	if output != "exec" {
		t.Fatalf("execution output = %q", output)
	}
}

func TestComponentHostKeys(t *testing.T) {
	config := wasmtime.NewConfig()
	config.SetWasmComponentModel(true)
	engine := wasmtime.NewEngineWithConfig(config)
	defer engine.Close()

	host, err := New(engine, fixtureWasm(t, "fixture-collision"))
	if err != nil {
		t.Fatalf("instantiate fixture: %v", err)
	}
	defer host.Close()
	if err := host.RunKeysProbe(); err != nil {
		t.Fatalf("keys probe: %v", err)
	}
}
