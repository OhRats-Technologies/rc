package identityhttp

import (
	"bytes"
	"encoding/json"
	"strconv"

	"wit_component/ohrats_rc_http_types"
)

const contentSecurityPolicy = "default-src 'self'; style-src 'self' https://assets.ohrats.party https://fonts.googleapis.com; font-src https://assets.ohrats.party https://fonts.gstatic.com; img-src 'self' https://assets.ohrats.party data:; script-src 'self' https://assets.ohrats.party https://challenges.cloudflare.com; connect-src 'self' https://challenges.cloudflare.com; frame-src https://challenges.cloudflare.com; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"

func htmlResponse(status uint16, body string) ohrats_rc_http_types.Response {
	return buildResponse(status, "text/html; charset=utf-8", "no-store", []byte(body))
}

func javascriptResponse(body []byte) ohrats_rc_http_types.Response {
	return buildResponse(
		200,
		"text/javascript; charset=utf-8",
		"public, max-age=31536000, immutable",
		body,
	)
}

func jsonResponse(status uint16, value any) (ohrats_rc_http_types.Response, error) {
	encoded, err := marshalJSON(value)
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	return buildResponse(status, "application/json; charset=utf-8", "no-store", encoded), nil
}

// marshalJSON matches serde_json's compact output, which does not escape HTML
// characters.
func marshalJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func errorResponse(status uint16, message string) (ohrats_rc_http_types.Response, error) {
	return jsonResponse(status, map[string]any{"error": message})
}

func redirectResponse(location string) ohrats_rc_http_types.Response {
	response := buildResponse(303, "text/plain; charset=utf-8", "no-store", []byte{})
	response.Headers = append(response.Headers, header("location", location))
	return response
}

func withCookie(response ohrats_rc_http_types.Response, cookie string) ohrats_rc_http_types.Response {
	response.Headers = append(response.Headers, header("set-cookie", cookie))
	return response
}

func sessionCookie(token string, maxAgeSeconds uint64, secure bool) string {
	suffix := ""
	if secure {
		suffix = "; Secure"
	}
	return "rc_session=" + token + "; Path=/; HttpOnly; SameSite=Lax; Max-Age=" +
		strconv.FormatUint(maxAgeSeconds, 10) + suffix
}

func clearSessionCookie(secure bool) string {
	return sessionCookie("", 0, secure)
}

func buildResponse(status uint16, contentType string, cacheControl string, body []byte) ohrats_rc_http_types.Response {
	return ohrats_rc_http_types.Response{
		Status: status,
		Headers: []ohrats_rc_http_types.Header{
			header("content-type", contentType),
			header("cache-control", cacheControl),
			header("x-content-type-options", "nosniff"),
			header("x-frame-options", "DENY"),
			header("referrer-policy", "same-origin"),
			header("content-security-policy", contentSecurityPolicy),
		},
		Body: body,
	}
}

func header(name string, value string) ohrats_rc_http_types.Header {
	return ohrats_rc_http_types.Header{Name: name, Value: value}
}
