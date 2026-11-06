package packagemanager

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"wit_component/ohrats_rc_plugin_component_store"
)

func installComponent(
	name string,
	artifact []byte,
	expectedDigest string,
) (ohrats_rc_plugin_component_store.InstalledComponent, error) {
	result := ohrats_rc_plugin_component_store.Install(name, artifact, someString(expectedDigest))
	if result.IsErr() {
		return ohrats_rc_plugin_component_store.InstalledComponent{}, errors.New(result.Err())
	}
	return result.Ok(), nil
}

func listComponents() ([]ohrats_rc_plugin_component_store.InstalledComponent, error) {
	result := ohrats_rc_plugin_component_store.Installed()
	if result.IsErr() {
		return nil, errors.New(result.Err())
	}
	return result.Ok(), nil
}

func removeComponent(name string) (bool, error) {
	result := ohrats_rc_plugin_component_store.Remove(name)
	if result.IsErr() {
		return false, errors.New(result.Err())
	}
	return result.Ok(), nil
}

func installedByName() (map[string]ohrats_rc_plugin_component_store.InstalledComponent, error) {
	values, err := listComponents()
	if err != nil {
		return nil, err
	}
	installed := make(map[string]ohrats_rc_plugin_component_store.InstalledComponent, len(values))
	for _, value := range values {
		installed[value.Name] = value
	}
	return installed, nil
}

func validateLock(desired desiredState, lock lockState) error {
	desiredNames := make(map[string]struct{}, len(desired.Components))
	for name := range desired.Components {
		desiredNames[name] = struct{}{}
	}
	lockedNames := make(map[string]struct{}, len(lock.Component))
	for _, value := range lock.Component {
		lockedNames[value.Name] = struct{}{}
	}
	if len(desiredNames) != len(lockedNames) {
		return errors.New("rc.lock does not match rc.toml; run rc update")
	}
	for name := range desiredNames {
		if _, ok := lockedNames[name]; !ok {
			return errors.New("rc.lock does not match rc.toml; run rc update")
		}
	}
	return nil
}

func validateLocked(
	expected lockedComponent,
	actual ohrats_rc_plugin_component_store.InstalledComponent,
) error {
	if expected.Id == actual.Id &&
		expected.Version == actual.Version &&
		expected.Digest == actual.Digest {
		return nil
	}
	return fmt.Errorf("locked component %s has unexpected metadata", expected.Name)
}

func validateCatalog(
	value resolvedPackage,
	installed ohrats_rc_plugin_component_store.InstalledComponent,
) error {
	if value.Catalog == nil {
		return nil
	}
	choice := *value.Catalog
	expectedId := choice.Namespace + ":" + choice.Package
	if installed.Id != expectedId || installed.Version != choice.Target.String() {
		return fmt.Errorf(
			"catalog selected %s %s, artifact contains %s %s",
			expectedId,
			choice.Target.String(),
			installed.Id,
			installed.Version,
		)
	}
	return nil
}

func managedName(value resolvedPackage) string {
	if value.Catalog != nil {
		return value.Catalog.Package
	}
	return value.Artifact.Name
}

func lockedFrom(
	value ohrats_rc_plugin_component_store.InstalledComponent,
	spec string,
	source string,
) lockedComponent {
	return lockedComponent{
		Name:           value.Name,
		Id:             value.Id,
		Version:        value.Version,
		Spec:           spec,
		ResolvedSource: source,
		Digest:         value.Digest,
	}
}

func resolvedVersions(value resolvedPackage) (string, string) {
	if value.Catalog != nil {
		return value.Catalog.Target.String(), value.Catalog.Latest.String()
	}
	digest := shortDigest(value.Artifact.Digest)
	return digest, digest
}

func shortDigest(value string) string {
	characters := []rune(value)
	if len(characters) > 19 {
		characters = characters[:19]
	}
	return string(characters)
}

func selectedNames(desired desiredState, names []string) ([]string, error) {
	if len(names) == 0 {
		result := make([]string, 0, len(desired.Components))
		for name := range desired.Components {
			result = append(result, name)
		}
		sort.Strings(result)
		return result, nil
	}
	for _, name := range names {
		if _, ok := desired.Components[name]; !ok {
			return nil, fmt.Errorf("unknown managed component %s", quoted(name))
		}
	}
	return names, nil
}

func updateArguments(args []string) ([]string, bool, error) {
	var names []string
	latest := false
	for _, value := range args {
		switch {
		case value == "--latest":
			latest = true
		case strings.HasPrefix(value, "-"):
			return nil, false, fmt.Errorf("unknown update option %s", quoted(value))
		default:
			names = append(names, value)
		}
	}
	return names, latest, nil
}

func oneArgument(args []string, usage string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	return "", fmt.Errorf("usage: %s", usage)
}

func noArguments(args []string, usage string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf("usage: %s", usage)
}
