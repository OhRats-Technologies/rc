package packagemanager

import (
	"fmt"
	"sort"
)

// Invoke dispatches one package-manager command.
func Invoke(command string, args []string) (uint32, error) {
	switch command {
	case "add":
		return addCommand(args)
	case "remove":
		return removeCommand(args)
	case "install":
		return installCommand(args)
	case "list":
		return listCommand(args)
	case "outdated":
		return outdatedCommand(args)
	case "update":
		return updateFlow(args)
	default:
		return 0, fmt.Errorf("unsupported command %s", quoted(command))
	}
}

func addCommand(args []string) (uint32, error) {
	spec, err := oneArgument(args, "rc add <spec>")
	if err != nil {
		return 0, err
	}
	resolved, err := resolveSource(spec, false)
	if err != nil {
		return 0, err
	}
	if err := rememberArtifact(resolved.Artifact); err != nil {
		return 0, err
	}
	name := managedName(resolved)
	installed, err := installComponent(name, resolved.Artifact.Bytes, resolved.Artifact.Digest)
	if err != nil {
		return 0, err
	}
	if err := validateCatalog(resolved, installed); err != nil {
		return 0, err
	}
	desired, err := loadDesiredState()
	if err != nil {
		return 0, err
	}
	desired.Components[name] = desiredComponent{Spec: spec}
	lock, err := loadLockState()
	if err != nil {
		return 0, err
	}
	lock.replace(lockedFrom(installed, spec, resolved.Artifact.Source))
	if err := desired.save(); err != nil {
		return 0, err
	}
	if err := lock.save(); err != nil {
		return 0, err
	}
	fmt.Printf("added %s %s\n", installed.Id, installed.Version)
	return 0, nil
}

func removeCommand(args []string) (uint32, error) {
	name, err := oneArgument(args, "rc remove <name>")
	if err != nil {
		return 0, err
	}
	desired, err := loadDesiredState()
	if err != nil {
		return 0, err
	}
	if _, ok := desired.Components[name]; !ok {
		return 0, fmt.Errorf("managed component %s is not in rc.toml", quoted(name))
	}
	removed, err := removeComponent(name)
	if err != nil {
		return 0, err
	}
	if !removed {
		return 0, fmt.Errorf("managed component %s is not installed", quoted(name))
	}
	lock, err := loadLockState()
	if err != nil {
		return 0, err
	}
	delete(desired.Components, name)
	lock.remove(name)
	if err := desired.save(); err != nil {
		return 0, err
	}
	if err := lock.save(); err != nil {
		return 0, err
	}
	fmt.Printf("removed %s\n", name)
	return 0, nil
}

func installCommand(args []string) (uint32, error) {
	if err := noArguments(args, "rc install"); err != nil {
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
	if err := validateLock(desired, lock); err != nil {
		return 0, err
	}
	installed, err := installedByName()
	if err != nil {
		return 0, err
	}
	for _, value := range lock.Component {
		if current, ok := installed[value.Name]; ok && current.Digest == value.Digest {
			continue
		}
		artifact, err := readExactArtifact(value)
		if err != nil {
			return 0, err
		}
		actual, err := installComponent(value.Name, artifact, value.Digest)
		if err != nil {
			return 0, err
		}
		if err := validateLocked(value, actual); err != nil {
			return 0, err
		}
	}
	desiredNames := map[string]struct{}{}
	for _, value := range lock.Component {
		desiredNames[value.Name] = struct{}{}
	}
	names := make([]string, 0, len(installed))
	for name := range installed {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := installed[name]
		if _, ok := desiredNames[name]; value.Managed && !ok {
			if _, err := removeComponent(value.Name); err != nil {
				return 0, err
			}
		}
	}
	return 0, nil
}

func listCommand(args []string) (uint32, error) {
	if err := noArguments(args, "rc list"); err != nil {
		return 0, err
	}
	values, err := listComponents()
	if err != nil {
		return 0, err
	}
	fmt.Println("NAME\tID\tVERSION\tMANAGED\tDIGEST")
	for _, value := range values {
		fmt.Printf(
			"%s\t%s\t%s\t%t\t%s\n",
			value.Name,
			value.Id,
			value.Version,
			value.Managed,
			value.Digest,
		)
	}
	return 0, nil
}

func outdatedCommand(args []string) (uint32, error) {
	desired, err := loadDesiredState()
	if err != nil {
		return 0, err
	}
	lock, err := loadLockState()
	if err != nil {
		return 0, err
	}
	fmt.Println("NAME\tCURRENT\tTARGET\tLATEST")
	names, err := selectedNames(desired, args)
	if err != nil {
		return 0, err
	}
	for _, name := range names {
		value := desired.Components[name]
		resolved, err := resolveSource(value.Spec, false)
		if err != nil {
			return 0, err
		}
		current := "-"
		if item := lock.find(name); item != nil {
			current = item.Version
		}
		target, latest := resolvedVersions(resolved)
		changed := true
		if item := lock.find(name); item != nil {
			changed = item.Digest != resolved.Artifact.Digest
		}
		if changed || target != latest {
			fmt.Printf("%s\t%s\t%s\t%s\n", name, current, target, latest)
		}
	}
	return 0, nil
}
