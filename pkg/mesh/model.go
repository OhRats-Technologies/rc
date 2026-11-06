package mesh

import (
	"errors"
)

const MaxEnvelopePayload = 1048576 // 1 MiB

type RealmID string
type PeerID string
type ServiceID string

type RouteTargetKind string

const (
	RouteTargetDevice  RouteTargetKind = "device"
	RouteTargetService RouteTargetKind = "service"
)

type RouteTarget struct {
	Kind RouteTargetKind `json:"kind"`
	ID   string          `json:"id"`
}

type RouteDescriptor struct {
	Realm     RealmID     `json:"realm"`
	Target    RouteTarget `json:"target"`
	Via       PeerID      `json:"via"`
	Cost      uint32      `json:"cost"`
	ExpiresAt int64       `json:"expiresAt"`
}

type MeshEnvelope struct {
	Version     uint8   `json:"version"`
	Realm       RealmID `json:"realm"`
	MessageID   string  `json:"messageId"`
	Source      PeerID  `json:"source"`
	Destination PeerID  `json:"destination"`
	ExpiresAt   int64   `json:"expiresAt"`
	HopLimit    uint8   `json:"hopLimit"`
	Payload     []byte  `json:"payload"`
}

type MeshPolicy struct {
	Enabled            bool  `json:"enabled"`
	AuthenticatedRelay bool  `json:"authenticatedRelay"`
	LanDiscovery       bool  `json:"lanDiscovery"`
	PublicRelay        bool  `json:"publicRelay"`
	MaximumHops        uint8 `json:"maximumHops"`
}

func DefaultMeshPolicy() MeshPolicy {
	return MeshPolicy{
		Enabled:            true,
		AuthenticatedRelay: true,
		LanDiscovery:       true,
		PublicRelay:        false,
		MaximumHops:        8,
	}
}

var ErrIdentifier = errors.New("mesh identifiers must be 1-128 visible ASCII characters")
var ErrEnvelope = errors.New("invalid mesh envelope")
var ErrExpired = errors.New("mesh envelope expired")
var ErrHopLimit = errors.New("mesh envelope hop limit exhausted")

func ValidateIdentifier(value string) (string, error) {
	if len(value) == 0 || len(value) > 128 {
		return "", ErrIdentifier
	}
	for i := 0; i < len(value); i++ {
		b := value[i]
		if b < 33 || b > 126 {
			return "", ErrIdentifier
		}
	}
	return value, nil
}

func NewRealmID(val string) (RealmID, error) {
	s, err := ValidateIdentifier(val)
	return RealmID(s), err
}

func NewPeerID(val string) (PeerID, error) {
	s, err := ValidateIdentifier(val)
	return PeerID(s), err
}

func NewServiceID(val string) (ServiceID, error) {
	s, err := ValidateIdentifier(val)
	return ServiceID(s), err
}

func (e *MeshEnvelope) Validate(nowMs int64, maxHops uint8) error {
	if e.Version != 1 || len(e.MessageID) == 0 || len(e.MessageID) > 128 {
		return ErrEnvelope
	}
	if e.Source == e.Destination || len(e.Payload) == 0 || len(e.Payload) > MaxEnvelopePayload {
		return ErrEnvelope
	}
	if e.ExpiresAt <= nowMs {
		return ErrExpired
	}
	if e.HopLimit == 0 || e.HopLimit > maxHops {
		return ErrHopLimit
	}
	return nil
}

func (e *MeshEnvelope) Forwarded() (*MeshEnvelope, error) {
	if e.HopLimit <= 1 {
		return nil, ErrHopLimit
	}
	next := *e
	next.HopLimit--
	return &next, nil
}
