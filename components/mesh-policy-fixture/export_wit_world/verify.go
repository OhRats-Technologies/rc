package export_wit_world

import (
	"errors"
	"fmt"
	"slices"

	"wit_component/ohrats_rc_mesh_capabilities"
	"wit_component/ohrats_rc_mesh_envelopes"
	"wit_component/ohrats_rc_mesh_topology"
	"wit_component/ohrats_rc_mesh_types"
)

func verify() error {
	if err := capabilityContract(); err != nil {
		return err
	}
	if err := topologyContract(); err != nil {
		return err
	}
	return envelopeContract()
}

func capabilityContract() error {
	negotiated := ohrats_rc_mesh_capabilities.Negotiate(
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
	if negotiated.IsErr() {
		return rejectionError(negotiated.Err())
	}
	if negotiated.Ok().IsNone() {
		return errors.New("compatible capability was not negotiated")
	}
	value := negotiated.Ok().Some()
	if value.Version != 2 || !slices.Equal(value.Features, []string{"datagram"}) {
		return errors.New("capability negotiation was not deterministic")
	}
	noncanonical := ohrats_rc_mesh_capabilities.Negotiate(
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
	if !noncanonical.IsErr() || noncanonical.Err().Tag() != ohrats_rc_mesh_types.Rejection_InvalidCapability {
		return errors.New("noncanonical capability input was accepted")
	}
	return nil
}

func topologyContract() error {
	advertisements := []ohrats_rc_mesh_types.LinkAdvertisement{
		advertisement("peer-a", []ohrats_rc_mesh_types.Neighbor{neighbor("peer-b", 2)}, nil),
		advertisement("peer-b", []ohrats_rc_mesh_types.Neighbor{neighbor("peer-c", 3)}, nil),
		advertisement("peer-c", nil, []ohrats_rc_mesh_types.ServiceAdvertisement{{Name: "artifact-cache", Cost: 7}}),
	}
	trusted := []string{"peer-b", "peer-c"}
	planned := ohrats_rc_mesh_topology.Routes("realm-a", "peer-a", trusted, advertisements, 1000)
	if planned.IsErr() {
		return rejectionError(planned.Err())
	}
	var route ohrats_rc_mesh_types.Route
	found := false
	for _, candidate := range planned.Ok() {
		if candidate.DestinationPeerId == "peer-c" {
			route = candidate
			found = true
			break
		}
	}
	if !found {
		return errors.New("route to peer-c was not planned")
	}
	if route.NextHopPeerId != "peer-b" || route.TotalCost != 5 {
		return errors.New("shortest mesh route was incorrect")
	}
	service := ohrats_rc_mesh_topology.Service("realm-a", "peer-a", trusted, advertisements, "artifact-cache", 1000)
	if service.IsErr() {
		return rejectionError(service.Err())
	}
	if service.Ok().IsNone() {
		return errors.New("service route was not planned")
	}
	selected := service.Ok().Some()
	if selected.ProviderPeerId != "peer-c" || selected.TotalCost != 12 {
		return errors.New("service route cost was incorrect")
	}
	return nil
}

func envelopeContract() error {
	value := ohrats_rc_mesh_types.Envelope{
		Version:           1,
		RealmId:           "realm-a",
		MessageId:         "message-a",
		SourcePeerId:      "peer-a",
		DestinationPeerId: "peer-c",
		IssuedAtMs:        500,
		ExpiresAtMs:       5000,
		MaxHops:           4,
		Ciphertext:        []uint8("opaque-ciphertext"),
		Route:             []string{},
	}
	if validated := ohrats_rc_mesh_envelopes.Validate(value, 1000, 8); validated.IsErr() {
		return rejectionError(validated.Err())
	}
	forwarded := ohrats_rc_mesh_envelopes.Forward(value, "peer-b")
	if forwarded.IsErr() {
		return rejectionError(forwarded.Err())
	}
	if !slices.Equal(forwarded.Ok().Route, []string{"peer-b"}) {
		return errors.New("mesh relay route was not recorded")
	}
	loop := ohrats_rc_mesh_envelopes.Forward(forwarded.Ok(), "peer-b")
	if !loop.IsErr() || loop.Err().Tag() != ohrats_rc_mesh_types.Rejection_RouteRejected {
		return errors.New("mesh relay loop was accepted")
	}
	expired := value
	expired.ExpiresAtMs = 999
	stale := ohrats_rc_mesh_envelopes.Validate(expired, 1000, 8)
	if !stale.IsErr() || stale.Err().Tag() != ohrats_rc_mesh_types.Rejection_Expired {
		return errors.New("expired mesh envelope was accepted")
	}
	return nil
}

func advertisement(origin string, neighbors []ohrats_rc_mesh_types.Neighbor, services []ohrats_rc_mesh_types.ServiceAdvertisement) ohrats_rc_mesh_types.LinkAdvertisement {
	return ohrats_rc_mesh_types.LinkAdvertisement{
		Version:      1,
		RealmId:      "realm-a",
		OriginPeerId: origin,
		Sequence:     1,
		IssuedAtMs:   500,
		ExpiresAtMs:  5000,
		Capabilities: nil,
		Neighbors:    neighbors,
		Services:     services,
	}
}

func neighbor(peerId string, cost uint32) ohrats_rc_mesh_types.Neighbor {
	return ohrats_rc_mesh_types.Neighbor{PeerId: peerId, Cost: cost}
}

func rejectionError(value ohrats_rc_mesh_types.Rejection) error {
	var label string
	switch value.Tag() {
	case ohrats_rc_mesh_types.Rejection_InvalidIdentifier:
		label = "invalid identifier"
	case ohrats_rc_mesh_types.Rejection_InvalidCapability:
		label = "invalid capability"
	case ohrats_rc_mesh_types.Rejection_InvalidAdvertisement:
		label = "invalid advertisement"
	case ohrats_rc_mesh_types.Rejection_Expired:
		label = "expired"
	case ohrats_rc_mesh_types.Rejection_RealmMismatch:
		label = "realm mismatch"
	case ohrats_rc_mesh_types.Rejection_UntrustedPeer:
		label = "untrusted peer"
	case ohrats_rc_mesh_types.Rejection_InvalidEnvelope:
		label = "invalid envelope"
	case ohrats_rc_mesh_types.Rejection_RouteRejected:
		label = "route rejected"
	default:
		label = fmt.Sprintf("rejection %d", value.Tag())
	}
	return fmt.Errorf("mesh policy rejected fixture: %s", label)
}
