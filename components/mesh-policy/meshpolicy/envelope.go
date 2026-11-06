package meshpolicy

import (
	"math"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_mesh_types"
)

// Validate accepts only current, bounded, well-formed envelopes.
func Validate(value ohrats_rc_mesh_types.Envelope, nowMs uint64, maximumHops uint8) witTypes.Result[witTypes.Unit, ohrats_rc_mesh_types.Rejection] {
	if err := identifier(value.RealmId); err != nil {
		return witTypes.Err[witTypes.Unit, ohrats_rc_mesh_types.Rejection](rejectionOf(err))
	}
	if err := identifier(value.SourcePeerId); err != nil {
		return witTypes.Err[witTypes.Unit, ohrats_rc_mesh_types.Rejection](rejectionOf(err))
	}
	if err := identifier(value.DestinationPeerId); err != nil {
		return witTypes.Err[witTypes.Unit, ohrats_rc_mesh_types.Rejection](rejectionOf(err))
	}
	invalid := false
	for _, peer := range value.Route {
		if identifier(peer) != nil {
			invalid = true
			break
		}
	}
	if value.Version != 1 ||
		len(value.MessageId) == 0 ||
		len(value.MessageId) > 128 ||
		value.SourcePeerId == value.DestinationPeerId ||
		len(value.Ciphertext) == 0 ||
		len(value.Ciphertext) > maxCiphertextBytes ||
		value.IssuedAtMs > saturatingAdd(nowMs, 60_000) ||
		value.ExpiresAtMs <= nowMs ||
		value.ExpiresAtMs <= value.IssuedAtMs ||
		value.MaxHops == 0 ||
		value.MaxHops > maximumHops ||
		value.MaxHops > 32 ||
		len(value.Route) > int(value.MaxHops) ||
		invalid {
		if value.ExpiresAtMs <= nowMs {
			return witTypes.Err[witTypes.Unit, ohrats_rc_mesh_types.Rejection](ohrats_rc_mesh_types.MakeRejectionExpired())
		}
		return witTypes.Err[witTypes.Unit, ohrats_rc_mesh_types.Rejection](ohrats_rc_mesh_types.MakeRejectionInvalidEnvelope())
	}
	return witTypes.Ok[witTypes.Unit, ohrats_rc_mesh_types.Rejection](witTypes.Unit{})
}

// Forward records the relay in the route unless it would loop or exceed the hop
// budget.
func Forward(value ohrats_rc_mesh_types.Envelope, relayPeerId string) witTypes.Result[ohrats_rc_mesh_types.Envelope, ohrats_rc_mesh_types.Rejection] {
	if err := identifier(relayPeerId); err != nil {
		return witTypes.Err[ohrats_rc_mesh_types.Envelope, ohrats_rc_mesh_types.Rejection](rejectionOf(err))
	}
	rejected := relayPeerId == value.DestinationPeerId || len(value.Route) >= int(value.MaxHops)
	if !rejected {
		for _, peer := range value.Route {
			if peer == relayPeerId {
				rejected = true
				break
			}
		}
	}
	if rejected {
		return witTypes.Err[ohrats_rc_mesh_types.Envelope, ohrats_rc_mesh_types.Rejection](ohrats_rc_mesh_types.MakeRejectionRouteRejected())
	}
	route := make([]string, 0, len(value.Route)+1)
	route = append(route, value.Route...)
	route = append(route, relayPeerId)
	value.Route = route
	return witTypes.Ok[ohrats_rc_mesh_types.Envelope, ohrats_rc_mesh_types.Rejection](value)
}

func saturatingAdd(value uint64, increment uint64) uint64 {
	if value > math.MaxUint64-increment {
		return math.MaxUint64
	}
	return value + increment
}
