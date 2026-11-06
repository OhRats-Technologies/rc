package packagemanager

import (
	"fmt"
	"strconv"
	"strings"
)

func formatUint(value uint64) string {
	return strconv.FormatUint(value, 10)
}

// quoted renders a string the way Rust's `{:?}` renders a str, which is what
// the original component used for identifiers embedded in error messages.
func quoted(value string) string {
	var builder strings.Builder
	builder.WriteByte('"')
	for _, character := range value {
		switch character {
		case '"':
			builder.WriteString(`\"`)
		case '\\':
			builder.WriteString(`\\`)
		case '\n':
			builder.WriteString(`\n`)
		case '\r':
			builder.WriteString(`\r`)
		case '\t':
			builder.WriteString(`\t`)
		case 0:
			builder.WriteString(`\0`)
		default:
			if character < 0x20 || character == 0x7f {
				fmt.Fprintf(&builder, `\u{%x}`, character)
			} else {
				builder.WriteRune(character)
			}
		}
	}
	builder.WriteByte('"')
	return builder.String()
}

func quotedChar(character rune) string {
	var builder strings.Builder
	builder.WriteByte('\'')
	switch character {
	case '\'':
		builder.WriteString(`\'`)
	case '"':
		builder.WriteString(`\"`)
	case '\\':
		builder.WriteString(`\\`)
	case '\n':
		builder.WriteString(`\n`)
	case '\r':
		builder.WriteString(`\r`)
	case '\t':
		builder.WriteString(`\t`)
	case 0:
		builder.WriteString(`\0`)
	default:
		if character < 0x20 || character == 0x7f {
			fmt.Fprintf(&builder, `\u{%x}`, character)
		} else {
			builder.WriteRune(character)
		}
	}
	builder.WriteByte('\'')
	return builder.String()
}

// decodeRune reports -1 for invalid input and the number of bytes to consume.
// A truncated trailing sequence consumes the remaining bytes once so lossy
// conversion emits a single replacement character.
func decodeRune(value []byte) (rune, int) {
	length := expectedLength(value[0])
	if length == 0 {
		return -1, 1
	}
	if length > len(value) {
		validPrefix := true
		for index := 1; index < len(value); index++ {
			if value[index]&0xC0 != 0x80 {
				validPrefix = false
				break
			}
		}
		if validPrefix {
			return -1, 0
		}
		return -1, 1
	}
	character, size := decodeFullRune(value[:length])
	if size == 0 {
		return -1, 1
	}
	return character, size
}

func expectedLength(lead byte) int {
	switch {
	case lead < 0x80:
		return 1
	case lead >= 0xC2 && lead <= 0xDF:
		return 2
	case lead >= 0xE0 && lead <= 0xEF:
		return 3
	case lead >= 0xF0 && lead <= 0xF4:
		return 4
	default:
		return 0
	}
}

func decodeFullRune(value []byte) (rune, int) {
	character, size := rune(0), 0
	switch len(value) {
	case 1:
		return rune(value[0]), 1
	case 2:
		character = rune(value[0]&0x1F)<<6 | rune(value[1]&0x3F)
		size = 2
	case 3:
		character = rune(value[0]&0x0F)<<12 | rune(value[1]&0x3F)<<6 | rune(value[2]&0x3F)
		size = 3
	case 4:
		character = rune(value[0]&0x07)<<18 | rune(value[1]&0x3F)<<12 | rune(value[2]&0x3F)<<6 | rune(value[3]&0x3F)
		size = 4
	default:
		return -1, 0
	}
	for index := 1; index < len(value); index++ {
		if value[index]&0xC0 != 0x80 {
			return -1, 0
		}
	}
	if expectedLength(value[0]) != len(value) {
		return -1, 0
	}
	if character > 0x10FFFF || (character >= 0xD800 && character <= 0xDFFF) {
		return -1, 0
	}
	if len(value) == 3 && character < 0x800 {
		return -1, 0
	}
	if len(value) == 4 && character < 0x10000 {
		return -1, 0
	}
	return character, size
}
