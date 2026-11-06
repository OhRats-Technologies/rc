package packagemanager

import (
	"fmt"
	"strings"
)

func encodeTOMLString(value string) string {
	var builder strings.Builder
	builder.WriteByte('"')
	for _, character := range value {
		switch character {
		case '"':
			builder.WriteString(`\"`)
		case '\\':
			builder.WriteString(`\\`)
		case '\b':
			builder.WriteString(`\b`)
		case '\t':
			builder.WriteString(`\t`)
		case '\n':
			builder.WriteString(`\n`)
		case '\f':
			builder.WriteString(`\f`)
		case '\r':
			builder.WriteString(`\r`)
		default:
			if character < 0x20 || character == 0x7f {
				fmt.Fprintf(&builder, `\u%04X`, character)
			} else {
				builder.WriteRune(character)
			}
		}
	}
	builder.WriteByte('"')
	return builder.String()
}

func encodeTOMLKey(value string) string {
	if value != "" {
		bare := true
		for index := 0; index < len(value); index++ {
			if !isBareKeyByte(value[index]) {
				bare = false
				break
			}
		}
		if bare {
			return value
		}
	}
	return encodeTOMLString(value)
}
