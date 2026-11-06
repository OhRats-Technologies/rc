package packagemanager

import (
	"errors"
	"fmt"

	"wit_component/ohrats_rc_plugin_component_store"
)

type pendingUpdate struct {
	name     string
	spec     string
	resolved resolvedPackage
}

func updateFlow(args []string) (uint32, error) {
	names, useLatest, err := updateArguments(args)
	if err != nil {
		return 0, err
	}
	desired, err := loadDesiredState()
	if err != nil {
		return 0, err
	}
	lock, err := loadLockState()
	if err != nil {
		return 0, err
	}
	selected, err := selectedNames(desired, names)
	if err != nil {
		return 0, err
	}
	var pending []pendingUpdate
	for _, name := range selected {
		original := desired.Components[name].Spec
		resolved, err := resolveSource(original, useLatest)
		if err != nil {
			return 0, err
		}
		if err := rememberArtifact(resolved.Artifact); err != nil {
			return 0, err
		}
		spec := original
		if useLatest && resolved.Catalog != nil {
			spec = resolved.Catalog.updatedSpec()
		}
		pending = append(pending, pendingUpdate{name: name, spec: spec, resolved: resolved})
	}

	candidates := make([]ohrats_rc_plugin_component_store.UpdateCandidate, 0, len(pending))
	for _, value := range pending {
		candidates = append(candidates, ohrats_rc_plugin_component_store.UpdateCandidate{
			Name:           value.name,
			Artifact:       value.resolved.Artifact.Bytes,
			ExpectedDigest: someString(value.resolved.Artifact.Digest),
		})
	}
	prepared, err := prepareComponents(candidates)
	if err != nil {
		return 0, err
	}
	for _, value := range pending {
		var preparedValue *ohrats_rc_plugin_component_store.InstalledComponent
		for _, candidate := range prepared.Components {
			if candidate.Name == value.name {
				preparedValue = &ohrats_rc_plugin_component_store.InstalledComponent{
					Name:    candidate.Name,
					Id:      candidate.Id,
					Version: candidate.Version,
					Digest:  candidate.Digest,
					Managed: true,
				}
				break
			}
		}
		if preparedValue == nil {
			abortComponents(prepared)
			return 0, fmt.Errorf("prepared update omitted component %s", quoted(value.name))
		}
		if err := validateCatalog(value.resolved, *preparedValue); err != nil {
			abortComponents(prepared)
			return 0, err
		}
	}
	installed, err := commitComponents(prepared)
	if err != nil {
		return 0, err
	}
	for _, value := range pending {
		var actual *ohrats_rc_plugin_component_store.InstalledComponent
		for index := range installed {
			if installed[index].Name == value.name {
				actual = &installed[index]
				break
			}
		}
		if actual == nil {
			return 0, fmt.Errorf("committed update omitted component %s", quoted(value.name))
		}
		changed := true
		if item := lock.find(value.name); item != nil {
			changed = item.Digest != actual.Digest
		}
		desired.Components[value.name] = desiredComponent{Spec: value.spec}
		lock.replace(lockedFrom(*actual, value.spec, value.resolved.Artifact.Source))
		if changed {
			fmt.Printf("updated %s %s\n", actual.Id, actual.Version)
		}
	}
	if err := desired.save(); err != nil {
		return 0, err
	}
	if err := lock.save(); err != nil {
		return 0, err
	}
	return 0, nil
}

func prepareComponents(
	candidates []ohrats_rc_plugin_component_store.UpdateCandidate,
) (ohrats_rc_plugin_component_store.PreparedSet, error) {
	result := ohrats_rc_plugin_component_store.Prepare(candidates)
	if result.IsErr() {
		return ohrats_rc_plugin_component_store.PreparedSet{}, errors.New(result.Err())
	}
	return result.Ok(), nil
}

func commitComponents(
	prepared ohrats_rc_plugin_component_store.PreparedSet,
) ([]ohrats_rc_plugin_component_store.InstalledComponent, error) {
	result := ohrats_rc_plugin_component_store.Commit(prepared)
	if result.IsErr() {
		return nil, errors.New(result.Err())
	}
	return result.Ok(), nil
}

func abortComponents(prepared ohrats_rc_plugin_component_store.PreparedSet) {
	ohrats_rc_plugin_component_store.Abort(prepared)
}
