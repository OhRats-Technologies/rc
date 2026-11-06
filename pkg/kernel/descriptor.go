package kernel

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

type ComponentDescriptor struct {
	ID       string   `json:"id"`
	Version  string   `json:"version"`
	Provides []string `json:"provides,omitempty"`
	Requires []string `json:"requires,omitempty"`
}

func (d *ComponentDescriptor) Validate() error {
	if strings.TrimSpace(d.ID) == "" {
		return errors.New("component descriptor missing id")
	}
	return nil
}

type LoadedComponent struct {
	Descriptor ComponentDescriptor
	Path       string
	Digest     string
	WasmBytes  []byte
}

func LoadComponentBytes(path string, wasmBytes []byte) (*LoadedComponent, error) {
	if len(wasmBytes) < 8 || string(wasmBytes[:4]) != "\x00asm" {
		return nil, errors.New("invalid WebAssembly binary header")
	}

	hash := sha256.Sum256(wasmBytes)
	digest := hex.EncodeToString(hash[:])

	// Derive ID from path or descriptor
	stem := strings.TrimSuffix(filepathBase(path), ".wasm")
	desc := ComponentDescriptor{
		ID:      stem,
		Version: "0.1.0",
	}

	return &LoadedComponent{
		Descriptor: desc,
		Path:       path,
		Digest:     digest,
		WasmBytes:  wasmBytes,
	}, nil
}

func filepathBase(p string) string {
	idx := strings.LastIndexByte(p, '/')
	if idx >= 0 {
		return p[idx+1:]
	}
	idx = strings.LastIndexByte(p, '\\')
	if idx >= 0 {
		return p[idx+1:]
	}
	return p
}

func (l *LoadedComponent) Summary() string {
	return fmt.Sprintf("%s (digest=%s, bytes=%d)", l.Descriptor.ID, l.Digest[:8], len(l.WasmBytes))
}
