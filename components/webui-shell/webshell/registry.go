package webshell

import (
	"errors"
	"fmt"
	"sort"

	"wit_component/ohrats_rc_plugin_call_context"
	"wit_component/ohrats_rc_webui_types"
)

type owned[T any] struct {
	owner string
	value T
}

type orderedMap[T any] struct {
	keys   []string
	values map[string]owned[T]
}

func newOrderedMap[T any]() *orderedMap[T] {
	return &orderedMap[T]{values: map[string]owned[T]{}}
}

func (m *orderedMap[T]) get(id string) (owned[T], bool) {
	value, ok := m.values[id]
	return value, ok
}

func (m *orderedMap[T]) insert(id string, value owned[T]) {
	if _, ok := m.values[id]; !ok {
		index := sort.SearchStrings(m.keys, id)
		m.keys = append(m.keys, "")
		copy(m.keys[index+1:], m.keys[index:])
		m.keys[index] = id
	}
	m.values[id] = value
}

func (m *orderedMap[T]) remove(id string) bool {
	if _, ok := m.values[id]; !ok {
		return false
	}
	delete(m.values, id)
	index := sort.SearchStrings(m.keys, id)
	if index < len(m.keys) && m.keys[index] == id {
		m.keys = append(m.keys[:index], m.keys[index+1:]...)
	}
	return true
}

func (m *orderedMap[T]) list() []owned[T] {
	values := make([]owned[T], 0, len(m.keys))
	for _, key := range m.keys {
		values = append(values, m.values[key])
	}
	return values
}

func (m *orderedMap[T]) clear() {
	m.keys = nil
	m.values = map[string]owned[T]{}
}

var (
	registeredPages         = newOrderedMap[ohrats_rc_webui_types.Page]()
	registeredContributions = newOrderedMap[ohrats_rc_webui_types.Contribution]()
)

// RegisterPage validates and records a component-owned page contribution.
func RegisterPage(value ohrats_rc_webui_types.Page) error {
	if err := validatePage(value); err != nil {
		return err
	}
	owner, err := caller()
	if err != nil {
		return err
	}
	return insertOwned(registeredPages, owner, value.Id, value, "page")
}

// RemovePage removes a page previously registered by the calling component.
func RemovePage(id string) (bool, error) {
	if err := validateID(id); err != nil {
		return false, err
	}
	owner, err := caller()
	if err != nil {
		return false, err
	}
	return removeOwned(registeredPages, owner, id, "page")
}

// Pages lists active page contributions in registration-ID order.
func Pages() []ohrats_rc_webui_types.Page {
	entries := registeredPages.list()
	values := make([]ohrats_rc_webui_types.Page, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.value)
	}
	return values
}

// RegisterContribution validates and records a component-owned slot contribution.
func RegisterContribution(value ohrats_rc_webui_types.Contribution) error {
	if err := validateID(value.Id); err != nil {
		return err
	}
	if len(value.TrustedHtml) > 16*1024 {
		return errors.New("WebUI contribution is too large")
	}
	owner, err := caller()
	if err != nil {
		return err
	}
	return insertOwned(registeredContributions, owner, value.Id, value, "contribution")
}

// RemoveContribution removes a contribution registered by the calling component.
func RemoveContribution(id string) (bool, error) {
	if err := validateID(id); err != nil {
		return false, err
	}
	owner, err := caller()
	if err != nil {
		return false, err
	}
	return removeOwned(registeredContributions, owner, id, "contribution")
}

// Contributions lists contributions for one slot ordered by order then ID.
func Contributions(slot ohrats_rc_webui_types.ExtensionSlot) []ohrats_rc_webui_types.Contribution {
	values := make([]ohrats_rc_webui_types.Contribution, 0, len(registeredContributions.keys))
	for _, entry := range registeredContributions.list() {
		if entry.value.Slot == slot {
			values = append(values, entry.value)
		}
	}
	sort.SliceStable(values, func(left, right int) bool {
		if values[left].Order != values[right].Order {
			return values[left].Order < values[right].Order
		}
		return values[left].Id < values[right].Id
	})
	return values
}

// Deactivate drops every registration owned by mounted components.
func Deactivate() {
	registeredPages.clear()
	registeredContributions.clear()
}

func validatePage(value ohrats_rc_webui_types.Page) error {
	if err := validateID(value.Id); err != nil {
		return err
	}
	if len(value.Title) == 0 || len(value.Title) > 96 {
		return errors.New("WebUI page title must contain 1 to 96 bytes")
	}
	if len(value.Path) == 0 || value.Path[0] != '/' || len(value.Path) > 256 || containsParentTraversal(value.Path) {
		return errors.New("invalid WebUI page path")
	}
	if len(value.Summary) > 256 || len(value.Content) > 16*1024 {
		return errors.New("WebUI page contribution is too large")
	}
	return nil
}

func containsParentTraversal(path string) bool {
	for index := 0; index+1 < len(path); index++ {
		if path[index] == '.' && path[index+1] == '.' {
			return true
		}
	}
	return false
}

func validateID(value string) error {
	valid := len(value) > 0 && len(value) <= 96
	if valid {
		for index := 0; index < len(value); index++ {
			byte := value[index]
			if !isASCIIAlphanumeric(byte) && byte != '.' && byte != '-' && byte != '_' {
				valid = false
				break
			}
		}
	}
	if valid {
		return nil
	}
	return fmt.Errorf("invalid WebUI page id %q", value)
}

func isASCIIAlphanumeric(value byte) bool {
	return (value >= '0' && value <= '9') ||
		(value >= 'a' && value <= 'z') ||
		(value >= 'A' && value <= 'Z')
}

func caller() (string, error) {
	value := ohrats_rc_plugin_call_context.CallerComponentId()
	if value.IsNone() {
		return "", errors.New("WebUI registration requires a component caller")
	}
	return value.Some(), nil
}

func insertOwned[T any](values *orderedMap[T], owner string, id string, value T, kind string) error {
	if entry, ok := values.get(id); ok && entry.owner != owner {
		return fmt.Errorf("WebUI %s ID belongs to another component", kind)
	}
	values.insert(id, owned[T]{owner: owner, value: value})
	return nil
}

func removeOwned[T any](values *orderedMap[T], owner string, id string, kind string) (bool, error) {
	if entry, ok := values.get(id); ok && entry.owner != owner {
		return false, fmt.Errorf("WebUI %s belongs to another component", kind)
	}
	return values.remove(id), nil
}
