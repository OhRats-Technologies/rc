package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// turnstileEndpoint is a variable so tests can substitute a local verifier.
var turnstileEndpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

type turnstileResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

// verifyTurnstile validates a Cloudflare Turnstile response token with the
// configured secret. It returns a user-facing error suitable for the auth UI.
func (s *Server) verifyTurnstile(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("complete the human verification first")
	}
	if s.cfg.TurnstileSecretKey == "" {
		return fmt.Errorf("public signup is unavailable")
	}
	body := url.Values{
		"secret":   {s.cfg.TurnstileSecretKey},
		"response": {token},
	}.Encode()
	verifyCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(verifyCtx, http.MethodPost, turnstileEndpoint, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("human verification unavailable")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("human verification unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("human verification unavailable")
	}
	var parsed turnstileResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&parsed); err != nil {
		return fmt.Errorf("human verification unavailable")
	}
	if !parsed.Success {
		return fmt.Errorf("human verification failed")
	}
	return nil
}
