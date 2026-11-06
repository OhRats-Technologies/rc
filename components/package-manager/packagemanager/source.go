package packagemanager

import (
	"errors"
	"fmt"
	"strings"

	"wit_component/ohrats_rc_plugin_package_source"
	"wit_component/ohrats_rc_plugin_service_registry"
)

const sourceService = "ohrats:rc-plugin/package-source"
const sourceVersion = "^0.1"

type resolvedPackage struct {
	Artifact ohrats_rc_plugin_package_source.PackageArtifact
	Catalog  *catalogChoice
}

type sourceSpec struct {
	direct      bool
	scheme      string
	spec        string
	namespace   string
	packageName string
}

func resolveSource(value string, latest bool) (resolvedPackage, error) {
	schemes, err := sourceSchemes()
	if err != nil {
		return resolvedPackage{}, err
	}
	classified, err := classifySource(value, schemes)
	if err != nil {
		return resolvedPackage{}, err
	}
	if classified.direct {
		artifact, err := resolvePackageArtifact(classified.scheme, classified.spec)
		if err != nil {
			return resolvedPackage{}, err
		}
		return resolvedPackage{Artifact: artifact}, nil
	}
	choice, err := resolveCatalog(classified.namespace, classified.packageName, latest)
	if err != nil {
		return resolvedPackage{}, err
	}
	scheme, spec, err := directSource(choice.Source, schemes)
	if err != nil {
		return resolvedPackage{}, err
	}
	artifact, err := resolvePackageArtifact(scheme, spec)
	if err != nil {
		return resolvedPackage{}, err
	}
	return resolvedPackage{Artifact: artifact, Catalog: &choice}, nil
}

func resolveExactSource(value string) (ohrats_rc_plugin_package_source.PackageArtifact, error) {
	schemes, err := sourceSchemes()
	if err != nil {
		return ohrats_rc_plugin_package_source.PackageArtifact{}, err
	}
	scheme, spec, err := directSource(value, schemes)
	if err != nil {
		return ohrats_rc_plugin_package_source.PackageArtifact{}, err
	}
	return resolvePackageArtifact(scheme, spec)
}

func resolvePackageArtifact(scheme string, spec string) (ohrats_rc_plugin_package_source.PackageArtifact, error) {
	result := ohrats_rc_plugin_package_source.Resolve(scheme, spec)
	if result.IsErr() {
		return ohrats_rc_plugin_package_source.PackageArtifact{}, errors.New(result.Err())
	}
	return result.Ok(), nil
}

func sourceSchemes() (map[string]struct{}, error) {
	result := ohrats_rc_plugin_service_registry.Providers(sourceService, sourceVersion)
	if result.IsErr() {
		return nil, errors.New(result.Err())
	}
	schemes := map[string]struct{}{}
	for _, provider := range result.Ok() {
		for _, key := range provider.Keys {
			schemes[key] = struct{}{}
		}
	}
	return schemes, nil
}

func classifySource(value string, schemes map[string]struct{}) (sourceSpec, error) {
	if isSourcePath(value) {
		return sourceSpec{direct: true, scheme: "file", spec: value}, nil
	}
	if index := strings.Index(value, "://"); index >= 0 {
		scheme := value[:index]
		if err := ensureSourceScheme(scheme, schemes); err != nil {
			return sourceSpec{}, err
		}
		return sourceSpec{direct: true, scheme: scheme, spec: value}, nil
	}
	if index := strings.Index(value, ":"); index >= 0 {
		prefix := value[:index]
		rest := value[index+1:]
		if _, ok := schemes[prefix]; ok {
			if rest == "" {
				return sourceSpec{}, fmt.Errorf("invalid package source %s", quoted(value))
			}
			return sourceSpec{direct: true, scheme: prefix, spec: rest}, nil
		}
		if !catalogPackageName(rest) {
			if err := ensureSourceScheme(prefix, schemes); err != nil {
				return sourceSpec{}, err
			}
		}
		return sourceSpec{namespace: prefix, packageName: rest}, nil
	}
	return sourceSpec{namespace: "ohrats", packageName: value}, nil
}

func directSource(value string, schemes map[string]struct{}) (string, string, error) {
	classified, err := classifySource(value, schemes)
	if err != nil {
		return "", "", err
	}
	if classified.direct {
		return classified.scheme, classified.spec, nil
	}
	return "", "", fmt.Errorf(
		"catalog entries must resolve to a direct source; %s is not a source provider",
		quoted(classified.namespace),
	)
}

func ensureSourceScheme(scheme string, schemes map[string]struct{}) error {
	if _, ok := schemes[scheme]; ok {
		return nil
	}
	return fmt.Errorf("package source provider %s is not installed", quoted(scheme))
}

func isSourcePath(value string) bool {
	return strings.HasPrefix(value, "/") ||
		strings.HasPrefix(value, "./") ||
		strings.HasPrefix(value, "../")
}

func catalogPackageName(value string) bool {
	name := value
	if index := strings.LastIndex(value, "@"); index >= 0 {
		name = value[:index]
	}
	if name == "" || len(name) > 96 {
		return false
	}
	for index := 0; index < len(name); index++ {
		character := name[index]
		if !isAsciiLetter(character) && (character < '0' || character > '9') &&
			character != '-' && character != '_' && character != '.' {
			return false
		}
	}
	return true
}
