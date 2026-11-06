package packagemanager

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_plugin_state_store"
)

const desiredStateName = "rc.toml"
const lockStateName = "rc.lock"

type desiredComponent struct {
	Spec string
}

type desiredState struct {
	Schema     uint32
	Components map[string]desiredComponent
}

type lockedComponent struct {
	Name           string
	Id             string
	Version        string
	Spec           string
	ResolvedSource string
	Digest         string
}

type lockState struct {
	Schema    uint32
	Component []lockedComponent
}

func loadDesiredState() (desiredState, error) {
	state := desiredState{Components: map[string]desiredComponent{}}
	found, err := readState(desiredStateName)
	if err != nil {
		return state, err
	}
	if found != nil {
		text, err := utf8String(found)
		if err != nil {
			return state, err
		}
		state, err = decodeDesiredState(text)
		if err != nil {
			return state, fmt.Errorf("invalid %s: %s", desiredStateName, err)
		}
	}
	if err := validateStateSchema(state.Schema, desiredStateName); err != nil {
		return state, err
	}
	return state, nil
}

func loadLockState() (lockState, error) {
	state := lockState{}
	found, err := readState(lockStateName)
	if err != nil {
		return state, err
	}
	if found != nil {
		text, err := utf8String(found)
		if err != nil {
			return state, err
		}
		state, err = decodeLockState(text)
		if err != nil {
			return state, fmt.Errorf("invalid %s: %s", lockStateName, err)
		}
	}
	if err := validateStateSchema(state.Schema, lockStateName); err != nil {
		return state, err
	}
	return state, nil
}

func readState(name string) ([]byte, error) {
	result := ohrats_rc_plugin_state_store.Read(name)
	if result.IsErr() {
		return nil, errors.New(result.Err())
	}
	option := result.Ok()
	if !option.IsSome() {
		return nil, nil
	}
	return option.Some(), nil
}

func (state *desiredState) save() error {
	state.Schema = 1
	return writeState(desiredStateName, encodeDesiredState(*state))
}

func (state *lockState) save() error {
	state.Schema = 1
	sort.SliceStable(state.Component, func(left int, right int) bool {
		return state.Component[left].Name < state.Component[right].Name
	})
	return writeState(lockStateName, encodeLockState(*state))
}

func (state *lockState) replace(value lockedComponent) {
	retained := state.Component[:0]
	for _, item := range state.Component {
		if item.Name != value.Name {
			retained = append(retained, item)
		}
	}
	state.Component = append(retained, value)
}

func (state *lockState) remove(name string) {
	retained := state.Component[:0]
	for _, item := range state.Component {
		if item.Name != name {
			retained = append(retained, item)
		}
	}
	state.Component = retained
}

func (state *lockState) find(name string) *lockedComponent {
	for index := range state.Component {
		if state.Component[index].Name == name {
			return &state.Component[index]
		}
	}
	return nil
}

func writeState(name string, text string) error {
	result := ohrats_rc_plugin_state_store.Write(name, []byte(text))
	if result.IsErr() {
		return errors.New(result.Err())
	}
	return nil
}

func validateStateSchema(schema uint32, name string) error {
	if schema == 0 || schema == 1 {
		return nil
	}
	return fmt.Errorf("unsupported %s schema %d", name, schema)
}

func decodeDesiredState(text string) (desiredState, error) {
	state := desiredState{Components: map[string]desiredComponent{}}
	root, err := parseTOML(text)
	if err != nil {
		return state, err
	}
	schemaValue, err := tomlRequiredField(root, "schema")
	if err != nil {
		return state, err
	}
	if state.Schema, err = tomlU32(schemaValue, "schema"); err != nil {
		return state, err
	}
	componentsValue, err := tomlRequiredField(root, "components")
	if err != nil {
		return state, err
	}
	components, err := tomlTableOf(componentsValue, "components")
	if err != nil {
		return state, err
	}
	names := sortedKeys(components)
	for _, name := range names {
		entry, err := tomlTableOf(components[name], "components."+name)
		if err != nil {
			return state, err
		}
		specValue, err := tomlRequiredField(entry, "spec")
		if err != nil {
			return state, err
		}
		spec, err := tomlString(specValue, "spec")
		if err != nil {
			return state, err
		}
		state.Components[name] = desiredComponent{Spec: spec}
	}
	return state, nil
}

func decodeLockState(text string) (lockState, error) {
	state := lockState{}
	root, err := parseTOML(text)
	if err != nil {
		return state, err
	}
	schemaValue, err := tomlRequiredField(root, "schema")
	if err != nil {
		return state, err
	}
	if state.Schema, err = tomlU32(schemaValue, "schema"); err != nil {
		return state, err
	}
	entriesValue, err := tomlRequiredField(root, "component")
	if err != nil {
		return state, err
	}
	entries, err := tomlArrayOf(entriesValue, "component")
	if err != nil {
		return state, err
	}
	for _, raw := range entries {
		entry, err := tomlTableOf(raw, "component")
		if err != nil {
			return state, err
		}
		value := lockedComponent{}
		fields := []struct {
			name   string
			target *string
		}{
			{"name", &value.Name},
			{"id", &value.Id},
			{"version", &value.Version},
			{"spec", &value.Spec},
			{"resolved_source", &value.ResolvedSource},
			{"digest", &value.Digest},
		}
		for _, field := range fields {
			raw, err := tomlRequiredField(entry, field.name)
			if err != nil {
				return state, err
			}
			if *field.target, err = tomlString(raw, field.name); err != nil {
				return state, err
			}
		}
		state.Component = append(state.Component, value)
	}
	return state, nil
}

func sortedKeys(table tomlTable) []string {
	names := make([]string, 0, len(table))
	for name := range table {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func encodeDesiredState(state desiredState) string {
	header := "schema = " + formatUint(uint64(state.Schema))
	names := make([]string, 0, len(state.Components))
	for name := range state.Components {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return header + "\n\n[components]\n"
	}
	blocks := make([]string, 0, len(names))
	for _, name := range names {
		blocks = append(blocks, "[components."+encodeTOMLKey(name)+"]\nspec = "+
			encodeTOMLString(state.Components[name].Spec))
	}
	return header + "\n\n" + strings.Join(blocks, "\n\n") + "\n"
}

func encodeLockState(state lockState) string {
	header := "schema = " + formatUint(uint64(state.Schema))
	if len(state.Component) == 0 {
		return header + "\ncomponent = []\n"
	}
	blocks := make([]string, 0, len(state.Component))
	for _, value := range state.Component {
		block := strings.Join([]string{
			"[[component]]",
			"name = " + encodeTOMLString(value.Name),
			"id = " + encodeTOMLString(value.Id),
			"version = " + encodeTOMLString(value.Version),
			"spec = " + encodeTOMLString(value.Spec),
			"resolved_source = " + encodeTOMLString(value.ResolvedSource),
			"digest = " + encodeTOMLString(value.Digest),
		}, "\n")
		blocks = append(blocks, block)
	}
	return header + "\n\n" + strings.Join(blocks, "\n\n") + "\n"
}

func someString(value string) witTypes.Option[string] {
	return witTypes.Some(value)
}
