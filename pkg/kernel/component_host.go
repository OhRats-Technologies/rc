//go:build cgo && !windows

package kernel

import (
	"path/filepath"

	"github.com/OhRats-Technologies/rc/pkg/kernel/capi"
	"github.com/OhRats-Technologies/rc/pkg/platform"
)

// HostedComponent is an instantiated component with the RC plugin host linked.
type HostedComponent struct {
	host *capi.Host
}

// Host instantiates a component with host imports and export invocation.
func (e *Engine) Host(wasm []byte) (*HostedComponent, error) {
	storagePath := ""
	if data, err := platform.DataDir(); err == nil {
		storagePath = filepath.Join(data, "components.db")
	}
	host, err := capi.NewWithStorage(e.engine, wasm, storagePath)
	if err != nil {
		return nil, err
	}
	return &HostedComponent{host: host}, nil
}

// Descriptor calls the plugin descriptor export.
func (h *HostedComponent) Descriptor() (HostDescriptor, error) {
	value, err := h.host.Descriptor()
	if err != nil {
		return HostDescriptor{}, err
	}
	descriptor := HostDescriptor{ID: value.ID, Version: value.Version}
	for _, service := range value.Provides {
		descriptor.Provides = append(descriptor.Provides, HostService{
			Name: service.Name, Version: service.Version, Priority: service.Priority, Keys: service.Keys,
		})
	}
	for _, requirement := range value.Requires {
		descriptor.Requires = append(descriptor.Requires, HostRequirement{
			Name: requirement.Name, Version: requirement.Version, Selection: requirement.Selection,
		})
	}
	for _, command := range value.Commands {
		descriptor.Commands = append(descriptor.Commands, HostCommand{
			Name: command.Name, Summary: command.Summary, Usage: command.Usage,
		})
	}
	return descriptor, nil
}

// Activate calls the plugin activate export.
func (h *HostedComponent) Activate() error { return h.host.Activate() }

// Invoke calls the plugin invoke export with a command and arguments.
func (h *HostedComponent) Invoke(command string, args []string) (uint32, string, error) {
	return h.host.Invoke(command, args)
}

// Close releases the component store and linker.
func (h *HostedComponent) Close() {
	if h != nil && h.host != nil {
		h.host.Close()
		h.host = nil
	}
}
