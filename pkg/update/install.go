package update

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/platform"
	"github.com/OhRats-Technologies/rc/pkg/version"
)

// Config controls one upgrade or component refresh.
type Config struct {
	ReleaseAPI     string
	CurrentVersion string
	ComponentDir   string
	BinaryDir      string
	Force          bool
	Reexec         bool
	Stdout         io.Writer
}

// Result reports what an upgrade did.
type Result struct {
	Updated    bool
	Version    string
	Components int
}

// Upgrade resolves, verifies, and installs the newest release.
func Upgrade(ctx context.Context, cfg Config) (*Result, error) {
	if cfg.CurrentVersion == "" {
		cfg.CurrentVersion = version.Version
	}
	if cfg.Stdout == nil {
		cfg.Stdout = io.Discard
	}
	api := strings.TrimSpace(cfg.ReleaseAPI)
	if api == "" {
		api = strings.TrimSpace(os.Getenv("RC_RELEASE_API"))
	}
	if api == "" {
		api = DefaultReleaseAPI
	}
	if cfg.ComponentDir == "" {
		dir, err := platform.ComponentDir()
		if err != nil {
			return nil, err
		}
		cfg.ComponentDir = dir
	}
	if cfg.BinaryDir == "" {
		dir, err := platform.BinaryDir()
		if err != nil {
			return nil, err
		}
		cfg.BinaryDir = dir
	}
	if err := recoverPending(cfg.BinaryDir); err != nil {
		return nil, err
	}

	release, err := FetchRelease(ctx, api)
	if err != nil {
		return nil, err
	}
	order, err := version.CompareStrings(release.Version, cfg.CurrentVersion)
	if err != nil {
		return nil, err
	}
	if order < 0 {
		return nil, fmt.Errorf("refusing downgrade from %s to %s", cfg.CurrentVersion, release.Version)
	}
	if order == 0 && !cfg.Force && runtimeComplete(cfg.BinaryDir, cfg.ComponentDir, cfg.CurrentVersion) {
		fmt.Fprintf(cfg.Stdout, "RC platform %s is already up to date\n", cfg.CurrentVersion)
		return &Result{Updated: false, Version: cfg.CurrentVersion}, nil
	}

	if asset, ok := release.Asset("rc-upgrade.json"); ok {
		data, err := Download(ctx, asset, MaxManifest)
		if err != nil {
			return nil, err
		}
		manifest, err := ParseManifest(data)
		if err != nil {
			return nil, err
		}
		if err := EnforceUpgradeManifest(manifest, cfg.CurrentVersion); err != nil {
			return nil, err
		}
	}

	profile, err := downloadProfile(ctx, release)
	if err != nil {
		return nil, err
	}

	stage, err := os.MkdirTemp("", "rc-profile-stage-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	if err := writeStagedComponents(stage, profile); err != nil {
		return nil, err
	}

	candidate := filepath.Join(cfg.BinaryDir, platform.ExecutableName("rc"))
	binaryData, binaryDigest, err := resolveCandidate(ctx, release, order, cfg)
	if err != nil {
		return nil, err
	}
	if binaryData != nil {
		staged, err := stageBinary(cfg.BinaryDir, binaryData)
		if err != nil {
			return nil, err
		}
		candidate = staged
	}
	if err := validateBinary(ctx, candidate, release.Version); err != nil {
		return nil, err
	}
	if err := validateProfile(ctx, candidate, stage); err != nil {
		return nil, err
	}

	installed, err := installComponents(cfg.ComponentDir, profile, cfg.Force)
	if err != nil {
		return nil, err
	}
	if binaryData != nil {
		if err := commitBinary(cfg.BinaryDir, candidate, binaryDigest, release.Version); err != nil {
			return nil, err
		}
	}
	fmt.Fprintf(cfg.Stdout, "RC platform upgraded to %s\n", release.Version)
	if cfg.Reexec {
		return &Result{Updated: true, Version: release.Version, Components: installed}, reexecCurrent()
	}
	return &Result{Updated: true, Version: release.Version, Components: installed}, nil
}

func resolveCandidate(ctx context.Context, release *Release, order int, cfg Config) ([]byte, string, error) {
	if order == 0 && !cfg.Force {
		return nil, "", nil
	}
	asset, ok := release.Asset(CLIAssetName())
	if !ok {
		if order == 0 {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("release %s has no %s asset", release.Version, CLIAssetName())
	}
	data, err := Download(ctx, asset, MaxBinary)
	if err != nil {
		return nil, "", err
	}
	executable, err := ExtractSingle(data, platform.ExecutableName("rc"), MaxBinary)
	if err != nil {
		return nil, "", err
	}
	return executable, Digest(executable), nil
}

// Recover completes or rolls back an interrupted binary replacement.
func Recover(binDir string) error {
	return recoverPending(binDir)
}
