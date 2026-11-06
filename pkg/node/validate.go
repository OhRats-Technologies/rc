//go:build !windows

package node

import "github.com/OhRats-Technologies/rc/pkg/platform"

func validatePrivateFile(path string) error {
	return platform.ValidatePrivatePath(path, false)
}
