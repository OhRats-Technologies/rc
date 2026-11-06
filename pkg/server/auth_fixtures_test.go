package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
)

type webauthnFixture struct {
	Origin       string `json:"origin"`
	RPID         string `json:"rpId"`
	Expected     string `json:"expectedUserHandle"`
	Registration struct {
		Challenge         string `json:"challenge"`
		ClientDataJSON    string `json:"clientDataJSON"`
		AttestationObject string `json:"attestationObject"`
		CredentialID      string `json:"credentialId"`
	} `json:"registration"`
	Authentication struct {
		Challenge         string `json:"challenge"`
		ClientDataJSON    string `json:"clientDataJSON"`
		AuthenticatorData string `json:"authenticatorData"`
		Signature         string `json:"signature"`
		UserHandle        string `json:"userHandle"`
		CredentialID      string `json:"credentialId"`
	} `json:"authentication"`
}

func loadFixture(t *testing.T) (webauthnFixture, webauthn.RelyingParty) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "webauthn-es256.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture webauthnFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	rp, err := webauthn.NewRelyingParty(fixture.RPID, fixture.Origin)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, rp
}

func registrationJSON(t *testing.T, fixture webauthnFixture) ([]byte, webauthn.Credential) {
	t.Helper()
	response := map[string]any{
		"id":    fixture.Registration.CredentialID,
		"rawId": fixture.Registration.CredentialID,
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    fixture.Registration.ClientDataJSON,
			"attestationObject": fixture.Registration.AttestationObject,
		},
	}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := base64.RawURLEncoding.DecodeString(fixture.Registration.Challenge)
	if err != nil {
		t.Fatal(err)
	}
	_, rp := loadFixture(t)
	registration, err := webauthn.VerifyRegistration(raw, challenge, rp)
	if err != nil {
		t.Fatal(err)
	}
	return raw, registration.Credential
}

func authenticationJSON(t *testing.T, fixture webauthnFixture) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"id":    fixture.Authentication.CredentialID,
		"rawId": fixture.Authentication.CredentialID,
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    fixture.Authentication.ClientDataJSON,
			"authenticatorData": fixture.Authentication.AuthenticatorData,
			"signature":         fixture.Authentication.Signature,
			"userHandle":        fixture.Authentication.UserHandle,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func newAuthServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	srv := New(Config{Addr: ":0", DataDir: t.TempDir(), PublicURL: "https://rc.example.test"})
	ts := httptest.NewServer(srv.httpSrv.Handler)
	t.Cleanup(func() { ts.Close(); _ = srv.Close() })
	return srv, ts
}

func postJSONAuth(t *testing.T, url string, body []byte, cookie *http.Cookie, header [2]string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if header[0] != "" {
		req.Header.Set(header[0], header[1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var payload []byte
	payload, _ = json.Marshal(nil)
	buffer := new(bytes.Buffer)
	_, _ = buffer.ReadFrom(res.Body)
	if buffer.Len() > 0 {
		payload = buffer.Bytes()
	}
	return res, payload
}

func sessionCookie(t *testing.T, res *http.Response) *http.Cookie {
	t.Helper()
	for _, cookie := range res.Cookies() {
		if cookie.Name == sessionCookieName {
			return cookie
		}
	}
	t.Fatal("session cookie missing")
	return nil
}
