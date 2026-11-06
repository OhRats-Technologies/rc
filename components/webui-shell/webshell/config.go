package webshell

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"wit_component/ohrats_rc_http_types"
	"wit_component/ohrats_rc_plugin_state_store"
)

const (
	publicSignupKey = "public-signup"
	publicURLKey    = "public-url"
)

const configUsage = "usage: rc webui-config [public-signup true|false|public-url URL|auto]"

func readState(name string) ([]byte, bool, error) {
	result := ohrats_rc_plugin_state_store.Read(name)
	if result.IsErr() {
		return nil, false, errors.New(result.Err())
	}
	option := result.Ok()
	if option.IsNone() {
		return nil, false, nil
	}
	return option.Some(), true, nil
}

func writeState(name string, value []byte) error {
	result := ohrats_rc_plugin_state_store.Write(name, value)
	if result.IsErr() {
		return errors.New(result.Err())
	}
	return nil
}

func removeState(name string) error {
	result := ohrats_rc_plugin_state_store.Remove(name)
	if result.IsErr() {
		return errors.New(result.Err())
	}
	return nil
}

// PublicSignup reports the deployment signup policy; failures read as disabled.
func PublicSignup() bool {
	value, ok, err := readState(publicSignupKey)
	return err == nil && ok && string(value) == "true"
}

// PublicURL resolves the externally visible origin for the current request.
func PublicURL(request ohrats_rc_http_types.Request) string {
	if value, ok, err := readState(publicURLKey); err == nil && ok {
		if text, valid := decodeUTF8(value); valid && validURL(text) {
			return strings.TrimRight(text, "/")
		}
	}
	scheme := request.Scheme
	if value, ok := forwarded(request, "x-forwarded-proto"); ok && (value == "http" || value == "https") {
		scheme = value
	}
	authority := request.Authority
	if value, ok := forwarded(request, "x-forwarded-host"); ok && validAuthority(value) {
		authority = value
	}
	return strings.TrimRight(fmt.Sprintf("%s://%s", scheme, authority), "/")
}

// InvokeConfig reads or updates deployment configuration from the CLI.
func InvokeConfig(args []string) (uint32, error) {
	switch {
	case len(args) == 0:
		fmt.Printf("public-signup\t%t\n", PublicSignup())
		value, ok, err := readState(publicURLKey)
		if err != nil {
			return 0, err
		}
		url := "auto"
		if ok {
			if text, valid := decodeUTF8(value); valid {
				url = text
			}
		}
		fmt.Printf("public-url\t%s\n", url)
		return 0, nil
	case len(args) == 2 && args[0] == publicSignupKey && (args[1] == "true" || args[1] == "false"):
		return 0, writeState(publicSignupKey, []byte(args[1]))
	case len(args) == 2 && args[0] == publicURLKey && args[1] == "auto":
		return 0, removeState(publicURLKey)
	case len(args) == 2 && args[0] == publicURLKey && validURL(args[1]):
		return 0, writeState(publicURLKey, []byte(strings.TrimRight(args[1], "/")))
	default:
		return 0, errors.New(configUsage)
	}
}

func decodeUTF8(value []byte) (string, bool) {
	if !utf8.Valid(value) {
		return "", false
	}
	return string(value), true
}

func forwarded(request ohrats_rc_http_types.Request, name string) (string, bool) {
	for _, header := range request.Headers {
		if !strings.EqualFold(header.Name, name) {
			continue
		}
		part, _, _ := strings.Cut(header.Value, ",")
		value := strings.TrimSpace(part)
		if value == "" {
			return "", false
		}
		return value, true
	}
	return "", false
}

func validURL(value string) bool {
	if !strings.HasPrefix(value, "https://") && !strings.HasPrefix(value, "http://") {
		return false
	}
	for index := 0; index < len(value); index++ {
		byte := value[index]
		if byte < 0x20 || byte == 0x7f || byte == ' ' {
			return false
		}
	}
	return true
}

func validAuthority(value string) bool {
	if len(value) == 0 || len(value) > 255 {
		return false
	}
	for index := 0; index < len(value); index++ {
		byte := value[index]
		if byte < 0x20 || byte == 0x7f || byte == ' ' || byte == '/' || byte == '\\' {
			return false
		}
	}
	return true
}
