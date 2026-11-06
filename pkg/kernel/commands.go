package kernel

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ComponentCommand is one CLI command exported by an installed component.
type ComponentCommand struct {
	Name        string
	Summary     string
	Usage       string
	ComponentID string
	Path        string
}

// Commands loads every component in dir and returns its declared commands.
func (e *Engine) Commands(dir string) ([]ComponentCommand, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var commands []ComponentCommand
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".wasm") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		wasm, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		host, err := e.Host(wasm)
		if err != nil {
			continue
		}
		descriptor, err := host.Descriptor()
		host.Close()
		if err != nil {
			continue
		}
		for _, command := range descriptor.Commands {
			commands = append(commands, ComponentCommand{
				Name:        command.Name,
				Summary:     command.Summary,
				Usage:       command.Usage,
				ComponentID: descriptor.ID,
				Path:        path,
			})
		}
	}
	sort.Slice(commands, func(i, j int) bool { return commands[i].Name < commands[j].Name })
	return commands, nil
}

// InvokeCommand activates the component that provides command and invokes it.
// found is false when no installed component declares the command.
func (e *Engine) InvokeCommand(dir, command string, args []string) (found bool, code uint32, message string, err error) {
	commands, err := e.Commands(dir)
	if err != nil {
		return false, 0, "", err
	}
	for _, candidate := range commands {
		if candidate.Name != command {
			continue
		}
		wasm, err := os.ReadFile(candidate.Path)
		if err != nil {
			return true, 0, "", err
		}
		host, err := e.Host(wasm)
		if err != nil {
			return true, 0, "", err
		}
		defer host.Close()
		if err := host.Activate(); err != nil {
			return true, 0, "", fmt.Errorf("activate %s: %w", candidate.ComponentID, err)
		}
		code, message, err := host.Invoke(command, args)
		return true, code, message, err
	}
	return false, 0, "", nil
}
