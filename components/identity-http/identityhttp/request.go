package identityhttp

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"wit_component/ohrats_rc_http_types"
)

const maxRequestBodyBytes = 1024 * 1024

func headerValue(request ohrats_rc_http_types.Request, name string) (string, bool) {
	for _, value := range request.Headers {
		if strings.EqualFold(value.Name, name) {
			return value.Value, true
		}
	}
	return "", false
}

func cookieHeader(request ohrats_rc_http_types.Request) string {
	value, _ := headerValue(request, "cookie")
	return value
}

func jsonBody[T any](request ohrats_rc_http_types.Request) (T, error) {
	var result T
	if len(request.Body) > maxRequestBodyBytes {
		return result, errors.New("request body is too large")
	}
	if err := json.Unmarshal(request.Body, &result); err != nil {
		return result, errors.New("invalid request")
	}
	return result, nil
}

func queryParameters(request ohrats_rc_http_types.Request) map[string]string {
	result := map[string]string{}
	for _, part := range strings.Split(request.Query, "&") {
		if part == "" {
			continue
		}
		key, value, found := strings.Cut(part, "=")
		if !found {
			value = ""
		}
		decodedKey, ok := decodeQueryComponent(key)
		if !ok {
			continue
		}
		decodedValue, ok := decodeQueryComponent(value)
		if !ok {
			continue
		}
		result[decodedKey] = decodedValue
	}
	return result
}

func safeNext(value string) bool {
	if !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || len(value) > 2048 {
		return false
	}
	return strings.IndexFunc(value, unicode.IsControl) < 0
}

func decodeQueryComponent(value string) (string, bool) {
	bytes := []byte(value)
	result := make([]byte, 0, len(bytes))
	for index := 0; index < len(bytes); index++ {
		switch bytes[index] {
		case '+':
			result = append(result, ' ')
		case '%':
			if index+2 < len(bytes) {
				high, highOK := hexDigit(bytes[index+1])
				low, lowOK := hexDigit(bytes[index+2])
				if !highOK || !lowOK {
					return "", false
				}
				result = append(result, high<<4|low)
				index += 2
			} else {
				return "", false
			}
		default:
			result = append(result, bytes[index])
		}
	}
	if !utf8.Valid(result) {
		return "", false
	}
	return string(result), true
}

func hexDigit(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}
