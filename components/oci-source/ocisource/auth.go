package ocisource

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type tokenResponse struct {
	Token       *string `json:"token"`
	AccessToken *string `json:"access_token"`
}

// bearerToken exchanges a Bearer challenge for a registry access token.
func bearerToken(challenge string, fallbackScope string) (*string, error) {
	parameters, found := strings.CutPrefix(challenge, "Bearer ")
	if !found {
		return nil, errors.New("OCI registry did not offer Bearer authentication")
	}
	values := map[string]string{}
	for _, part := range strings.Split(parameters, ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		values[name] = strings.Trim(value, "\"")
	}
	realm, found := values["realm"]
	if !found {
		return nil, errors.New("OCI Bearer challenge has no realm")
	}
	var query []string
	if service, ok := values["service"]; ok {
		query = append(query, "service="+encode(service))
	}
	scope, ok := values["scope"]
	if !ok {
		scope = fallbackScope
	}
	query = append(query, "scope="+encode(scope))
	separator := "?"
	if strings.Contains(realm, "?") {
		separator = "&"
	}
	response, err := get(realm+separator+strings.Join(query, "&"), "application/json", tokenLimit, nil)
	if err != nil {
		return nil, err
	}
	if err := ensureSuccess(response, "token"); err != nil {
		return nil, err
	}
	var parsed tokenResponse
	if err := json.Unmarshal(response.Body, &parsed); err != nil {
		return nil, fmt.Errorf("invalid OCI token response: %s", err)
	}
	token := parsed.Token
	if token == nil {
		token = parsed.AccessToken
	}
	if token == nil || *token == "" {
		return nil, errors.New("OCI token response has no token")
	}
	return token, nil
}

// encode percent-encodes every byte outside the unreserved URL set.
func encode(value string) string {
	var builder strings.Builder
	for index := 0; index < len(value); index++ {
		byte := value[index]
		switch {
		case byte >= 'A' && byte <= 'Z',
			byte >= 'a' && byte <= 'z',
			byte >= '0' && byte <= '9',
			byte == '-', byte == '_', byte == '.', byte == '~':
			builder.WriteByte(byte)
		default:
			fmt.Fprintf(&builder, "%%%02X", byte)
		}
	}
	return builder.String()
}
