package mesh

import (
	"encoding/json"
	"errors"
)

var (
	ErrStateDigestShape    = errors.New("invalid mesh state digest")
	ErrStateDigestExpired  = errors.New("mesh state digest expired")
	ErrStateDigestIdentity = errors.New("mesh state digest identity mismatch")
	ErrStateDigestSig      = errors.New("invalid mesh state digest signature")
	ErrStateDigestEncoding = errors.New("mesh state digest encoding failed")
)

type AuthorityHead struct {
	Generation uint64 `json:"generation"`
	Hash       string `json:"hash"`
	ValidUntil int64  `json:"validUntil"`
}

type ReleaseHead struct {
	Version string `json:"version"`
	Target  string `json:"target"`
	Sha256  string `json:"sha256"`
	Size    uint64 `json:"size"`
}

type MeshStateDigest struct {
	V                    uint32        `json:"v"`
	RealmID              RealmID       `json:"realmId"`
	Origin               PeerID        `json:"origin"`
	Sequence             uint64        `json:"sequence"`
	IssuedAt             int64         `json:"issuedAt"`
	ExpiresAt            int64         `json:"expiresAt"`
	Authority            AuthorityHead `json:"authority"`
	DeviceOperationsRoot string        `json:"deviceOperationsRoot"`
	RevocationEpoch      uint64        `json:"revocationEpoch"`
	Releases             []ReleaseHead `json:"releases,omitempty"`
}

type SignedStateDigest struct {
	Digest    MeshStateDigest `json:"digest"`
	Signature string          `json:"signature"`
}

func SignStateDigest(digest MeshStateDigest, identitySeed string) (*SignedStateDigest, error) {
	if err := validateStateDigest(&digest); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(digest)
	if err != nil {
		return nil, err
	}
	sig, err := SignPayload(identitySeed, "rc-mesh-state-v1", payload)
	if err != nil {
		return nil, err
	}
	return &SignedStateDigest{
		Digest:    digest,
		Signature: sig,
	}, nil
}

func (s *SignedStateDigest) Verify(publicKey string, nowMs int64) error {
	if err := validateStateDigest(&s.Digest); err != nil {
		return err
	}
	if s.Digest.IssuedAt > nowMs+60000 || s.Digest.ExpiresAt <= nowMs {
		return ErrStateDigestExpired
	}
	expected, err := PeerIDFromPublicKey(publicKey)
	if err != nil || expected != s.Digest.Origin {
		return ErrStateDigestIdentity
	}
	payload, err := json.Marshal(s.Digest)
	if err != nil {
		return ErrStateDigestEncoding
	}
	if err := VerifyPayload(publicKey, "rc-mesh-state-v1", payload, s.Signature); err != nil {
		return ErrStateDigestSig
	}
	return nil
}

func validateStateDigest(d *MeshStateDigest) error {
	if d.V != 1 || len(d.RealmID) == 0 || len(d.Origin) == 0 ||
		d.ExpiresAt <= d.IssuedAt || len(d.Authority.Hash) != 64 ||
		len(d.DeviceOperationsRoot) != 64 || len(d.Releases) > 64 {
		return ErrStateDigestShape
	}
	for _, r := range d.Releases {
		if len(r.Version) == 0 || len(r.Target) == 0 || len(r.Sha256) != 64 || r.Size == 0 {
			return ErrStateDigestShape
		}
	}
	return nil
}
