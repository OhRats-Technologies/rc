package identityhttp

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode"

	"wit_component/ohrats_rc_http_types"
	"wit_component/ohrats_rc_identity_credentials"
	"wit_component/ohrats_rc_plugin_host"
	"wit_component/ohrats_rc_plugin_types"
	"wit_component/ohrats_rc_session_management"
)

type nameInput struct {
	Name *string `json:"name"`
}

type verifyInput struct {
	CeremonyID *string         `json:"ceremonyId"`
	Response   json.RawMessage `json:"response"`
}

type loginVerifyInput struct {
	CeremonyID *string         `json:"ceremonyId"`
	Response   json.RawMessage `json:"response"`
	Lifetime   *string         `json:"lifetime"`
}

func setupOptions(request ohrats_rc_http_types.Request) (ohrats_rc_http_types.Response, error) {
	count, err := userCount()
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	if count > 0 {
		return errorResponse(409, "setup already completed")
	}
	authorized, err := setupAuthorized(cookieHeader(request))
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	if !authorized {
		return errorResponse(403, "Open the RC setup link first.")
	}
	input, err := jsonBody[nameInput](request)
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	if input.Name == nil {
		return ohrats_rc_http_types.Response{}, errors.New("invalid request")
	}
	name, err := cleanName(*input.Name)
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	userID, err := randomUserID()
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	relyingParty, err := RelyingPartyFromRequest(request)
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	ceremonyID, options, err := beginRegistration("setup", userID, name, relyingParty)
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	return jsonResponse(200, map[string]any{"ceremonyId": ceremonyID, "options": options})
}

func setupVerify(request ohrats_rc_http_types.Request) (ohrats_rc_http_types.Response, error) {
	count, err := userCount()
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	if count > 0 {
		return errorResponse(409, "setup already completed")
	}
	input, err := jsonBody[verifyInput](request)
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	if input.CeremonyID == nil || len(input.Response) == 0 {
		return ohrats_rc_http_types.Response{}, errors.New("invalid request")
	}
	registration, err := finishRegistration("setup", *input.CeremonyID, input.Response)
	if err != nil {
		logAuthFailure("registration", err)
		return errorResponse(401, "passkey verification failed")
	}
	created := ohrats_rc_identity_credentials.CreateUser(
		registration.UserID,
		registration.DisplayName,
		"Passkey",
		registration.Credential,
	)
	if created.IsErr() {
		return ohrats_rc_http_types.Response{}, errors.New(created.Err())
	}
	return issueSession(request, created.Ok().Id, "30d", 201)
}

func loginOptions(request ohrats_rc_http_types.Request) (ohrats_rc_http_types.Response, error) {
	relyingParty, err := RelyingPartyFromRequest(request)
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	ceremonyID, options, err := beginLogin(relyingParty)
	if err != nil {
		return errorResponse(401, err.Error())
	}
	return jsonResponse(200, map[string]any{"ceremonyId": ceremonyID, "options": options})
}

func loginVerify(request ohrats_rc_http_types.Request) (ohrats_rc_http_types.Response, error) {
	input, err := jsonBody[loginVerifyInput](request)
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	if input.CeremonyID == nil || len(input.Response) == 0 {
		return ohrats_rc_http_types.Response{}, errors.New("invalid request")
	}
	userID, err := finishLogin(*input.CeremonyID, input.Response)
	if err != nil {
		logAuthFailure("authentication", err)
		return errorResponse(401, "passkey verification failed")
	}
	lifetime := "30d"
	if input.Lifetime != nil {
		lifetime = *input.Lifetime
	}
	return issueSession(request, userID, lifetime, 200)
}

func logout(request ohrats_rc_http_types.Request) (ohrats_rc_http_types.Response, error) {
	revoked := ohrats_rc_session_management.Revoke(cookieHeader(request))
	if revoked.IsErr() {
		return ohrats_rc_http_types.Response{}, errors.New(revoked.Err())
	}
	relyingParty, err := RelyingPartyFromRequest(request)
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	return withCookie(redirectResponse("/"), clearSessionCookie(relyingParty.Secure)), nil
}

func issueSession(
	request ohrats_rc_http_types.Request,
	userID string,
	lifetime string,
	status uint16,
) (ohrats_rc_http_types.Response, error) {
	seconds, err := lifetimeSeconds(lifetime)
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	expiresAtMs := saturatingAdd(nowMs(), seconds*1000)
	issued := ohrats_rc_session_management.Issue(userID, expiresAtMs)
	if issued.IsErr() {
		return ohrats_rc_http_types.Response{}, errors.New(issued.Err())
	}
	relyingParty, err := RelyingPartyFromRequest(request)
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	value, err := jsonResponse(status, map[string]any{"ok": true, "expiresAt": expiresAtMs})
	if err != nil {
		return ohrats_rc_http_types.Response{}, err
	}
	return withCookie(value, sessionCookie(issued.Ok().Token, seconds, relyingParty.Secure)), nil
}

func cleanName(value string) (string, error) {
	trimmed := []rune(strings.TrimSpace(value))
	if len(trimmed) > 120 {
		trimmed = trimmed[:120]
	}
	result := string(trimmed)
	if result == "" || strings.IndexFunc(result, unicode.IsControl) >= 0 {
		return "", errors.New("name required")
	}
	return result, nil
}

func lifetimeSeconds(value string) (uint64, error) {
	switch value {
	case "1h":
		return 60 * 60, nil
	case "1d":
		return 24 * 60 * 60, nil
	case "7d":
		return 7 * 24 * 60 * 60, nil
	case "30d":
		return 30 * 24 * 60 * 60, nil
	case "90d":
		return 90 * 24 * 60 * 60, nil
	case "180d":
		return 180 * 24 * 60 * 60, nil
	case "1y":
		return 365 * 24 * 60 * 60, nil
	default:
		return 0, errors.New("invalid authorization lifetime")
	}
}

func logAuthFailure(kind string, err error) {
	message := []rune(err.Error())
	if len(message) > 240 {
		message = message[:240]
	}
	ohrats_rc_plugin_host.Log(
		ohrats_rc_plugin_types.LogLevel_Warn,
		"WebAuthn "+kind+" failed: "+string(message),
	)
}
