//go:build !windows

package update

import "github.com/OhRats-Technologies/rc/pkg/platform"

func protectPrivateDir(path string) error {
	return platform.ProtectPrivatePath(path, true)
}
