//go:build cgo && !windows

package capi

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bytecodealliance/wasmtime-go/v50"
)

func TestComponentHostHTTP(t *testing.T) {
	config := wasmtime.NewConfig()
	config.SetWasmComponentModel(true)
	engine := wasmtime.NewEngineWithConfig(config)
	defer engine.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("\x00asm\x01\x00\x00\x00"))
	}))
	defer server.Close()

	host, err := New(engine, fixtureWasm(t, "http-source"))
	if err != nil {
		t.Fatalf("instantiate http-source: %v", err)
	}
	defer host.Close()
	name, err := host.RunHTTPSourceProbe(server.URL + "/artifact.wasm")
	if err != nil {
		t.Fatalf("http probe: %v", err)
	}
	if name != "artifact" {
		t.Fatalf("artifact name = %q", name)
	}
}

func TestComponentHostNet(t *testing.T) {
	config := wasmtime.NewConfig()
	config.SetWasmComponentModel(true)
	engine := wasmtime.NewEngineWithConfig(config)
	defer engine.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				buffer := make([]byte, 16)
				n, _ := conn.Read(buffer)
				_, _ = conn.Write(buffer[:n])
			}()
		}
	}()

	host, err := NewWithNet(engine, fixtureWasm(t, "fixture-collision"), "", []string{listener.Addr().String()})
	if err != nil {
		t.Fatalf("instantiate fixture: %v", err)
	}
	defer host.Close()
	output, err := host.RunNetProbe(listener.Addr().String())
	if err != nil {
		t.Fatalf("net probe: %v", err)
	}
	if output != "ping" {
		t.Fatalf("net output = %q", output)
	}
}
