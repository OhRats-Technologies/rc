package packagemanager

import (
	"errors"
	"fmt"

	"wit_component/ohrats_rc_artifact_cache_cache"
	"wit_component/ohrats_rc_artifact_cache_types"
	"wit_component/ohrats_rc_plugin_artifact_cache"
	"wit_component/ohrats_rc_plugin_package_source"
	"wit_component/ohrats_rc_plugin_service_registry"
)

const cacheService = "ohrats:rc-artifact-cache/cache"
const cacheVersion = "^0.1"
const maxArtifactBytes uint64 = 48 * 1024 * 1024

func rememberArtifact(artifact ohrats_rc_plugin_package_source.PackageArtifact) error {
	return writeArtifactCache(artifact.Digest, artifact.Bytes)
}

func writeArtifactCache(digest string, artifact []byte) error {
	result := ohrats_rc_plugin_artifact_cache.Write(digest, artifact)
	if result.IsErr() {
		return errors.New(result.Err())
	}
	return nil
}

func readExactArtifact(value lockedComponent) ([]byte, error) {
	artifact, found, err := fetchCachedArtifact(value.Digest)
	if err != nil {
		return nil, err
	}
	if found {
		if err := writeArtifactCache(value.Digest, artifact); err != nil {
			return nil, err
		}
		return artifact, nil
	}
	resolved, err := resolveExactSource(value.ResolvedSource)
	if err != nil {
		return nil, err
	}
	if resolved.Digest != value.Digest {
		return nil, fmt.Errorf(
			"locked digest %s is unavailable from %s; the source now resolves to %s",
			value.Digest,
			value.ResolvedSource,
			resolved.Digest,
		)
	}
	if err := rememberArtifact(resolved); err != nil {
		return nil, err
	}
	return resolved.Bytes, nil
}

func fetchCachedArtifact(digest string) ([]byte, bool, error) {
	result := ohrats_rc_plugin_service_registry.Providers(cacheService, cacheVersion)
	if result.IsErr() {
		return nil, false, errors.New(result.Err())
	}
	seen := map[string]struct{}{}
	for _, provider := range result.Ok() {
		for _, key := range provider.Keys {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			request := ohrats_rc_artifact_cache_types.FetchRequest{
				Digest:   digest,
				MaxBytes: maxArtifactBytes,
			}
			fetched := ohrats_rc_artifact_cache_cache.Fetch(key, request)
			if fetched.IsErr() {
				return nil, false, errors.New(fetched.Err())
			}
			option := fetched.Ok()
			if !option.IsSome() {
				continue
			}
			artifact := option.Some()
			if artifact.Digest != digest {
				return nil, false, fmt.Errorf(
					"cache provider %s returned the wrong digest",
					quoted(key),
				)
			}
			return artifact.Bytes, true, nil
		}
	}
	return nil, false, nil
}
