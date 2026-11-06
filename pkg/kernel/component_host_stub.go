//go:build !cgo || windows

package kernel

import "errors"

// HostedComponent is unavailable without cgo on this platform.
type HostedComponent struct{}

// Host reports that the component host is unavailable.
func (e *Engine) Host(_ []byte) (*HostedComponent, error) {
	return nil, errors.New("component host requires cgo and a supported platform")
}

// Descriptor is unavailable.
func (h *HostedComponent) Descriptor() (HostDescriptor, error) {
	return HostDescriptor{}, errors.New("component host unavailable")
}

// Activate is unavailable.
func (h *HostedComponent) Activate() error { return errors.New("component host unavailable") }

// Invoke is unavailable.
func (h *HostedComponent) Invoke(string, []string) (uint32, string, error) {
	return 0, "", errors.New("component host unavailable")
}

// Close is a no-op.
func (h *HostedComponent) Close() {}
