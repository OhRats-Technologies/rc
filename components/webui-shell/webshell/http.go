package webshell

import (
	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_http_types"
)

const contentSecurityPolicy = "default-src 'self'; style-src 'self' https://assets.ohrats.party https://fonts.googleapis.com; font-src https://assets.ohrats.party https://fonts.gstatic.com; img-src 'self' https://assets.ohrats.party data:; script-src 'self' https://assets.ohrats.party; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"

// Handle serves immutable WebUI assets, robots.txt, and public pages.
func Handle(value ohrats_rc_http_types.Request) witTypes.Result[witTypes.Option[ohrats_rc_http_types.Response], string] {
	if value.Method != "GET" && value.Method != "HEAD" {
		return noneResponse()
	}
	head := value.Method == "HEAD"
	if contentType, body, ok := assetFor(value.Path); ok {
		return someResponse(response(200, contentType, "public, max-age=31536000, immutable", body, head))
	}
	if value.Path == "/robots.txt" {
		return someResponse(response(200, "text/plain; charset=utf-8", "public, max-age=3600", []byte("User-agent: *\nAllow: /\n"), head))
	}
	publicURL := PublicURL(value)
	page, ok := renderPage(value.Path, PublicSignup(), publicURL)
	if !ok {
		return noneResponse()
	}
	return someResponse(response(page.status, "text/html; charset=utf-8", page.cacheControl, []byte(page.body), head))
}

func response(status uint16, contentType string, cacheControl string, body []byte, head bool) ohrats_rc_http_types.Response {
	value := ohrats_rc_http_types.Response{
		Status: status,
		Headers: []ohrats_rc_http_types.Header{
			header("content-type", contentType),
			header("cache-control", cacheControl),
			header("content-security-policy", contentSecurityPolicy),
			header("referrer-policy", "same-origin"),
			header("x-content-type-options", "nosniff"),
			header("x-frame-options", "DENY"),
		},
	}
	if head {
		value.Body = []byte{}
	} else {
		value.Body = body
	}
	return value
}

func header(name string, value string) ohrats_rc_http_types.Header {
	return ohrats_rc_http_types.Header{Name: name, Value: value}
}

func noneResponse() witTypes.Result[witTypes.Option[ohrats_rc_http_types.Response], string] {
	return witTypes.Ok[witTypes.Option[ohrats_rc_http_types.Response], string](
		witTypes.None[ohrats_rc_http_types.Response](),
	)
}

func someResponse(value ohrats_rc_http_types.Response) witTypes.Result[witTypes.Option[ohrats_rc_http_types.Response], string] {
	return witTypes.Ok[witTypes.Option[ohrats_rc_http_types.Response], string](
		witTypes.Some(value),
	)
}
