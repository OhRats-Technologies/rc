package server

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/node"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

type testControl struct {
	ID      string
	UserID  string
	private ed25519.PrivateKey
}

func seedTestControl(t *testing.T, srv *Server, userID, originURL string) *testControl {
	t.Helper()
	parsed, err := url.Parse(originURL)
	if err != nil || parsed.Host == "" {
		t.Fatal("invalid test origin")
	}
	origin := parsed.Scheme + "://" + parsed.Host
	rpID := parsed.Hostname()

	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cose, err := webauthn.ES256CoseKey(
		append([]byte(nil), ecdsaKey.PublicKey.X.FillBytes(make([]byte, 32))...),
		append([]byte(nil), ecdsaKey.PublicKey.Y.FillBytes(make([]byte, 32))...),
	)
	if err != nil {
		t.Fatal(err)
	}
	credentialID := []byte("rc-test-control-" + randomID(8))
	if _, err := srv.auth.AddPasskey(userID, "Control Test", credentialID, cose, authstore.AlgorithmES256); err != nil {
		t.Fatal(err)
	}
	controlPublic, controlPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clientID := "control_" + randomID(12)
	grant := protocol.ControlGrant{
		V: 1, ClientID: clientID, UserID: userID,
		SigningPublicKey: base64.RawURLEncoding.EncodeToString(controlPublic),
		IssuedAt:         time.Now().UnixMilli(),
		ExpiresAt:        time.Now().Add(30 * 24 * time.Hour).UnixMilli(),
	}
	grantJSON, _ := json.Marshal(grant)
	assertion := testWebAuthnAssertion(t, ecdsaKey, credentialID,
		crypto.ControlGrantChallenge(string(grantJSON)), origin, rpID)
	if err := srv.auth.PutControlClient(authstore.ControlClient{
		ID: clientID, UserID: userID, Kind: "browser",
		SigningPublicKey: grant.SigningPublicKey,
		CredentialID:     base64.RawURLEncoding.EncodeToString(credentialID),
		Grant:            string(grantJSON), Assertion: assertion,
		CreatedAt: time.Now().UTC(), ExpiresAt: time.UnixMilli(grant.ExpiresAt).UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership(defaultWorkspaceID, userID, "owner"); err != nil {
		t.Fatal(err)
	}
	return &testControl{ID: clientID, UserID: userID, private: controlPrivate}
}

func (c *testControl) sign(payload string) string {
	return base64.RawURLEncoding.EncodeToString(ed25519.Sign(c.private, []byte(payload)))
}

func seedTestMcpGrant(t *testing.T, srv *Server, control *testControl, scopes []string) string {
	t.Helper()
	return seedTestMcpGrantUntil(t, srv, control, scopes, time.Now().Add(time.Hour).UnixMilli())
}

func seedTestMcpGrantUntil(t *testing.T, srv *Server, control *testControl, scopes []string, expiresAt int64) string {
	t.Helper()
	audience := "account"
	payload := protocol.McpGrantPayload{
		V: 2, ID: "mcp_" + randomID(18), UserID: control.UserID,
		ClientID: "mcp-test-client", ClientName: "Test Client",
		Audience: &audience, Scopes: scopes,
		IssuedAt: time.Now().UnixMilli(), ExpiresAt: expiresAt,
	}
	grantJSON, _ := json.Marshal(payload)
	signaturePayload, err := mcpSignaturePayload(string(grantJSON))
	if err != nil {
		t.Fatal(err)
	}
	signature := control.sign(signaturePayload)
	client, err := srv.auth.ControlClient(control.UserID, control.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.PutMcpGrant(authstore.McpGrantRecord{
		ID: payload.ID, UserID: control.UserID, ClientID: payload.ClientID,
		Name: payload.ClientName, Grant: string(grantJSON), Signature: signature,
		ControlClientID: control.ID, CredentialID: client.CredentialID,
		ControlGrant: client.Grant, ControlAssertion: client.Assertion,
		CreatedAt: time.Now().UTC(), ExpiresAt: time.UnixMilli(payload.ExpiresAt).UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	return payload.ID
}

func testWebAuthnAssertion(
	t *testing.T, key *ecdsa.PrivateKey, credentialID []byte, challenge, origin, rpID string,
) string {
	t.Helper()
	clientData := []byte(fmt.Sprintf(
		`{"type":"webauthn.get","challenge":%q,"origin":%q,"crossOrigin":false}`,
		challenge, origin,
	))
	rpHash := sha256.Sum256([]byte(rpID))
	authData := append(append([]byte(nil), rpHash[:]...), 0x05, 0, 0, 0, 0)
	clientHash := sha256.Sum256(clientData)
	signed := append(append([]byte(nil), authData...), clientHash[:]...)
	digest := sha256.Sum256(signed)
	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	assertion, _ := json.Marshal(map[string]any{
		"id":    base64.RawURLEncoding.EncodeToString(credentialID),
		"rawId": base64.RawURLEncoding.EncodeToString(credentialID),
		"type":  "public-key",
		"response": map[string]string{
			"clientDataJSON":    base64.RawURLEncoding.EncodeToString(clientData),
			"authenticatorData": base64.RawURLEncoding.EncodeToString(authData),
			"signature":         base64.RawURLEncoding.EncodeToString(signature),
		},
	})
	return string(assertion)
}

func waitTestNodeLock(t *testing.T, srv *Server, deviceID string) {
	t.Helper()
	snapshot, err := srv.authoritySnapshot(defaultWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	hash := node.SnapshotHash(snapshot)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		device, ok := srv.registry.Get(deviceID)
		if ok && device.LockHash == hash {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Node %s did not acknowledge RC Lock %s", deviceID, hash)
}
