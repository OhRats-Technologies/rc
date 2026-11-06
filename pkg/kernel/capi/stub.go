//go:build !cgo || windows

package capi

import "errors"

// Host is unavailable without cgo on this platform.
type Host struct{}

// New reports that the component host is unavailable.
func New(_ any, _ []byte) (*Host, error) {
	return nil, errors.New("component host requires cgo and a supported platform")
}

// Close is a no-op.
func (h *Host) Close() {}

// Descriptor is unavailable.
func (h *Host) Descriptor() (Descriptor, error) {
	return Descriptor{}, errors.New("component host unavailable")
}

// Activate is unavailable.
func (h *Host) Activate() error { return errors.New("component host unavailable") }

// Invoke is unavailable.
func (h *Host) Invoke(string, []string) (uint32, string, error) {
	return 0, "", errors.New("component host unavailable")
}
