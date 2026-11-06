package storagefixture

import (
	"fmt"
	"strings"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
)

func noneKey() witTypes.Option[[]uint8] {
	return witTypes.None[[]uint8]()
}

// quoted renders a string the way Rust's `{:?}` renders a str.
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

// lossyUTF8 mirrors Rust's String::from_utf8_lossy: every invalid sequence is
// replaced with U+FFFD, and an incomplete trailing sequence is replaced once.
func lossyUTF8(value []byte) string {
	var builder strings.Builder
	for index := 0; index < len(value); {
		character, size := decodeRune(value[index:])
		if character != -1 {
			builder.WriteRune(character)
			index += size
			continue
		}
		builder.WriteRune('\uFFFD')
		if size == 0 {
			index = len(value)
		} else {
			index += size
		}
	}
	return builder.String()
}

func decodeRune(value []byte) (rune, int) {
	length := expectedLength(value[0])
	if length == 0 {
		return -1, 1
	}
	if length > len(value) {
		for index := 1; index < len(value); index++ {
			if value[index]&0xC0 != 0x80 {
				return -1, 1
			}
		}
		return -1, 0
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
	character := rune(0)
	switch len(value) {
	case 1:
		return rune(value[0]), 1
	case 2:
		character = rune(value[0]&0x1F)<<6 | rune(value[1]&0x3F)
	case 3:
		character = rune(value[0]&0x0F)<<12 | rune(value[1]&0x3F)<<6 | rune(value[2]&0x3F)
	case 4:
		character = rune(value[0]&0x07)<<18 | rune(value[1]&0x3F)<<12 |
			rune(value[2]&0x3F)<<6 | rune(value[3]&0x3F)
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
	return character, len(value)
}
