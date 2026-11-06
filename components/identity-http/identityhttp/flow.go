package identityhttp

import (
	"crypto/sha256"
	"errors"
	"strings"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/assets"
	"wit_component/ohrats_rc_http_types"
	"wit_component/ohrats_rc_identity_users"
	"wit_component/ohrats_rc_session_lookup"
)

var authScriptPath = buildAuthScriptPath()

func buildAuthScriptPath() string {
	digest := sha256.Sum256(assets.AuthJS)
	return "/assets/identity-auth." + hexString(digest[:])[:16] + ".js"
}

// Handle routes identity page, API, and browser-asset requests.
func Handle(request ohrats_rc_http_types.Request) witTypes.Result[witTypes.Option[ohrats_rc_http_types.Response], string] {
	if request.Path == authScriptPath && (request.Method == "GET" || request.Method == "HEAD") {
		response := javascriptResponse(assets.AuthJS)
		if request.Method == "HEAD" {
			response.Body = []byte{}
		}
		return someResponse(response)
	}
	response, found, err := route(request)
	if err != nil {
		return witTypes.Err[witTypes.Option[ohrats_rc_http_types.Response], string](err.Error())
	}
	if !found {
		return noneResponse()
	}
	if request.Method == "HEAD" {
		response.Body = []byte{}
	}
	return someResponse(response)
}

func route(request ohrats_rc_http_types.Request) (ohrats_rc_http_types.Response, bool, error) {
	method := request.Method
	path := request.Path
	switch {
	case (method == "GET" || method == "HEAD") && path == "/":
		return home(request)
	case (method == "GET" || method == "HEAD") && path == "/login":
		response, err := loginRoute(request)
		return response, true, err
	case (method == "GET" || method == "HEAD") && strings.HasPrefix(path, "/setup/"):
		response, err := setupLink(request, path[7:])
		return response, true, err
	case method == "GET" && path == "/api/v1/status":
		response, err := status(request)
		return response, true, err
	case method == "POST" && path == "/api/v1/auth/setup/options":
		response, err := setupOptions(request)
		return response, true, err
	case method == "POST" && path == "/api/v1/auth/setup/verify":
		response, err := setupVerify(request)
		return response, true, err
	case method == "POST" && path == "/api/v1/auth/login/options":
		response, err := loginOptions(request)
		return response, true, err
	case method == "POST" && path == "/api/v1/auth/login/verify":
		response, err := loginVerify(request)
		return response, true, err
	case method == "POST" && path == "/account/logout":
		response, err := logout(request)
		return response, true, err
	default:
		return ohrats_rc_http_types.Response{}, false, nil
	}
}

func home(request ohrats_rc_http_types.Request) (ohrats_rc_http_types.Response, bool, error) {
	count, err := userCount()
	if err != nil {
		return ohrats_rc_http_types.Response{}, false, err
	}
	if count == 0 {
		authorized, err := setupAuthorized(cookieHeader(request))
		if err != nil {
			return ohrats_rc_http_types.Response{}, false, err
		}
		return htmlResponse(200, renderSetupPage(authorized)), true, nil
	}
	session, err := findSession(cookieHeader(request))
	if err != nil {
		return ohrats_rc_http_types.Response{}, false, err
	}
	if session.IsSome() {
		return redirectResponse("/devices"), true, nil
	}
	return ohrats_rc_http_types.Response{}, false, nil
}

func loginRoute(request ohrats_rc_http_types.Request) (ohrats_rc_http_types.Response, error) {
	count, err := userCount()
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	if count == 0 {
		return redirectResponse("/"), nil
	}
	session, err := findSession(cookieHeader(request))
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	if session.IsSome() {
		return redirectResponse("/devices"), nil
	}
	next := "/devices"
	if value, ok := queryParameters(request)["next"]; ok && safeNext(value) {
		next = value
	}
	return htmlResponse(200, renderLoginPage(next)), nil
}

func setupLink(request ohrats_rc_http_types.Request, token string) (ohrats_rc_http_types.Response, error) {
	count, err := userCount()
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	if count > 0 {
		return redirectResponse("/"), nil
	}
	relyingParty, err := RelyingPartyFromRequest(request)
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	cookie, present, err := setupCookie(token, relyingParty.Secure)
	if err != nil {
		return errorResponse(403, "invalid setup link")
	}
	if !present {
		return redirectResponse("/"), nil
	}
	return withCookie(redirectResponse("/"), cookie), nil
}

func status(request ohrats_rc_http_types.Request) (ohrats_rc_http_types.Response, error) {
	count, err := userCount()
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	required := count == 0
	authorized := false
	if required {
		authorized, err = setupAuthorized(cookieHeader(request))
		if err != nil {
			return ohrats_rc_http_types.Response{}, err
		}
	}
	return jsonResponse(200, map[string]any{
		"setupRequired":   required,
		"setupAuthorized": authorized,
		"publicSignup":    false,
		"version":         "0.1.0",
	})
}

func userCount() (uint64, error) {
	result := ohrats_rc_identity_users.Count()
	if result.IsErr() {
		return 0, errors.New(result.Err())
	}
	return result.Ok(), nil
}

func findSession(cookie string) (witTypes.Option[ohrats_rc_session_lookup.Session], error) {
	result := ohrats_rc_session_lookup.Find(cookie)
	if result.IsErr() {
		return witTypes.None[ohrats_rc_session_lookup.Session](), errors.New(result.Err())
	}
	return result.Ok(), nil
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
