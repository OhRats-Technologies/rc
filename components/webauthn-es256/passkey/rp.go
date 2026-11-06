package passkey

import (
	"crypto/sha256"
	"net"
	"net/url"
	"strings"
	"unicode"
)

// RelyingParty is a validated WebAuthn RP ID and accepted browser origin.
type RelyingParty struct {
	ID     string
	Origin string
}

// NewRelyingParty validates an RP ID and origin, normalizing both the same way
// passkey-rp's RelyingParty::new does.
func NewRelyingParty(id string, origin string) (RelyingParty, error) {
	normalizedID, err := normalizeRPID(id)
	if err != nil {
		return RelyingParty{}, err
	}
	normalizedOrigin, err := normalizeOrigin(origin)
	if err != nil {
		return RelyingParty{}, err
	}
	if err := validateRPIDForOrigin(normalizedID, normalizedOrigin); err != nil {
		return RelyingParty{}, err
	}
	return RelyingParty{ID: normalizedID, Origin: normalizedOrigin}, nil
}

func (value RelyingParty) idHash() []byte {
	digest := sha256.Sum256([]byte(value.ID))
	return digest[:]
}

func normalizeRPID(value string) (string, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimRight(value, ".")
	if value == "" || len(value) > 1024 || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return "", ErrInvalidRelyingPartyID
	}
	if strings.ContainsAny(value, ":/@?#[]\\%") {
		return "", ErrInvalidRelyingPartyID
	}
	id := strings.ToLower(value)
	if net.ParseIP(id) != nil {
		return "", ErrInvalidRelyingPartyID
	}
	if id == "localhost" {
		return id, nil
	}
	if len(id) > 253 {
		return "", ErrInvalidRelyingPartyID
	}
	labels := strings.Split(id, ".")
	if len(labels) < 2 {
		return "", ErrInvalidRelyingPartyID
	}
	for _, label := range labels {
		if !validDomainLabel(label) {
			return "", ErrInvalidRelyingPartyID
		}
	}
	if isPublicSuffix(id) {
		return "", ErrInvalidRelyingPartyID
	}
	return id, nil
}

func validDomainLabel(label string) bool {
	if label == "" || len(label) > 63 {
		return false
	}
	for index := 0; index < len(label); index++ {
		byteValue := label[index]
		alphanumeric := byteValue >= 'a' && byteValue <= 'z' ||
			byteValue >= '0' && byteValue <= '9'
		if !alphanumeric && byteValue != '-' {
			return false
		}
		if index == 0 || index == len(label)-1 {
			if !alphanumeric {
				return false
			}
		}
	}
	return true
}

// isPublicSuffix recognizes the common multi-label suffixes an RP ID may not
// use. It is a hand-maintained subset of the Public Suffix List.
func isPublicSuffix(id string) bool {
	switch id {
	case "co.uk", "org.uk", "ac.uk", "gov.uk", "me.uk", "net.uk",
		"com.au", "net.au", "org.au", "edu.au", "gov.au",
		"co.nz", "net.nz", "org.nz",
		"co.jp", "ne.jp", "or.jp",
		"com.br", "com.cn", "com.sg", "com.hk", "com.tw", "com.mx",
		"com.ar", "com.tr", "co.in", "co.za", "co.kr", "com.my",
		"com.ph", "com.vn", "co.id", "co.il", "com.pt", "com.es",
		"com.pl", "com.ru", "co.ua":
		return true
	default:
		return false
	}
}

func normalizeOrigin(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if strings.ContainsAny(trimmed, "?#") {
		return "", ErrInvalidOrigin
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", ErrInvalidOrigin
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrInvalidOrigin
	}
	if parsed.User != nil {
		_, hasPassword := parsed.User.Password()
		if parsed.User.Username() != "" || hasPassword {
			return "", ErrInvalidOrigin
		}
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", ErrInvalidOrigin
	}
	if parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", ErrInvalidOrigin
	}
	scheme := strings.ToLower(parsed.Scheme)
	host := parsed.Hostname()
	if host == "" {
		return "", ErrInvalidOrigin
	}
	switch scheme {
	case "https":
	case "http":
		if !isLoopback(host) {
			return "", ErrInvalidOrigin
		}
	default:
		return "", ErrInvalidOrigin
	}
	port := parsed.Port()
	if scheme == "https" && port == "443" || scheme == "http" && port == "80" {
		port = ""
	}
	host = strings.ToLower(host)
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		return scheme + "://" + host + ":" + port, nil
	}
	return scheme + "://" + host, nil
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func validateRPIDForOrigin(id string, origin string) error {
	parsed, err := url.Parse(origin)
	if err != nil {
		return ErrInvalidOrigin
	}
	host := parsed.Hostname()
	if host == "" {
		return ErrInvalidOrigin
	}
	host = strings.ToLower(host)
	if host == id {
		return nil
	}
	if strings.HasSuffix(host, id) {
		prefix := host[:len(host)-len(id)]
		if strings.HasSuffix(prefix, ".") {
			return nil
		}
	}
	return ErrInvalidRelyingPartyID
}
