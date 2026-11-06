//go:build cgo

package kernel

import (
	"errors"
	"sync"

	"github.com/bytecodealliance/wasmtime-go/v50"
)

type Engine struct {
	mu     sync.Mutex
	engine *wasmtime.Engine
	linker *wasmtime.ComponentLinker
}

func NewEngine() (*Engine, error) {
	cfg := wasmtime.NewConfig()
	cfg.SetWasmComponentModel(true)

	engine := wasmtime.NewEngineWithConfig(cfg)
	if engine == nil {
		return nil, errors.New("failed to initialize wasmtime engine")
	}

	linker := wasmtime.NewComponentLinker(engine)
	if linker == nil {
		return nil, errors.New("failed to initialize component linker")
	}

	return &Engine{
		engine: engine,
		linker: linker,
	}, nil
}

func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.linker != nil {
		e.linker.Close()
		e.linker = nil
	}
	if e.engine != nil {
		e.engine.Close()
		e.engine = nil
	}
}

func (e *Engine) Instantiate(comp *LoadedComponent) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	wasmComp, err := wasmtime.NewComponent(e.engine, comp.WasmBytes)
	if err != nil {
		return err
	}
	defer wasmComp.Close()

	store := wasmtime.NewStore(e.engine)
	defer store.Close()

	if err := e.linker.DefineUnknownImportsAsTraps(wasmComp); err != nil {
		return err
	}

	inst, err := e.linker.Instantiate(store, wasmComp)
	if err != nil {
		return err
	}
	_ = inst
	return nil
}
