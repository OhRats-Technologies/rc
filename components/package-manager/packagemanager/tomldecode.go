package packagemanager

import (
	"fmt"
	"math"
)

func tomlRequiredField(table tomlTable, name string) (any, error) {
	value, ok := table[name]
	if !ok {
		return nil, fmt.Errorf("missing field `%s`", name)
	}
	return value, nil
}

func tomlTableOf(value any, field string) (tomlTable, error) {
	table, ok := value.(tomlTable)
	if !ok {
		return nil, tomlTypeError(value, field, "a map")
	}
	return table, nil
}

func tomlArrayOf(value any, field string) ([]any, error) {
	values, ok := value.([]any)
	if !ok {
		return nil, tomlTypeError(value, field, "a sequence")
	}
	return values, nil
}

func tomlString(value any, field string) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", tomlTypeError(value, field, "a string")
	}
	return text, nil
}

func tomlU32(value any, field string) (uint32, error) {
	switch number := value.(type) {
	case int64:
		if number >= 0 && number <= math.MaxUint32 {
			return uint32(number), nil
		}
	case uint64:
		if number <= math.MaxUint32 {
			return uint32(number), nil
		}
	}
	return 0, tomlTypeError(value, field, "u32")
}

func tomlTypeError(value any, field string, expected string) error {
	return fmt.Errorf("invalid type: %s, expected %s for `%s`", describeTOMLValue(value), expected, field)
}

func describeTOMLValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "unit value"
	case string:
		return "string " + quoted(typed)
	case bool:
		if typed {
			return "boolean `true`"
		}
		return "boolean `false`"
	case int64:
		return fmt.Sprintf("integer `%d`", typed)
	case uint64:
		return fmt.Sprintf("integer `%d`", typed)
	case float64:
		return fmt.Sprintf("floating point `%v`", typed)
	case []any:
		return "sequence"
	case tomlTable:
		return "map"
	default:
		return fmt.Sprintf("%T", value)
	}
}
