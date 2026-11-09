package apiclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
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

// ControlWebRTC exchanges one browser/CLI offer for the Node answer.
func (c *ApiClient) ControlWebRTC(
	ctx context.Context, sessionID, deviceID, sdp string, mode protocol.ControlIceMode,
) (string, error) {
	var response WebRtcAnswer
	path := "/api/v1/control/" + url.PathEscape(sessionID) + "/webrtc"
	if err := c.Post(ctx, path, map[string]any{
		"deviceId": deviceID, "sdp": sdp, "mode": mode,
	}, &response); err != nil {
		return "", err
	}
	if response.SDP == "" {
		return "", &ApiError{Message: "Node returned an empty WebRTC answer"}
	}
	return response.SDP, nil
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
