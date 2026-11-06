// Package capi exposes the small part of Wasmtime's component C API that the
// Go kernel needs to define host functions and call component exports.
package capi

// Service is one provided service in a component descriptor.
type Service struct {
	Name     string
	Version  string
	Priority int32
	Keys     []string
}

// Requirement is one required service in a component descriptor.
type Requirement struct {
	Name      string
	Version   string
	Selection uint8
}

// Command is one component-provided CLI command.
type Command struct {
	Name    string
	Summary string
	Usage   string
}

// Descriptor is the plugin descriptor exported by a component.
type Descriptor struct {
	ID       string
	Version  string
	Provides []Service
	Requires []Requirement
	Commands []Command
}
