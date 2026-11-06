package mesh

import (
	"encoding/json"
	"errors"
)

const AdvertisementVersion uint32 = 1

var (
	ErrAdvShape    = errors.New("invalid mesh advertisement")
	ErrAdvIdentity = errors.New("mesh advertisement identity mismatch")
	ErrAdvExpired  = errors.New("mesh advertisement expired")
	ErrAdvSig      = errors.New("invalid mesh advertisement signature")
	ErrAdvEncoding = errors.New("mesh advertisement encoding failed")
)

type Neighbor struct {
	PeerID PeerID `json:"peerId"`
	Cost   uint32 `json:"cost"`
}

type ServiceAdvertisement struct {
	Name string `json:"name"`
	Cost uint32 `json:"cost"`
}

type LinkAdvertisement struct {
	V            uint32                    `json:"v"`
	RealmID      RealmID                   `json:"realmId"`
	Origin       PeerID                    `json:"origin"`
	Sequence     uint64                    `json:"sequence"`
	IssuedAt     int64                     `json:"issuedAt"`
	ExpiresAt    int64                     `json:"expiresAt"`
	Capabilities []CapabilityAdvertisement `json:"capabilities,omitempty"`
	Neighbors    []Neighbor                `json:"neighbors"`
	Services     []ServiceAdvertisement    `json:"services"`
}

type SignedAdvertisement struct {
	Advertisement LinkAdvertisement `json:"advertisement"`
	Signature     string            `json:"signature"`
}

func SignAdvertisement(adv LinkAdvertisement, seed string) (*SignedAdvertisement, error) {
	if err := validateAdvShape(&adv); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(adv)
	if err != nil {
		return nil, err
	}
	sig, err := SignPayload(seed, "rc-mesh-link-v1", payload)
	if err != nil {
		return nil, err
	}
	return &SignedAdvertisement{
		Advertisement: adv,
		Signature:     sig,
	}, nil
}

func (s *SignedAdvertisement) Verify(publicKey string, nowMs int64) error {
	if err := validateAdvShape(&s.Advertisement); err != nil {
		return err
	}
	expected, err := PeerIDFromPublicKey(publicKey)
	if err != nil || expected != s.Advertisement.Origin {
		return ErrAdvIdentity
	}
	if s.Advertisement.IssuedAt > nowMs+60000 || s.Advertisement.ExpiresAt <= nowMs || s.Advertisement.ExpiresAt <= s.Advertisement.IssuedAt {
		return ErrAdvExpired
	}
	payload, err := json.Marshal(s.Advertisement)
	if err != nil {
		return ErrAdvEncoding
	}
	if err := VerifyPayload(publicKey, "rc-mesh-link-v1", payload, s.Signature); err != nil {
		return ErrAdvSig
	}
	return nil
}

func validateAdvShape(a *LinkAdvertisement) error {
	if a.V != AdvertisementVersion || len(a.RealmID) == 0 || len(a.Origin) == 0 ||
		len(a.Capabilities) > MaxCapabilities || len(a.Neighbors) > 256 || len(a.Services) > 64 {
		return ErrAdvShape
	}

	seenN := make(map[PeerID]bool)
	for _, n := range a.Neighbors {
		if n.Cost == 0 || n.PeerID == a.Origin || seenN[n.PeerID] {
			return ErrAdvShape
		}
		seenN[n.PeerID] = true
	}

	seenS := make(map[string]bool)
	for _, s := range a.Services {
		if len(s.Name) == 0 || len(s.Name) > 128 || s.Cost == 0 || seenS[s.Name] {
			return ErrAdvShape
		}
		seenS[s.Name] = true
	}

	seenC := make(map[string]bool)
	for _, c := range a.Capabilities {
		if err := c.Validate(); err != nil || seenC[c.ID] {
			return ErrAdvShape
		}
		seenC[c.ID] = true
	}
	return nil
}
