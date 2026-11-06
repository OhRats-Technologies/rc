package meshpolicy

import (
	"testing"

	"wit_component/ohrats_rc_mesh_types"
)

func TestNegotiateChoosesHighestCommonVersionAndIntersection(t *testing.T) {
	result := Negotiate(
		ohrats_rc_mesh_types.CapabilityRequirement{
			Id:                "rc.transport.quic",
			Versions:          []uint32{1, 2, 3},
			SupportedFeatures: []string{"datagram", "relay"},
			RequiredFeatures:  []string{"datagram"},
		},
		ohrats_rc_mesh_types.CapabilityAdvertisement{
			Id:       "rc.transport.quic",
			Versions: []uint32{1, 2},
			Features: []string{"datagram"},
		},
	)
	if result.IsErr() {
		t.Fatalf("negotiate rejected valid capabilities: %d", result.Err().Tag())
	}
	negotiated := result.Ok().Some()
	if negotiated.Version != 2 {
		t.Fatalf("version = %d, want 2", negotiated.Version)
	}
	if len(negotiated.Features) != 1 || negotiated.Features[0] != "datagram" {
		t.Fatalf("features = %v, want [datagram]", negotiated.Features)
	}
}

func TestNegotiateRejectsNoncanonicalVersions(t *testing.T) {
	result := Negotiate(
		ohrats_rc_mesh_types.CapabilityRequirement{
			Id:                "rc.transport.quic",
			Versions:          []uint32{2, 1},
			SupportedFeatures: []string{"datagram"},
			RequiredFeatures:  []string{},
		},
		ohrats_rc_mesh_types.CapabilityAdvertisement{
			Id:       "rc.transport.quic",
			Versions: []uint32{1, 2},
			Features: []string{"datagram"},
		},
	)
	if !result.IsErr() || result.Err().Tag() != ohrats_rc_mesh_types.Rejection_InvalidCapability {
		t.Fatalf("noncanonical versions were accepted")
	}
}

func TestRoutesChooseShortestTrustedPath(t *testing.T) {
	advertisements := []ohrats_rc_mesh_types.LinkAdvertisement{
		testAdvertisement("peer-a", []ohrats_rc_mesh_types.Neighbor{{PeerId: "peer-b", Cost: 2}}),
		testAdvertisement("peer-b", []ohrats_rc_mesh_types.Neighbor{{PeerId: "peer-c", Cost: 3}}),
	}
	result := Routes("realm-a", "peer-a", []string{"peer-b", "peer-c"}, advertisements, 1_000)
	if result.IsErr() {
		t.Fatalf("routes rejected valid advertisements: %d", result.Err().Tag())
	}
	for _, route := range result.Ok() {
		if route.DestinationPeerId != "peer-c" {
			continue
		}
		if route.NextHopPeerId != "peer-b" || route.TotalCost != 5 {
			t.Fatalf("route = %+v, want next hop peer-b at cost 5", route)
		}
		return
	}
	t.Fatal("route to peer-c was not planned")
}

func TestServiceChoosesCheapestProvider(t *testing.T) {
	advertisements := []ohrats_rc_mesh_types.LinkAdvertisement{
		testAdvertisement("peer-a", []ohrats_rc_mesh_types.Neighbor{{PeerId: "peer-b", Cost: 2}}),
		testAdvertisement("peer-b", []ohrats_rc_mesh_types.Neighbor{{PeerId: "peer-c", Cost: 3}}),
		{
			Version:      1,
			RealmId:      "realm-a",
			OriginPeerId: "peer-c",
			Sequence:     1,
			IssuedAtMs:   500,
			ExpiresAtMs:  5_000,
			Services:     []ohrats_rc_mesh_types.ServiceAdvertisement{{Name: "artifact-cache", Cost: 7}},
		},
	}
	result := Service("realm-a", "peer-a", []string{"peer-b", "peer-c"}, advertisements, "artifact-cache", 1_000)
	if result.IsErr() {
		t.Fatalf("service rejected valid advertisements: %d", result.Err().Tag())
	}
	route := result.Ok().Some()
	if route.ProviderPeerId != "peer-c" || route.TotalCost != 12 {
		t.Fatalf("service route = %+v, want peer-c at cost 12", route)
	}
}

func TestEnvelopeRejectsRelayLoopsAndExpiry(t *testing.T) {
	value := testEnvelope()
	if result := Validate(value, 1_000, 8); result.IsErr() {
		t.Fatalf("valid envelope rejected: %d", result.Err().Tag())
	}
	forwarded := Forward(value, "peer-b")
	if forwarded.IsErr() {
		t.Fatalf("valid relay rejected: %d", forwarded.Err().Tag())
	}
	if len(forwarded.Ok().Route) != 1 || forwarded.Ok().Route[0] != "peer-b" {
		t.Fatalf("route = %v, want [peer-b]", forwarded.Ok().Route)
	}
	loop := Forward(forwarded.Ok(), "peer-b")
	if !loop.IsErr() || loop.Err().Tag() != ohrats_rc_mesh_types.Rejection_RouteRejected {
		t.Fatal("relay loop was accepted")
	}
	expired := value
	expired.ExpiresAtMs = 999
	if result := Validate(expired, 1_000, 8); !result.IsErr() || result.Err().Tag() != ohrats_rc_mesh_types.Rejection_Expired {
		t.Fatal("expired envelope was accepted")
	}
}

func testAdvertisement(origin string, neighbors []ohrats_rc_mesh_types.Neighbor) ohrats_rc_mesh_types.LinkAdvertisement {
	return ohrats_rc_mesh_types.LinkAdvertisement{
		Version:      1,
		RealmId:      "realm-a",
		OriginPeerId: origin,
		Sequence:     1,
		IssuedAtMs:   500,
		ExpiresAtMs:  5_000,
		Neighbors:    neighbors,
	}
}

func testEnvelope() ohrats_rc_mesh_types.Envelope {
	return ohrats_rc_mesh_types.Envelope{
		Version:           1,
		RealmId:           "realm-a",
		MessageId:         "message-a",
		SourcePeerId:      "peer-a",
		DestinationPeerId: "peer-c",
		IssuedAtMs:        500,
		ExpiresAtMs:       5_000,
		MaxHops:           4,
		Ciphertext:        []uint8("opaque-ciphertext"),
	}
}
