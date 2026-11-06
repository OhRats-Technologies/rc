package mesh

import (
	"errors"
	"fmt"
	"sync"
)

type PeerRouteProvider interface {
	PeerID() PeerID
	Send(envelope *SignedMeshEnvelope) error
}

type EnvelopeDispositionKind int

const (
	DispositionDeliver EnvelopeDispositionKind = iota
	DispositionForward
)

type EnvelopeDisposition struct {
	Kind  EnvelopeDispositionKind
	Route Route
}

type MeshRealm struct {
	ID       RealmID
	Local    PeerID
	Topology *Topology
	replay   *ReplayGuard
	mu       sync.RWMutex
	routes   []PeerRouteProvider
}

func NewMeshRealm(id RealmID, local PeerID) *MeshRealm {
	return &MeshRealm{
		ID:       id,
		Local:    local,
		Topology: NewTopology(id, local),
		replay:   NewReplayGuard(),
	}
}

func (m *MeshRealm) RegisterRoute(provider PeerRouteProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes = append(m.routes, provider)
}

func (m *MeshRealm) AcceptEnvelope(envelope *SignedMeshEnvelope, sourcePublicKey string, nowMs int64) (EnvelopeDisposition, error) {
	if envelope.Header.RealmID != m.ID {
		return EnvelopeDisposition{}, errors.New("mesh envelope belongs to another realm")
	}
	if _, err := envelope.Verify(sourcePublicKey, nowMs); err != nil {
		return EnvelopeDisposition{}, err
	}
	if !m.replay.Accept(envelope, nowMs) {
		return EnvelopeDisposition{}, errors.New("mesh envelope replay rejected")
	}
	if envelope.Header.Destination == m.Local {
		return EnvelopeDisposition{Kind: DispositionDeliver}, nil
	}
	route, ok := m.Topology.Route(envelope.Header.Destination, nowMs)
	if !ok {
		return EnvelopeDisposition{}, errors.New("mesh destination is unreachable")
	}
	return EnvelopeDisposition{Kind: DispositionForward, Route: route}, nil
}

func (m *MeshRealm) Forward(envelope *SignedMeshEnvelope, route Route) error {
	m.mu.RLock()
	var nextHopProvider PeerRouteProvider
	for _, p := range m.routes {
		if p.PeerID() == route.NextHop {
			nextHopProvider = p
			break
		}
	}
	m.mu.RUnlock()

	if nextHopProvider == nil {
		return fmt.Errorf("mesh next hop %s is unavailable", route.NextHop)
	}

	forwarded, err := envelope.Forward(m.Local)
	if err != nil {
		return err
	}
	return nextHopProvider.Send(forwarded)
}
