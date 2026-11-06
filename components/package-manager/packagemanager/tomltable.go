package packagemanager

import (
	"fmt"
	"strings"
)

func (parser *tomlParser) skipWaste() {
	for !parser.eof() {
		switch parser.peek() {
		case ' ', '\t', '\r', '\n':
			parser.pos++
		case '#':
			for !parser.eof() && parser.peek() != '\n' {
				parser.pos++
			}
		default:
			return
		}
	}
}

func (parser *tomlParser) skipSpaces() {
	for !parser.eof() && (parser.peek() == ' ' || parser.peek() == '\t') {
		parser.pos++
	}
}

func (parser *tomlParser) consume(value string) bool {
	if strings.HasPrefix(parser.text[parser.pos:], value) {
		parser.pos += len(value)
		return true
	}
	return false
}

func (parser *tomlParser) eof() bool {
	return parser.pos >= len(parser.text)
}

func (parser *tomlParser) peek() byte {
	return parser.text[parser.pos]
}

func (parser *tomlParser) errorf(format string, args ...any) error {
	line := 1
	column := 1
	for index := 0; index < parser.pos && index < len(parser.text); index++ {
		if parser.text[index] == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	return fmt.Errorf(
		"TOML parse error at line %d, column %d: %s",
		line,
		column,
		fmt.Sprintf(format, args...),
	)
}

func (parser *tomlParser) wrap(err error) error {
	return parser.errorf("%s", err.Error())
}

func assignTOMLPath(table tomlTable, path []string, value any) error {
	if len(path) == 0 {
		return fmt.Errorf("empty key")
	}
	current := table
	for _, segment := range path[:len(path)-1] {
		existing, ok := current[segment]
		if !ok {
			child := tomlTable{}
			current[segment] = child
			current = child
			continue
		}
		child, ok := existing.(tomlTable)
		if !ok {
			return fmt.Errorf("duplicate key `%s`", segment)
		}
		current = child
	}
	last := path[len(path)-1]
	if _, exists := current[last]; exists {
		return fmt.Errorf("duplicate key `%s`", last)
	}
	current[last] = value
	return nil
}

func resolveTOMLHeader(root tomlTable, path []string, array bool) (tomlTable, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("empty table name")
	}
	current := root
	for _, segment := range path[:len(path)-1] {
		existing, ok := current[segment]
		if !ok {
			child := tomlTable{}
			current[segment] = child
			current = child
			continue
		}
		child, ok := existing.(tomlTable)
		if !ok {
			return nil, fmt.Errorf("duplicate key `%s`", segment)
		}
		current = child
	}
	last := path[len(path)-1]
	if array {
		switch existing := current[last].(type) {
		case nil:
			table := tomlTable{}
			current[last] = []any{table}
			return table, nil
		case []any:
			table := tomlTable{}
			current[last] = append(existing, table)
			return table, nil
		default:
			return nil, fmt.Errorf("duplicate key `%s`", last)
		}
	}
	switch existing := current[last].(type) {
	case nil:
		table := tomlTable{}
		current[last] = table
		return table, nil
	case tomlTable:
		return existing, nil
	default:
		return nil, fmt.Errorf("duplicate key `%s`", last)
	}
}
