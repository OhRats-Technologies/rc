package mesh

import (
	"encoding/json"
	"errors"
	"sync"
)

const EnvelopeVersion uint32 = 1

var (
	ErrEnvelopeShape     = errors.New("invalid mesh envelope")
	ErrEnvelopeExpired   = errors.New("mesh envelope expired")
	ErrEnvelopeIdentity  = errors.New("mesh envelope identity mismatch")
	ErrEnvelopeSignature = errors.New("invalid mesh envelope signature")
	ErrEnvelopeEncoding  = errors.New("invalid mesh envelope encoding")
	ErrEnvelopeRoute     = errors.New("mesh envelope route rejected")
)

type EnvelopeHeader struct {
	V           uint32  `json:"v"`
	RealmID     RealmID `json:"realmId"`
	MessageID   string  `json:"messageId"`
	Source      PeerID  `json:"source"`
	Destination PeerID  `json:"destination"`
	IssuedAt    int64   `json:"issuedAt"`
	ExpiresAt   int64   `json:"expiresAt"`
	MaxHops     uint8   `json:"maxHops"`
}

type SignedMeshEnvelope struct {
	Header     EnvelopeHeader `json:"header"`
	Ciphertext string         `json:"ciphertext"`
	Signature  string         `json:"signature"`
	Route      []PeerID       `json:"route,omitempty"`
}

func SignEnvelope(header EnvelopeHeader, ciphertext []byte, seed string) (*SignedMeshEnvelope, error) {
	if err := validateHeader(&header); err != nil {
		return nil, err
	}
	if len(ciphertext) == 0 || len(ciphertext) > 1500000 {
		return nil, ErrEnvelopeShape
	}
	encoded := b64URL.EncodeToString(ciphertext)
	payload, err := json.Marshal([]any{header, encoded})
	if err != nil {
		return nil, err
	}
	sig, err := SignPayload(seed, "rc-mesh-envelope-v1", payload)
	if err != nil {
		return nil, err
	}
	return &SignedMeshEnvelope{
		Header:     header,
		Ciphertext: encoded,
		Signature:  sig,
	}, nil
}

func (s *SignedMeshEnvelope) Verify(sourcePublicKey string, nowMs int64) ([]byte, error) {
	if err := validateHeader(&s.Header); err != nil {
		return nil, err
	}
	if s.Header.IssuedAt > nowMs+60000 || s.Header.ExpiresAt <= nowMs {
		return nil, ErrEnvelopeExpired
	}
	expected, err := PeerIDFromPublicKey(sourcePublicKey)
	if err != nil || expected != s.Header.Source {
		return nil, ErrEnvelopeIdentity
	}
	ciphertext, err := b64URL.DecodeString(s.Ciphertext)
	if err != nil {
		return nil, ErrEnvelopeEncoding
	}
	if len(ciphertext) == 0 || len(ciphertext) > 1500000 {
		return nil, ErrEnvelopeShape
	}
	payload, err := json.Marshal([]any{s.Header, s.Ciphertext})
	if err != nil {
		return nil, ErrEnvelopeEncoding
	}
	if err := VerifyPayload(sourcePublicKey, "rc-mesh-envelope-v1", payload, s.Signature); err != nil {
		return nil, ErrEnvelopeSignature
	}
	return ciphertext, nil
}

func (s *SignedMeshEnvelope) Forward(relay PeerID) (*SignedMeshEnvelope, error) {
	if s.Header.Destination == relay || len(s.Route) >= int(s.Header.MaxHops) {
		return nil, ErrEnvelopeRoute
	}
	for _, p := range s.Route {
		if p == relay {
			return nil, ErrEnvelopeRoute
		}
	}
	next := *s
	next.Route = append(append([]PeerID(nil), s.Route...), relay)
	return &next, nil
}

func validateHeader(h *EnvelopeHeader) error {
	if h.V != EnvelopeVersion ||
		len(h.RealmID) == 0 ||
		len(h.MessageID) == 0 ||
		len(h.MessageID) > 128 ||
		len(h.Source) == 0 ||
		len(h.Destination) == 0 ||
		h.Source == h.Destination ||
		h.MaxHops == 0 ||
		h.MaxHops > 32 ||
		h.ExpiresAt <= h.IssuedAt {
		return ErrEnvelopeShape
	}
	return nil
}

type ReplayGuard struct {
	mu   sync.Mutex
	seen map[string]int64 // key is string(source)+"\x00"+messageID
}

func NewReplayGuard() *ReplayGuard {
	return &ReplayGuard{seen: make(map[string]int64)}
}

func (rg *ReplayGuard) Accept(env *SignedMeshEnvelope, nowMs int64) bool {
	rg.mu.Lock()
	defer rg.mu.Unlock()

	for k, exp := range rg.seen {
		if exp <= nowMs {
			delete(rg.seen, k)
		}
	}
	key := string(env.Header.Source) + "\x00" + env.Header.MessageID
	if _, ok := rg.seen[key]; ok {
		return false
	}
	rg.seen[key] = env.Header.ExpiresAt
	return true
}
