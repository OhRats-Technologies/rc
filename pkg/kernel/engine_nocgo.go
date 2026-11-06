//go:build !cgo

package kernel

import (
	"errors"
)

type Engine struct{}

func NewEngine() (*Engine, error) {
	return nil, errors.New("wasmtime component engine requires CGO")
}

func (e *Engine) Close() {}

func (e *Engine) Instantiate(comp *LoadedComponent) error {
	return errors.New("wasmtime component engine requires CGO")
}
