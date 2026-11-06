package mesh

import "errors"

type CoordinatorRole string

const (
	CoordinatorRoleTier0     CoordinatorRole = "tier0"
	CoordinatorRoleSecondary CoordinatorRole = "secondary"
)

type CoordinatorPolicy struct {
	Role     CoordinatorRole `json:"role"`
	Upstream *ServiceID      `json:"upstream,omitempty"`
}

var (
	ErrTier0Upstream   = errors.New("Tier-0 coordinators cannot have an upstream authority")
	ErrMissingUpstream = errors.New("secondary coordinators require an upstream authority")
	ErrRevocationLease = errors.New("invalid revocation lease")
	ErrRevocationExp   = errors.New("revocation lease expired")
)

func Tier0Policy() CoordinatorPolicy {
	return CoordinatorPolicy{Role: CoordinatorRoleTier0}
}

func SecondaryPolicy(upstream ServiceID) CoordinatorPolicy {
	return CoordinatorPolicy{
		Role:     CoordinatorRoleSecondary,
		Upstream: &upstream,
	}
}

type RevocationLease struct {
	Realm         RealmID `json:"realm"`
	Epoch         uint64  `json:"epoch"`
	IssuedAt      int64   `json:"issuedAt"`
	ValidUntil    int64   `json:"validUntil"`
	TombstoneRoot string  `json:"tombstoneRoot"`
}

func (l *RevocationLease) Validate(nowMs int64) error {
	if l.Epoch == 0 || l.IssuedAt > nowMs || l.ValidUntil <= l.IssuedAt || len(l.TombstoneRoot) != 64 {
		return ErrRevocationLease
	}
	for i := 0; i < len(l.TombstoneRoot); i++ {
		b := l.TombstoneRoot[i]
		isHex := (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
		if !isHex {
			return ErrRevocationLease
		}
	}
	if l.ValidUntil <= nowMs {
		return ErrRevocationExp
	}
	return nil
}
