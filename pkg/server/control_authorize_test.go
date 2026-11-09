package server

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
)

func TestControlAuthorizationEndpoints(t *testing.T) {
	srv, ts := newAuthServer(t)
	user, err := srv.auth.CreateUser("Control User")
	if err != nil {
		t.Fatal(err)
	}
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cose, err := webauthn.ES256CoseKey(
		ecdsaKey.PublicKey.X.FillBytes(make([]byte, 32)),
		ecdsaKey.PublicKey.Y.FillBytes(make([]byte, 32)),
	)
	if err != nil {
		t.Fatal(err)
	}
	credentialID := []byte("control-endpoint-" + randomID(8))
	if _, err := srv.auth.AddPasskey(user.ID, "Control", credentialID, cose, authstore.AlgorithmES256); err != nil {
		t.Fatal(err)
	}
	session, _, err := srv.auth.IssueSession(user.ID, "1h")
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookieName, Value: session}
	controlPublic, controlPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clientID := "control_" + randomID(12)
	optionsBody, _ := json.Marshal(map[string]any{
		"clientId": clientID, "signingPublicKey": base64.RawURLEncoding.EncodeToString(controlPublic),
		"lifetime": "30d", "kind": "browser",
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/control/authorize/options", bytes.NewReader(optionsBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res, payload := oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("control options = %d: %s", res.StatusCode, payload)
	}
	var start struct {
		AuthorizationID string         `json:"authorizationId"`
		Options         map[string]any `json:"options"`
	}
	if err := json.Unmarshal(payload, &start); err != nil {
		t.Fatal(err)
	}
	challenge, _ := start.Options["challenge"].(string)
	origin, _ := url.Parse("https://rc.example.test")
	assertion := testWebAuthnAssertion(t, ecdsaKey, credentialID, challenge,
		origin.String(), origin.Hostname())
	verifyBody, _ := json.Marshal(map[string]any{
		"authorizationId": start.AuthorizationID, "response": json.RawMessage(assertion),
	})
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/control/authorize/verify", bytes.NewReader(verifyBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res, payload = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("control verify = %d: %s", res.StatusCode, payload)
	}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/control/clients/"+clientID, nil)
	req.AddCookie(cookie)
	res, payload = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte(`"authorized":true`)) {
		t.Fatalf("control status = %d: %s", res.StatusCode, payload)
	}
	client, err := srv.auth.ControlClient(user.ID, clientID)
	if err != nil {
		t.Fatalf("stored control client: %v", err)
	}
	if client.SigningPublicKey != base64.RawURLEncoding.EncodeToString(controlPublic) {
		t.Fatal("stored control signing key mismatch")
	}
	_ = controlPrivate
}
