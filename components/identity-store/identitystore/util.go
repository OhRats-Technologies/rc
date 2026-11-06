package identitystore

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"
)

func nowMs() uint64 {
	value := time.Now().UnixMilli()
	if value < 0 {
		return 0
	}
	return uint64(value)
}

// byteArray encodes a byte vector as a JSON number array, matching the serde
// representation the stored records were written with.
type byteArray []uint8

func (value byteArray) MarshalJSON() ([]byte, error) {
	out := make([]byte, 0, 2+len(value)*4)
	out = append(out, '[')
	for index, item := range value {
		if index > 0 {
			out = append(out, ',')
		}
		out = strconv.AppendUint(out, uint64(item), 10)
	}
	out = append(out, ']')
	return out, nil
}

func (value *byteArray) UnmarshalJSON(data []byte) error {
	var numbers []uint16
	if err := json.Unmarshal(data, &numbers); err != nil {
		return err
	}
	result := make(byteArray, len(numbers))
	for index, number := range numbers {
		if number > 0xff {
			return errors.New("invalid stored byte array")
		}
		result[index] = byte(number)
	}
	*value = result
	return nil
}

// emptyByteArray keeps an absent byte vector encoded as `[]`, never `null`.
func emptyByteArray(value byteArray) byteArray {
	if value == nil {
		return byteArray{}
	}
	return value
}

func encode(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New(err.Error())
	}
	return encoded, nil
}

func decode[T any](value []byte) (T, error) {
	var decoded T
	if err := json.Unmarshal(value, &decoded); err != nil {
		return decoded, errors.New(err.Error())
	}
	return decoded, nil
}

func random(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return nil, errors.New(err.Error())
	}
	return value, nil
}

func randomToken(size int) (string, error) {
	value, err := random(size)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func randomHexID(size int) (string, error) {
	value, err := random(size)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func digest(value []byte) []byte {
	sum := sha256.Sum256(value)
	return sum[:]
}

// utf8String converts a stored byte vector to text and reports the same
// diagnostics as a strict UTF-8 decode of the record.
func utf8String(value []byte) (string, error) {
	if utf8.Valid(value) {
		return string(value), nil
	}
	index := 0
	for index < len(value) {
		decoded, size := utf8.DecodeRune(value[index:])
		if decoded != utf8.RuneError || size > 1 {
			index += size
			continue
		}
		if length, ok := invalidSequenceLength(value[index:]); ok {
			return "", fmt.Errorf("invalid utf-8 sequence of %d bytes from index %d", length, index)
		}
		return "", fmt.Errorf("incomplete utf-8 byte sequence from index %d", index)
	}
	return "", fmt.Errorf("invalid utf-8 sequence of 1 bytes from index %d", index-1)
}

// invalidSequenceLength reports how many bytes the erroneous sequence spans,
// distinguishing a malformed sequence from a truncated one at the buffer end.
func invalidSequenceLength(value []byte) (int, bool) {
	leading := value[0]
	var expected int
	switch {
	case leading >= 0xc2 && leading <= 0xdf:
		expected = 2
	case leading >= 0xe0 && leading <= 0xef:
		expected = 3
	case leading >= 0xf0 && leading <= 0xf4:
		expected = 4
	default:
		return 1, true
	}
	for index := 1; index < expected && index < len(value); index++ {
		if value[index]&0xc0 != 0x80 {
			return index, true
		}
	}
	if len(value) >= expected {
		return expected, true
	}
	return 0, false
}
