package packagemanager

import (
	"math"
	"strconv"
	"strings"
)

func (parser *tomlParser) parseArray() (any, error) {
	parser.pos++
	values := []any{}
	for {
		parser.skipWaste()
		if parser.eof() {
			return nil, parser.errorf("unterminated array")
		}
		if parser.peek() == ']' {
			parser.pos++
			return values, nil
		}
		value, err := parser.parseValue()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		parser.skipWaste()
		if parser.eof() {
			return nil, parser.errorf("unterminated array")
		}
		if parser.peek() == ',' {
			parser.pos++
			continue
		}
		if parser.peek() == ']' {
			parser.pos++
			return values, nil
		}
		return nil, parser.errorf("expected `,` or `]`")
	}
}

func (parser *tomlParser) parseInlineTable() (any, error) {
	parser.pos++
	table := tomlTable{}
	parser.skipSpaces()
	if parser.eof() {
		return nil, parser.errorf("unterminated inline table")
	}
	if parser.peek() == '}' {
		parser.pos++
		return table, nil
	}
	for {
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
		if err := assignTOMLPath(table, path, value); err != nil {
			return nil, parser.wrap(err)
		}
		parser.skipSpaces()
		if parser.eof() {
			return nil, parser.errorf("unterminated inline table")
		}
		if parser.peek() == ',' {
			parser.pos++
			parser.skipSpaces()
			continue
		}
		if parser.peek() == '}' {
			parser.pos++
			return table, nil
		}
		return nil, parser.errorf("expected `,` or `}`")
	}
}

func (parser *tomlParser) parseString() (string, error) {
	quote := parser.peek()
	triple := strings.HasPrefix(parser.text[parser.pos:], string([]byte{quote, quote, quote}))
	if triple {
		parser.pos += 3
		return parser.parseMultilineString(quote)
	}
	parser.pos++
	var builder strings.Builder
	for {
		if parser.eof() {
			return "", parser.errorf("unterminated string")
		}
		value := parser.peek()
		if value == quote {
			parser.pos++
			return builder.String(), nil
		}
		if value == '\n' || value == '\r' {
			return "", parser.errorf("unterminated string")
		}
		if quote == '"' && value == '\\' {
			character, err := parser.parseEscape()
			if err != nil {
				return "", err
			}
			builder.WriteString(character)
			continue
		}
		builder.WriteByte(value)
		parser.pos++
	}
}

func (parser *tomlParser) parseMultilineString(quote byte) (string, error) {
	var builder strings.Builder
	if strings.HasPrefix(parser.text[parser.pos:], "\r\n") {
		parser.pos += 2
	} else if !parser.eof() && parser.peek() == '\n' {
		parser.pos++
	}
	closing := string([]byte{quote, quote, quote})
	for {
		if parser.eof() {
			return "", parser.errorf("unterminated string")
		}
		if strings.HasPrefix(parser.text[parser.pos:], closing) {
			parser.pos += 3
			return builder.String(), nil
		}
		value := parser.peek()
		if quote == '"' && value == '\\' {
			if parser.consumeEscapeNewline() {
				continue
			}
			character, err := parser.parseEscape()
			if err != nil {
				return "", err
			}
			builder.WriteString(character)
			continue
		}
		builder.WriteByte(value)
		parser.pos++
	}
}

func (parser *tomlParser) consumeEscapeNewline() bool {
	rest := parser.text[parser.pos:]
	if !strings.HasPrefix(rest, "\\") {
		return false
	}
	index := 1
	for index < len(rest) && (rest[index] == ' ' || rest[index] == '\t') {
		index++
	}
	if index >= len(rest) || (rest[index] != '\n' && rest[index] != '\r') {
		return false
	}
	for index < len(rest) && (rest[index] == ' ' || rest[index] == '\t' || rest[index] == '\r' || rest[index] == '\n') {
		index++
	}
	parser.pos += index
	return true
}

func (parser *tomlParser) parseEscape() (string, error) {
	parser.pos++
	if parser.eof() {
		return "", parser.errorf("unterminated escape sequence")
	}
	value := parser.peek()
	parser.pos++
	switch value {
	case 'b':
		return "\b", nil
	case 't':
		return "\t", nil
	case 'n':
		return "\n", nil
	case 'f':
		return "\f", nil
	case 'r':
		return "\r", nil
	case '"':
		return "\"", nil
	case '\\':
		return "\\", nil
	case 'u', 'U':
		length := 4
		if value == 'U' {
			length = 8
		}
		if parser.pos+length > len(parser.text) {
			return "", parser.errorf("invalid unicode escape")
		}
		digits := parser.text[parser.pos : parser.pos+length]
		code, err := strconv.ParseUint(digits, 16, 32)
		if err != nil {
			return "", parser.errorf("invalid unicode escape")
		}
		parser.pos += length
		return string(rune(code)), nil
	default:
		return "", parser.errorf("invalid escape sequence")
	}
}

func (parser *tomlParser) parseNumber() (any, error) {
	start := parser.pos
	for !parser.eof() && !isValueDelimiter(parser.peek()) {
		parser.pos++
	}
	token := parser.text[start:parser.pos]
	if token == "" {
		return nil, parser.errorf("expected a value")
	}
	switch token {
	case "inf", "+inf":
		return math.Inf(1), nil
	case "-inf":
		return math.Inf(-1), nil
	case "nan", "+nan", "-nan":
		return math.NaN(), nil
	}
	if value, ok := parseTOMLInteger(token); ok {
		return value, nil
	}
	cleaned := strings.ReplaceAll(token, "_", "")
	if value, err := strconv.ParseFloat(cleaned, 64); err == nil {
		return value, nil
	}
	return nil, parser.errorf("invalid value %q", token)
}

func parseTOMLInteger(token string) (any, bool) {
	cleaned := strings.ReplaceAll(token, "_", "")
	sign := int64(1)
	base := 10
	digits := cleaned
	if strings.HasPrefix(digits, "+") {
		digits = digits[1:]
	} else if strings.HasPrefix(digits, "-") {
		sign = -1
		digits = digits[1:]
	}
	if len(digits) > 2 && digits[0] == '0' {
		switch digits[1] {
		case 'x':
			base = 16
		case 'o':
			base = 8
		case 'b':
			base = 2
		}
		if base != 10 {
			digits = digits[2:]
		}
	}
	value, err := strconv.ParseUint(digits, base, 64)
	if err != nil {
		return nil, false
	}
	if sign < 0 {
		if value > uint64(1)<<63 {
			return nil, false
		}
		return -int64(value), true
	}
	if value > uint64(1)<<63-1 {
		return value, true
	}
	return int64(value), true
}
