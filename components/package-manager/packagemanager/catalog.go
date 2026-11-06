package packagemanager

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"wit_component/ohrats_rc_plugin_catalog_store"
)

type catalogChoice struct {
	Namespace string
	Package   string
	Target    Version
	Latest    Version
	Source    string
}

func (choice catalogChoice) updatedSpec() string {
	rangeText := ""
	if choice.Latest.Major == 0 {
		rangeText = "^0." + formatUint(choice.Latest.Minor)
	} else {
		rangeText = "^" + formatUint(choice.Latest.Major)
	}
	return choice.Namespace + ":" + choice.Package + "@" + rangeText
}

type catalogPackage struct {
	Name    string
	Version string
	Source  string
}

type catalogDocument struct {
	Schema    uint32
	Namespace string
	Package   []catalogPackage
}

func resolveCatalog(namespace string, value string, useLatest bool) (catalogChoice, error) {
	if err := validateToken(namespace, "catalog namespace"); err != nil {
		return catalogChoice{}, err
	}
	packageName, requested, err := packageRequest(value)
	if err != nil {
		return catalogChoice{}, err
	}
	result := ohrats_rc_plugin_catalog_store.Read(namespace)
	if result.IsErr() {
		return catalogChoice{}, errors.New(result.Err())
	}
	option := result.Ok()
	if !option.IsSome() {
		return catalogChoice{}, fmt.Errorf("catalog %s is not installed", quoted(namespace))
	}
	text, err := utf8String(option.Some())
	if err != nil {
		return catalogChoice{}, err
	}
	catalog, err := decodeCatalog(text)
	if err != nil {
		return catalogChoice{}, fmt.Errorf("invalid %s catalog: %s", namespace, err)
	}
	if catalog.Schema != 1 || catalog.Namespace != namespace {
		return catalogChoice{}, fmt.Errorf("invalid catalog identity for %s", quoted(namespace))
	}
	type release struct {
		version Version
		source  string
	}
	var releases []release
	for _, candidate := range catalog.Package {
		if candidate.Name != packageName {
			continue
		}
		version, err := ParseVersion(candidate.Version)
		if err != nil {
			return catalogChoice{}, fmt.Errorf(
				"invalid %s:%s version: %s",
				namespace,
				packageName,
				err,
			)
		}
		releases = append(releases, release{version: version, source: candidate.Source})
	}
	sort.SliceStable(releases, func(left int, right int) bool {
		return releases[left].version.Compare(releases[right].version) < 0
	})
	if len(releases) == 0 {
		return catalogChoice{}, fmt.Errorf("package %s:%s is not in the catalog", namespace, packageName)
	}
	latest := releases[len(releases)-1].version
	selected := -1
	if useLatest {
		selected = len(releases) - 1
	} else {
		for index := len(releases) - 1; index >= 0; index-- {
			if requested.Matches(releases[index].version) {
				selected = index
				break
			}
		}
	}
	if selected < 0 {
		return catalogChoice{}, fmt.Errorf(
			"no %s:%s release matches %s",
			namespace,
			packageName,
			requested.String(),
		)
	}
	return catalogChoice{
		Namespace: namespace,
		Package:   packageName,
		Target:    releases[selected].version,
		Latest:    latest,
		Source:    releases[selected].source,
	}, nil
}

func packageRequest(value string) (string, VersionReq, error) {
	name := value
	requirement := "*"
	if index := strings.LastIndex(value, "@"); index >= 0 {
		name = value[:index]
		requirement = value[index+1:]
	}
	if err := validateToken(name, "package name"); err != nil {
		return "", VersionReq{}, err
	}
	parsed, err := ParseVersionReq(requirement)
	if err != nil {
		return "", VersionReq{}, fmt.Errorf("invalid package version requirement: %s", err)
	}
	return name, parsed, nil
}

func validateToken(value string, label string) error {
	valid := value != "" && len(value) <= 96
	if valid {
		for index := 0; index < len(value); index++ {
			character := value[index]
			if !isAsciiLetter(character) && (character < '0' || character > '9') &&
				character != '-' && character != '_' && character != '.' {
				valid = false
				break
			}
		}
	}
	if valid {
		return nil
	}
	return fmt.Errorf("invalid %s %s", label, quoted(value))
}

func decodeCatalog(text string) (catalogDocument, error) {
	catalog := catalogDocument{}
	root, err := parseTOML(text)
	if err != nil {
		return catalog, err
	}
	schemaValue, err := tomlRequiredField(root, "schema")
	if err != nil {
		return catalog, err
	}
	if catalog.Schema, err = tomlU32(schemaValue, "schema"); err != nil {
		return catalog, err
	}
	namespaceValue, err := tomlRequiredField(root, "namespace")
	if err != nil {
		return catalog, err
	}
	if catalog.Namespace, err = tomlString(namespaceValue, "namespace"); err != nil {
		return catalog, err
	}
	packagesValue, ok := root["package"]
	if !ok {
		return catalog, nil
	}
	entries, err := tomlArrayOf(packagesValue, "package")
	if err != nil {
		return catalog, err
	}
	for _, raw := range entries {
		entry, err := tomlTableOf(raw, "package")
		if err != nil {
			return catalog, err
		}
		value := catalogPackage{}
		fields := []struct {
			name   string
			target *string
		}{
			{"name", &value.Name},
			{"version", &value.Version},
			{"source", &value.Source},
		}
		for _, field := range fields {
			raw, err := tomlRequiredField(entry, field.name)
			if err != nil {
				return catalog, err
			}
			if *field.target, err = tomlString(raw, field.name); err != nil {
				return catalog, err
			}
		}
		catalog.Package = append(catalog.Package, value)
	}
	return catalog, nil
}

func utf8String(value []byte) (string, error) {
	for index := 0; index < len(value); {
		character, size := decodeRune(value[index:])
		if character == -1 {
			if size == 0 {
				return "", fmt.Errorf("incomplete utf-8 byte sequence from index %d", index)
			}
			return "", fmt.Errorf("invalid utf-8 sequence of %d bytes from index %d", size, index)
		}
		index += size
	}
	return string(value), nil
}
