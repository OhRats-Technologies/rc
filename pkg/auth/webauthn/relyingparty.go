package webauthn

import (
	"errors"
	"strings"
)

const maxAuthorityBytes = 255

// RelyingPartyFromURL derives the relying party from a configured public URL,
// such as the deployment's canonical origin. It reports the same failures as
// the identity configuration path.
func RelyingPartyFromURL(publicURL string) (RelyingParty, error) {
	value := strings.TrimSpace(publicURL)
	if !validURL(value) {
		return RelyingParty{}, errors.New("invalid identity public URL")
	}
	return relyingPartyFromOrigin(strings.TrimRight(value, "/"))
}

// RelyingPartyFromRequest derives the relying party from an explicitly
// configured public URL or, when that is empty, from the request scheme,
// authority, and X-Forwarded-Proto/X-Forwarded-Host headers. Headers are given
// as name/value pairs so this package needs no net/http dependency.
func RelyingPartyFromRequest(
	publicURL string,
	scheme string,
	authority string,
	headers [][2]string,
) (RelyingParty, error) {
	if value := strings.TrimSpace(publicURL); value != "" {
		return RelyingPartyFromURL(value)
	}
	if forwarded, ok := forwardedValue(headers, "x-forwarded-proto"); ok &&
		(forwarded == "http" || forwarded == "https") {
		scheme = forwarded
	}
	if forwarded, ok := forwardedValue(headers, "x-forwarded-host"); ok &&
		validAuthority(forwarded) {
		authority = forwarded
	}
	if !validAuthority(authority) {
		return RelyingParty{}, errors.New("invalid identity public URL")
	}
	return relyingPartyFromOrigin(strings.TrimRight(strings.ToLower(scheme)+"://"+authority, "/"))
}

func relyingPartyFromOrigin(origin string) (RelyingParty, error) {
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
	if id == "" || len(id) > maxAuthorityBytes {
		return RelyingParty{}, errors.New("invalid identity relying-party id")
	}
	if scheme != "https" && id != "localhost" {
		return RelyingParty{}, errors.New("passkeys require HTTPS or localhost")
	}
	return NewRelyingParty(id, origin)
}

func forwardedValue(headers [][2]string, name string) (string, bool) {
	for _, header := range headers {
		if !strings.EqualFold(header[0], name) {
			continue
		}
		first := header[1]
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
	if value == "" || len(value) > maxAuthorityBytes {
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
