package update

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/platform"
)

// RefreshComponents installs only the core component bundle.
func RefreshComponents(ctx context.Context, cfg Config) (*Result, error) {
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
	release, err := FetchRelease(ctx, api)
	if err != nil {
		return nil, err
	}
	profile, err := downloadProfile(ctx, release)
	if err != nil {
		return nil, err
	}
	installed, err := installComponents(cfg.ComponentDir, profile, cfg.Force)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(cfg.Stdout, "RC components updated to %s (%d components)\n", release.Version, installed)
	return &Result{Updated: true, Version: release.Version, Components: installed}, nil
}

func downloadProfile(ctx context.Context, release *Release) (*Profile, error) {
	if asset, ok := release.Asset("rc-core-profile.tar.gz"); ok {
		data, err := Download(ctx, asset, MaxBundle)
		if err != nil {
			return nil, err
		}
		return ExtractProfile(data, true)
	}
	asset, ok := release.Asset("rc-core-components.tar.gz")
	if !ok {
		return nil, fmt.Errorf("release %s has no core component bundle", release.Version)
	}
	data, err := Download(ctx, asset, MaxBundle)
	if err != nil {
		return nil, err
	}
	return ExtractProfile(data, false)
}
