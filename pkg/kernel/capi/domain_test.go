//go:build cgo && !windows

package capi

import (
	"testing"

	"github.com/bytecodealliance/wasmtime-go/v50"
)

func TestDomainHostMeshFixture(t *testing.T) {
	config := wasmtime.NewConfig()
	config.SetWasmComponentModel(true)
	engine := wasmtime.NewEngineWithConfig(config)
	defer engine.Close()
	host, err := New(engine, fixtureWasm(t, "mesh-policy-fixture"))
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	defer host.Close()
	code, message, err := host.Invoke("mesh-policy-verify", nil)
	if err != nil || code != 0 {
		t.Fatalf("mesh-policy-verify = %d, %q, %v", code, message, err)
	}
}
