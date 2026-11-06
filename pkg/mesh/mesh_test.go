package mesh

import (
	"crypto/ed25519"
	"reflect"
	"testing"
	"time"
)

func TestNegotiateCapabilities(t *testing.T) {
	local, err := NewCapabilityAdvertisement(
		"rc.transport.quic",
		[]uint32{1, 2, 3},
		[]string{"datagram", "ipv6", "relay"},
	)
	if err != nil {
		t.Fatalf("NewCapabilityAdvertisement failed: %v", err)
	}

	req, err := NewCapabilityRequirement(
		local.ID,
		local.Versions,
		local.Features,
		[]string{"datagram"},
	)
	if err != nil {
		t.Fatalf("NewCapabilityRequirement failed: %v", err)
	}

	remote, err := NewCapabilityAdvertisement(
		"rc.transport.quic",
		[]uint32{1, 2},
		[]string{"datagram", "relay"},
	)
	if err != nil {
		t.Fatalf("NewCapabilityAdvertisement remote failed: %v", err)
	}

	neg := req.Negotiate(remote)
	if neg == nil {
		t.Fatal("expected successful negotiation")
	}
	if neg.Version != 2 {
		t.Fatalf("expected negotiated version 2, got %d", neg.Version)
	}
	expectedFeatures := []string{"datagram", "relay"}
	if !reflect.DeepEqual(neg.Features, expectedFeatures) {
		t.Fatalf("expected features %+v, got %+v", expectedFeatures, neg.Features)
	}
}

func TestEnvelopeSignVerifyAndForward(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	pubStr := b64URL.EncodeToString(pub)
	seedStr := b64URL.EncodeToString(priv.Seed())

	sourceID, err := PeerIDFromPublicKey(pubStr)
	if err != nil {
		t.Fatalf("PeerIDFromPublicKey failed: %v", err)
	}

	destID := PeerID("dest-peer-001")
	now := time.Now().UnixMilli()

	header := EnvelopeHeader{
		V:           EnvelopeVersion,
		RealmID:     "realm-test",
		MessageID:   "msg-001",
		Source:      sourceID,
		Destination: destID,
		IssuedAt:    now,
		ExpiresAt:   now + 60000,
		MaxHops:     4,
	}

	env, err := SignEnvelope(header, []byte("secret payload"), seedStr)
	if err != nil {
		t.Fatalf("SignEnvelope failed: %v", err)
	}

	plaintext, err := env.Verify(pubStr, now)
	if err != nil || string(plaintext) != "secret payload" {
		t.Fatalf("Verify failed: plaintext=%s, err=%v", string(plaintext), err)
	}

	forwarded, err := env.Forward("relay-1")
	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}
	if len(forwarded.Route) != 1 || forwarded.Route[0] != "relay-1" {
		t.Fatalf("unexpected route after forward: %+v", forwarded.Route)
	}
}

func TestTopologyRouting(t *testing.T) {
	realm := RealmID("test-realm")
	local := PeerID("node-A")
	pubA, privA, _ := ed25519.GenerateKey(nil)
	pubB, privB, _ := ed25519.GenerateKey(nil)
	pubC, privC, _ := ed25519.GenerateKey(nil)

	local, _ = PeerIDFromPublicKey(b64URL.EncodeToString(pubA))
	nodeB, _ := PeerIDFromPublicKey(b64URL.EncodeToString(pubB))
	nodeC, _ := PeerIDFromPublicKey(b64URL.EncodeToString(pubC))

	topo := NewTopology(realm, local)
	topo.TrustPeer(local, b64URL.EncodeToString(pubA))
	topo.TrustPeer(nodeB, b64URL.EncodeToString(pubB))
	topo.TrustPeer(nodeC, b64URL.EncodeToString(pubC))

	now := time.Now().UnixMilli()

	// Adv A: neighbor B (cost 10)
	advA := LinkAdvertisement{
		V:         AdvertisementVersion,
		RealmID:   realm,
		Origin:    local,
		Sequence:  1,
		IssuedAt:  now,
		ExpiresAt: now + 60000,
		Neighbors: []Neighbor{
			{PeerID: nodeB, Cost: 10},
		},
	}
	sAdvA, _ := SignAdvertisement(advA, b64URL.EncodeToString(privA.Seed()))
	_, _ = topo.Insert(*sAdvA, now)

	// Adv B: neighbor A (cost 10), neighbor C (cost 5)
	advB := LinkAdvertisement{
		V:         AdvertisementVersion,
		RealmID:   realm,
		Origin:    nodeB,
		Sequence:  1,
		IssuedAt:  now,
		ExpiresAt: now + 60000,
		Neighbors: []Neighbor{
			{PeerID: local, Cost: 10},
			{PeerID: nodeC, Cost: 5},
		},
	}
	sAdvB, _ := SignAdvertisement(advB, b64URL.EncodeToString(privB.Seed()))
	_, _ = topo.Insert(*sAdvB, now)

	// Adv C: neighbor B (cost 5)
	advC := LinkAdvertisement{
		V:         AdvertisementVersion,
		RealmID:   realm,
		Origin:    nodeC,
		Sequence:  1,
		IssuedAt:  now,
		ExpiresAt: now + 60000,
		Neighbors: []Neighbor{
			{PeerID: nodeB, Cost: 5},
		},
	}
	sAdvC, _ := SignAdvertisement(advC, b64URL.EncodeToString(privC.Seed()))
	_, _ = topo.Insert(*sAdvC, now)

	// Route from A to C: should go through B, total cost 10 + 5 = 15
	route, ok := topo.Route(nodeC, now)
	if !ok {
		t.Fatal("expected route from A to C")
	}
	if route.NextHop != nodeB || route.TotalCost != 15 {
		t.Fatalf("unexpected route to C: %+v", route)
	}
}
