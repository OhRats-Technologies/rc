package packagemanager

import "strings"

// tomlTable is a decoded TOML table. Values are string, int64, uint64, float64,
// bool, []any or tomlTable.
type tomlTable map[string]any

type tomlParser struct {
	text string
	pos  int
}

func parseTOML(text string) (tomlTable, error) {
	parser := &tomlParser{text: text}
	root := tomlTable{}
	current := root
	for {
		parser.skipWaste()
		if parser.eof() {
			break
		}
		if parser.peek() == '[' {
			table, err := parser.parseHeader(root)
			if err != nil {
				return nil, err
			}
			current = table
			continue
		}
		path, err := parser.parseKeyPath()
		if err != nil {
			return nil, err
		}
		parser.skipSpaces()
		if !parser.consume("=") {
			return nil, parser.errorf("expected `=`")
		}
		parser.skipSpaces()
		value, err := parser.parseValue()
		if err != nil {
			return nil, err
		}
		if err := assignTOMLPath(current, path, value); err != nil {
			return nil, parser.wrap(err)
		}
		if err := parser.finishStatement(); err != nil {
			return nil, err
		}
	}
	return root, nil
}

func (parser *tomlParser) parseHeader(root tomlTable) (tomlTable, error) {
	array := false
	parser.pos++
	if !parser.eof() && parser.peek() == '[' {
		array = true
		parser.pos++
	}
	parser.skipSpaces()
	path, err := parser.parseKeyPath()
	if err != nil {
		return nil, err
	}
	parser.skipSpaces()
	if array {
		if !parser.consume("]]") {
			return nil, parser.errorf("expected `]]`")
		}
	} else if !parser.consume("]") {
		return nil, parser.errorf("expected `]`")
	}
	table, err := resolveTOMLHeader(root, path, array)
	if err != nil {
		return nil, parser.wrap(err)
	}
	return table, parser.finishStatement()
}

func (parser *tomlParser) finishStatement() error {
	parser.skipSpaces()
	if parser.eof() {
		return nil
	}
	if parser.peek() == '#' {
		for !parser.eof() && parser.peek() != '\n' {
			parser.pos++
		}
	}
	if parser.eof() {
		return nil
	}
	if parser.peek() == '\r' {
		parser.pos++
	}
	if parser.eof() {
		return nil
	}
	if parser.peek() != '\n' {
		return parser.errorf("expected newline")
	}
	parser.pos++
	return nil
}

func (parser *tomlParser) parseKeyPath() ([]string, error) {
	var path []string
	for {
		parser.skipSpaces()
		segment, err := parser.parseKeySegment()
		if err != nil {
			return nil, err
		}
		path = append(path, segment)
		parser.skipSpaces()
		if !strings.HasPrefix(parser.text[parser.pos:], ".") {
			return path, nil
		}
		parser.pos++
	}
}

func (parser *tomlParser) parseKeySegment() (string, error) {
	if parser.eof() {
		return "", parser.errorf("expected a key")
	}
	switch parser.peek() {
	case '"', '\'':
		return parser.parseString()
	default:
		start := parser.pos
		for !parser.eof() {
			value := parser.peek()
			if isBareKeyByte(value) {
				parser.pos++
				continue
			}
			break
		}
		if start == parser.pos {
			return "", parser.errorf("expected a key")
		}
		return parser.text[start:parser.pos], nil
	}
}

func isBareKeyByte(value byte) bool {
	return isAsciiLetter(value) || (value >= '0' && value <= '9') || value == '_' || value == '-'
}

func (parser *tomlParser) parseValue() (any, error) {
	if parser.eof() {
		return nil, parser.errorf("expected a value")
	}
	switch parser.peek() {
	case '"', '\'':
		return parser.parseString()
	case '[':
		return parser.parseArray()
	case '{':
		return parser.parseInlineTable()
	}
	rest := parser.text[parser.pos:]
	switch {
	case strings.HasPrefix(rest, "true") && isValueEnd(rest, 4):
		parser.pos += 4
		return true, nil
	case strings.HasPrefix(rest, "false") && isValueEnd(rest, 5):
		parser.pos += 5
		return false, nil
	}
	return parser.parseNumber()
}

func isValueEnd(text string, index int) bool {
	if index >= len(text) {
		return true
	}
	return isValueDelimiter(text[index])
}

func isValueDelimiter(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n', ',', ']', '}', '#':
		return true
	default:
		return false
	}
}
