package webauthn

import (
	"math"
	"unicode/utf8"
)

// maxCBORDepth bounds nesting so hostile inputs cannot exhaust the stack.
const maxCBORDepth = 64

// cborMap preserves key order; last assignment wins, matching serde_cbor's
// BTreeMap-backed Value::Map.
type cborMap struct {
	keys   []any
	values []any
}

// cborTag stands in for a tagged CBOR value. The verifier only accepts plain
// maps and byte strings, so tags fail the shape checks that follow.
type cborTag struct {
	inner any
}

type cborDecoder struct {
	data  []byte
	off   int
	depth int
}

func decodeCBOR(data []byte) (any, int, error) {
	decoder := &cborDecoder{data: data}
	value, err := decoder.value()
	if err != nil {
		return nil, 0, err
	}
	return value, decoder.off, nil
}

// decodeCBORValue decodes exactly one CBOR value and rejects trailing data.
func decodeCBORValue(data []byte) (any, error) {
	value, consumed, err := decodeCBOR(data)
	if err != nil {
		return nil, err
	}
	if consumed != len(data) {
		return nil, ErrInvalidCBOR
	}
	return value, nil
}

func (d *cborDecoder) value() (any, error) {
	if d.off >= len(d.data) || d.depth >= maxCBORDepth {
		return nil, ErrInvalidCBOR
	}
	initial := d.data[d.off]
	d.off++
	major := initial >> 5
	info := initial & 0x1f
	d.depth++
	defer func() { d.depth-- }()

	switch major {
	case 0:
		value, err := d.argument(info)
		if err != nil {
			return nil, err
		}
		if value > math.MaxInt64 {
			return nil, ErrInvalidCBOR
		}
		return int64(value), nil
	case 1:
		value, err := d.argument(info)
		if err != nil {
			return nil, err
		}
		if value > math.MaxInt64 {
			return nil, ErrInvalidCBOR
		}
		return -1 - int64(value), nil
	case 2:
		return d.byteString(2, info)
	case 3:
		raw, err := d.byteString(3, info)
		if err != nil {
			return nil, err
		}
		if !utf8.Valid(raw) {
			return nil, ErrInvalidCBOR
		}
		return string(raw), nil
	case 4:
		return d.array(info)
	case 5:
		return d.mapping(info)
	case 6:
		if _, err := d.argument(info); err != nil {
			return nil, err
		}
		inner, err := d.value()
		if err != nil {
			return nil, err
		}
		return cborTag{inner: inner}, nil
	case 7:
		switch info {
		case 20:
			return false, nil
		case 21:
			return true, nil
		case 22:
			return nil, nil
		default:
			return nil, ErrInvalidCBOR
		}
	default:
		return nil, ErrInvalidCBOR
	}
}

func (d *cborDecoder) argument(info byte) (uint64, error) {
	switch {
	case info < 24:
		return uint64(info), nil
	case info == 24:
		raw, err := d.take(1)
		if err != nil {
			return 0, err
		}
		return uint64(raw[0]), nil
	case info == 25:
		raw, err := d.take(2)
		if err != nil {
			return 0, err
		}
		return uint64(raw[0])<<8 | uint64(raw[1]), nil
	case info == 26:
		raw, err := d.take(4)
		if err != nil {
			return 0, err
		}
		return uint64(raw[0])<<24 | uint64(raw[1])<<16 | uint64(raw[2])<<8 | uint64(raw[3]), nil
	case info == 27:
		raw, err := d.take(8)
		if err != nil {
			return 0, err
		}
		var value uint64
		for _, item := range raw {
			value = value<<8 | uint64(item)
		}
		return value, nil
	default:
		return 0, ErrInvalidCBOR
	}
}

func (d *cborDecoder) take(length int) ([]byte, error) {
	if length < 0 || len(d.data)-d.off < length {
		return nil, ErrInvalidCBOR
	}
	raw := d.data[d.off : d.off+length]
	d.off += length
	return raw, nil
}

// byteString decodes a definite or indefinite string of the given major type,
// copying the result so returned key material never aliases the caller's
// buffer.
func (d *cborDecoder) byteString(major byte, info byte) ([]byte, error) {
	if info != 31 {
		length, err := d.argument(info)
		if err != nil {
			return nil, err
		}
		raw, err := d.take(int(length))
		if err != nil {
			return nil, err
		}
		result := make([]byte, len(raw))
		copy(result, raw)
		return result, nil
	}
	result := []byte{}
	for {
		if d.off >= len(d.data) {
			return nil, ErrInvalidCBOR
		}
		if d.data[d.off] == 0xff {
			d.off++
			return result, nil
		}
		initial := d.data[d.off]
		d.off++
		if initial>>5 != major || initial&0x1f == 31 {
			return nil, ErrInvalidCBOR
		}
		length, err := d.argument(initial & 0x1f)
		if err != nil {
			return nil, err
		}
		raw, err := d.take(int(length))
		if err != nil {
			return nil, err
		}
		result = append(result, raw...)
	}
}

func (d *cborDecoder) array(info byte) (any, error) {
	if info == 31 {
		values := []any{}
		for {
			if d.off >= len(d.data) {
				return nil, ErrInvalidCBOR
			}
			if d.data[d.off] == 0xff {
				d.off++
				return values, nil
			}
			value, err := d.value()
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
	}
	length, err := d.argument(info)
	if err != nil {
		return nil, err
	}
	if length > uint64(len(d.data)-d.off) {
		return nil, ErrInvalidCBOR
	}
	values := make([]any, 0, int(length))
	for index := uint64(0); index < length; index++ {
		value, err := d.value()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func (d *cborDecoder) mapping(info byte) (any, error) {
	result := cborMap{}
	if info != 31 {
		length, err := d.argument(info)
		if err != nil {
			return nil, err
		}
		if length > uint64(len(d.data)-d.off) {
			return nil, ErrInvalidCBOR
		}
		for index := uint64(0); index < length; index++ {
			key, err := d.value()
			if err != nil {
				return nil, err
			}
			value, err := d.value()
			if err != nil {
				return nil, err
			}
			result.keys = append(result.keys, key)
			result.values = append(result.values, value)
		}
		return result, nil
	}
	for {
		if d.off >= len(d.data) {
			return nil, ErrInvalidCBOR
		}
		if d.data[d.off] == 0xff {
			d.off++
			return result, nil
		}
		key, err := d.value()
		if err != nil {
			return nil, err
		}
		value, err := d.value()
		if err != nil {
			return nil, err
		}
		result.keys = append(result.keys, key)
		result.values = append(result.values, value)
	}
}
