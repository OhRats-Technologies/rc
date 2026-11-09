package integration

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"testing"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/mesh"
)

func TestKeyCustodyAndMeshPolicyDomains(t *testing.T) {
	seed := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	public := base64.RawURLEncoding.EncodeToString(private.Public().(ed25519.PublicKey))
	signature, err := crypto.SignEd25519Seed(seed, []byte("rc-domain-payload"))
	if err != nil {
		t.Fatal(err)
	}
	if err := crypto.VerifyEd25519(public, []byte("rc-domain-payload"), signature); err != nil {
		t.Fatalf("key custody signature = %v", err)
	}
	clientPrivate := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	nodeStaticPrivate := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	nodeEphemeralPrivate := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	clientPublic, err := crypto.X25519Public(clientPrivate)
	if err != nil {
		t.Fatal(err)
	}
	nodeStaticPublic, err := crypto.X25519Public(nodeStaticPrivate)
	if err != nil {
		t.Fatal(err)
	}
	nodeEphemeralPublic, err := crypto.X25519Public(nodeEphemeralPrivate)
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := crypto.DeriveClientKey(clientPrivate, nodeStaticPublic, nodeEphemeralPublic,
		"domain-challenge", "dev_domain", "client_domain")
	if err != nil {
		t.Fatal(err)
	}
	nodeKey, err := crypto.DeriveNodeKey(nodeStaticPrivate, nodeEphemeralPrivate, clientPublic,
		"domain-challenge", "dev_domain", "client_domain")
	if err != nil {
		t.Fatal(err)
	}
	if clientKey != nodeKey {
		t.Fatal("client and Node key agreement diverged")
	}
	ciphertext, err := crypto.EncryptFrame(&clientKey, 1, 1, "session_domain", "c2n", []byte("domain-frame"))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := crypto.DecryptFrame(&nodeKey, 1, 1, "session_domain", "c2n", ciphertext)
	if err != nil || string(plaintext) != "domain-frame" {
		t.Fatalf("key domain frame = %q, %v", plaintext, err)
	}

	realm, err := mesh.NewRealmID("realm-domain")
	if err != nil {
		t.Fatal(err)
	}
	source, err := mesh.PeerIDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := mesh.NewPeerID("peer-destination")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	envelope, err := mesh.SignEnvelope(mesh.EnvelopeHeader{
		V: mesh.EnvelopeVersion, RealmID: realm, MessageID: "message-domain",
		Source: source, Destination: destination, IssuedAt: now, ExpiresAt: now + 60_000, MaxHops: 4,
	}, []byte("mesh-domain-payload"), seed)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := envelope.Verify(public, now)
	if err != nil || string(decoded) != "mesh-domain-payload" {
		t.Fatalf("mesh envelope = %q, %v", decoded, err)
	}
	replay := mesh.NewReplayGuard()
	if !replay.Accept(envelope, now) {
		t.Fatal("fresh mesh envelope was rejected")
	}
	if replay.Accept(envelope, now) {
		t.Fatal("replayed mesh envelope was accepted")
	}
	policy := mesh.DefaultMeshPolicy()
	if !policy.Enabled || !policy.AuthenticatedRelay || policy.PublicRelay || policy.MaximumHops != 8 {
		t.Fatalf("mesh policy = %+v", policy)
	}
	if _, err := mesh.ValidateIdentifier("bad identifier"); err == nil {
		t.Fatal("mesh identifier accepted whitespace")
	}
}
