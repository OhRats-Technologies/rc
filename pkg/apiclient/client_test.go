package apiclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
)

func TestApiClientPoPRequestVerification(t *testing.T) {
	seed := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" // 32 bytes of zeros
	cred, err := CredentialFromSigningSeed("key_123", seed)
	if err != nil {
		t.Fatalf("CredentialFromSigningSeed failed: %v", err)
	}

	var verified bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		keyID := r.Header.Get("x-rc-key-id")
		timestamp := r.Header.Get("x-rc-timestamp")
		nonce := r.Header.Get("x-rc-nonce")
		sig := r.Header.Get("x-rc-signature")

		req := crypto.ApiRequest{
			KeyID:      keyID,
			Timestamp:  timestamp,
			Nonce:      nonce,
			Method:     r.Method,
			RequestURI: r.URL.RequestURI(),
			Body:       body,
		}

		pubKey := cred.Pop.PublicKeyString()
		if err := crypto.VerifyApi(pubKey, sig, req); err != nil {
			t.Errorf("VerifyApi on server failed: %v", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		verified = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"devices":[{"id":"dev_1","name":"TestNode"}]}`))
	}))
	defer server.Close()

	client, err := New(server.URL, cred)
	if err != nil {
		t.Fatalf("New ApiClient failed: %v", err)
	}

	devices, err := client.Devices(context.Background())
	if err != nil {
		t.Fatalf("client.Devices failed: %v", err)
	}

	if !verified {
		t.Fatal("server did not verify request headers")
	}
	if len(devices) != 1 || devices[0].Name != "TestNode" {
		t.Fatalf("unexpected devices: %+v", devices)
	}
}
