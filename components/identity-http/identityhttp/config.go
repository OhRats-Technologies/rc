package identityhttp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"wit_component/ohrats_rc_http_types"
	"wit_component/ohrats_rc_plugin_state_store"
)

const (
	publicURLKey     = "public-url"
	setupTokenKey    = "setup-token-hash"
	setupTokenCookie = "rc_setup"
)

// RelyingParty is the deployment identity used for WebAuthn ceremonies.
type RelyingParty struct {
	ID     string
	Origin string
	Secure bool
}

// RelyingPartyFromRequest derives the relying party from configured or
// forwarded deployment URLs.
func RelyingPartyFromRequest(request ohrats_rc_http_types.Request) (RelyingParty, error) {
	origin := publicURL(request)
	scheme, rest, found := strings.Cut(origin, "://")
	if !found {
		return RelyingParty{}, errors.New("invalid identity public URL")
	}
	authority := rest
	if index := strings.IndexByte(rest, '/'); index >= 0 {
		authority = rest[:index]
	}
	id := ""
	if strings.HasPrefix(authority, "[") {
		if index := strings.IndexByte(authority, ']'); index >= 0 {
			id = authority[:index+1]
		} else {
			id = authority + "]"
		}
	} else {
		id = authority
		if index := strings.IndexByte(authority, ':'); index >= 0 {
			id = authority[:index]
		}
	}
	if id == "" || len(id) > 255 {
		return RelyingParty{}, errors.New("invalid identity relying-party id")
	}
	secure := scheme == "https"
	if !secure && id != "localhost" {
		return RelyingParty{}, errors.New("passkeys require HTTPS or localhost")
	}
	return RelyingParty{ID: id, Origin: origin, Secure: secure}, nil
}

func publicURL(request ohrats_rc_http_types.Request) string {
	if stored, found, err := readState(publicURLKey); err == nil && found && utf8.Valid(stored) {
		if value := string(stored); validURL(value) {
			return strings.TrimRight(value, "/")
		}
	}
	scheme := request.Scheme
	if forwarded, ok := forwardedValue(request, "x-forwarded-proto"); ok &&
		(forwarded == "http" || forwarded == "https") {
		scheme = forwarded
	}
	authority := request.Authority
	if forwarded, ok := forwardedValue(request, "x-forwarded-host"); ok &&
		validAuthority(forwarded) {
		authority = forwarded
	}
	return strings.TrimRight(scheme+"://"+authority, "/")
}

func setupAuthorized(cookie string) (bool, error) {
	expected, found, err := readState(setupTokenKey)
	if err != nil {
		return false, err
	}
	if !found {
		return true, nil
	}
	value, ok := cookieValue(cookie, setupTokenCookie)
	if !ok {
		return false, nil
	}
	return value == hexString(expected), nil
}

func setupCookie(token string, secure bool) (string, bool, error) {
	expected, found, err := readState(setupTokenKey)
	if err != nil {
		return "", false, err
	}
	if !found {
		return "", false, nil
	}
	digest := sha256.Sum256([]byte(token))
	if !bytes.Equal(digest[:], expected) {
		return "", false, errors.New("invalid setup link")
	}
	suffix := ""
	if secure {
		suffix = "; Secure"
	}
	return setupTokenCookie + "=" + hexString(digest[:]) +
		"; Path=/; HttpOnly; SameSite=Strict; Max-Age=900" + suffix, true, nil
}

// Invoke implements the identity-config command.
func Invoke(args []string) (uint32, error) {
	if len(args) == 0 {
		stored, found, err := readState(publicURLKey)
		if err != nil {
			return 0, err
		}
		url := "auto"
		if found && utf8.Valid(stored) {
			url = string(stored)
		}
		fmt.Printf("public-url\t%s\n", url)
		_, configured, err := readState(setupTokenKey)
		if err != nil {
			return 0, err
		}
		token := "none"
		if configured {
			token = "configured"
		}
		fmt.Printf("setup-token\t%s\n", token)
		return 0, nil
	}
	if len(args) != 2 {
		return 0, errors.New("usage: rc identity-config [public-url URL|auto|setup-token TOKEN|none]")
	}
	key := args[0]
	value := args[1]
	switch {
	case key == publicURLKey && value == "auto":
		return 0, removeState(publicURLKey)
	case key == publicURLKey && validURL(value):
		return 0, writeState(publicURLKey, []byte(strings.TrimRight(value, "/")))
	case key == "setup-token" && value == "none":
		return 0, removeState(setupTokenKey)
	case key == "setup-token" && validSetupToken(value):
		digest := sha256.Sum256([]byte(value))
		return 0, writeState(setupTokenKey, digest[:])
	default:
		return 0, errors.New("usage: rc identity-config [public-url URL|auto|setup-token TOKEN|none]")
	}
}

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

func cookieValue(header string, name string) (string, bool) {
	for _, part := range strings.Split(header, ";") {
		key, value, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found || key != name {
			continue
		}
		return value, true
	}
	return "", false
}

func forwardedValue(request ohrats_rc_http_types.Request, name string) (string, bool) {
	for _, value := range request.Headers {
		if !strings.EqualFold(value.Name, name) {
			continue
		}
		first := value.Value
		if index := strings.IndexByte(first, ','); index >= 0 {
			first = first[:index]
		}
		trimmed := strings.TrimSpace(first)
		if trimmed == "" {
			return "", false
		}
		return trimmed, true
	}
	return "", false
}

func validURL(value string) bool {
	if !strings.HasPrefix(value, "https://") && !strings.HasPrefix(value, "http://") {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] <= 0x20 || value[index] == 0x7f {
			return false
		}
	}
	return true
}

func validAuthority(value string) bool {
	if value == "" || len(value) > 255 {
		return false
	}
	for index := 0; index < len(value); index++ {
		byteValue := value[index]
		if byteValue <= 0x20 || byteValue == 0x7f || byteValue == '/' || byteValue == '\\' {
			return false
		}
	}
	return true
}

func validSetupToken(value string) bool {
	if len(value) < 24 || len(value) > 256 {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < 0x21 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

func hexString(value []byte) string {
	return hex.EncodeToString(value)
}
