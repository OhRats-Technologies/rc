package server

import (
	"encoding/base64"
	"strconv"
)

func decodeProcessInput(value, encoding string) ([]byte, error) {
	if encoding == "" || encoding == "text" {
		return []byte(value), nil
	}
	if encoding != "base64" {
		return nil, mcpArgumentError("encoding must be text or base64")
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	return nil, mcpArgumentError("data is not valid base64")
}

func minUint64(value, maximum uint64) uint64 {
	if value > maximum {
		return maximum
	}
	return value
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

func utoa(value uint64) string {
	return strconv.FormatUint(value, 10)
}

func processResultFailed(result mcpProcessResult) bool {
	return result.Status == "lost" || (result.Status == "exited" &&
		(result.ExitCode == nil || *result.ExitCode != 0 || result.Signal != nil))
}
