package apiclient

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/websocket"
)

// ControlChallenge asks the selected Node for a single-use control challenge.
func (c *ApiClient) ControlChallenge(ctx context.Context, deviceID string) (string, error) {
	var response ControlChallenge
	if err := c.Post(ctx, "/api/v1/control/challenge", map[string]string{"deviceId": deviceID}, &response); err != nil {
		return "", err
	}
	if response.Challenge == "" {
		return "", &ApiError{Message: "Node returned an empty control challenge"}
	}
	return response.Challenge, nil
}

// ControlOpen completes the authenticated Node/control handshake.
func (c *ApiClient) ControlOpen(ctx context.Context, input ControlOpen) (*ControlReady, error) {
	var response ControlReady
	if err := c.Post(ctx, "/api/v1/control/open", input, &response); err != nil {
		return nil, err
	}
	if response.SessionID == "" || response.TransportPublicKey == "" ||
		response.EphemeralPublicKey == "" || response.Signature == "" {
		return nil, &ApiError{Message: "invalid control ready response"}
	}
	return &response, nil
}

// ControlPipe dials the authenticated WebSocket that relays encrypted control
// frames for one session. The caller owns the returned connection.
func (c *ApiClient) ControlPipe(ctx context.Context, sessionID string) (*websocket.Conn, error) {
	path := "/api/v1/control/pipe/" + url.PathEscape(sessionID)
	target := *c.base
	switch target.Scheme {
	case "https":
		target.Scheme = "wss"
	case "http":
		target.Scheme = "ws"
	default:
		return nil, &ApiError{Message: "invalid RC server URL"}
	}
	target.Path = path
	target.RawQuery = ""

	headers := http.Header{}
	if c.credential.Kind == CredentialPop && c.credential.Pop != nil {
		for key, value := range c.credential.Pop.Headers(http.MethodGet, path, nil) {
			headers.Set(key, value)
		}
	}
	origin := &url.URL{Scheme: "http", Host: target.Host}
	if target.Scheme == "wss" {
		origin.Scheme = "https"
	}
	conn, err := websocket.DialConfig(&websocket.Config{
		Location: &target,
		Origin:   origin,
		Version:  websocket.ProtocolVersionHybi13,
		Header:   headers,
		Dialer:   &net.Dialer{Timeout: 15 * time.Second},
	})
	if err != nil {
		return nil, &ApiError{Message: "control pipe unavailable", Err: err}
	}
	return conn, nil
}

// CloseControl releases one authenticated control session.
func (c *ApiClient) CloseControl(ctx context.Context, sessionID string) error {
	path := "/api/v1/control/" + url.PathEscape(sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.base.ResolveReference(&url.URL{Path: path}).String(), nil)
	if err != nil {
		return &ApiError{Err: err}
	}
	if c.credential.Kind == CredentialPop && c.credential.Pop != nil {
		for key, value := range c.credential.Pop.Headers(http.MethodDelete, path, nil) {
			req.Header.Set(key, value)
		}
	}
	res, err := c.http.Do(req)
	if err != nil {
		return &ApiError{Err: err}
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &ApiError{Status: res.StatusCode, Message: fmt.Sprintf("control close failed (%d)", res.StatusCode)}
	}
	return nil
}
