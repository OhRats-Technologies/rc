package mesh

import (
	"errors"
	"sort"
)

const (
	MaxCapabilities       = 64
	MaxCapabilityVersions = 16
	MaxCapabilityFeatures = 32
)

var (
	ErrCapID               = errors.New("invalid capability id")
	ErrCapVersions         = errors.New("invalid capability versions")
	ErrCapFeatures         = errors.New("invalid capability features")
	ErrCapRequiredFeatures = errors.New("required capability features are not locally supported")
)

type CapabilityAdvertisement struct {
	ID       string   `json:"id"`
	Versions []uint32 `json:"versions"`
	Features []string `json:"features,omitempty"`
}

func NewCapabilityAdvertisement(id string, versions []uint32, features []string) (*CapabilityAdvertisement, error) {
	vSet := make(map[uint32]bool)
	var vList []uint32
	for _, v := range versions {
		if !vSet[v] {
			vSet[v] = true
			vList = append(vList, v)
		}
	}
	sort.Slice(vList, func(i, j int) bool { return vList[i] < vList[j] })

	fSet := make(map[string]bool)
	var fList []string
	for _, f := range features {
		if !fSet[f] {
			fSet[f] = true
			fList = append(fList, f)
		}
	}
	sort.Strings(fList)

	c := &CapabilityAdvertisement{
		ID:       id,
		Versions: vList,
		Features: fList,
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *CapabilityAdvertisement) Validate() error {
	if !validToken(c.ID, 128, true) {
		return ErrCapID
	}
	if len(c.Versions) == 0 || len(c.Versions) > MaxCapabilityVersions || !isStrictlySortedU32(c.Versions) {
		return ErrCapVersions
	}
	for _, v := range c.Versions {
		if v == 0 {
			return ErrCapVersions
		}
	}
	if len(c.Features) > MaxCapabilityFeatures || !isStrictlySortedStr(c.Features) {
		return ErrCapFeatures
	}
	for _, f := range c.Features {
		if !validToken(f, 64, false) {
			return ErrCapFeatures
		}
	}
	return nil
}

type CapabilityRequirement struct {
	ID                string   `json:"id"`
	Versions          []uint32 `json:"versions"`
	SupportedFeatures []string `json:"supportedFeatures"`
	RequiredFeatures  []string `json:"requiredFeatures"`
}

func NewCapabilityRequirement(id string, versions []uint32, supportedFeatures, requiredFeatures []string) (*CapabilityRequirement, error) {
	adv, err := NewCapabilityAdvertisement(id, versions, supportedFeatures)
	if err != nil {
		return nil, err
	}
	rSet := make(map[string]bool)
	var rList []string
	for _, r := range requiredFeatures {
		if !rSet[r] {
			rSet[r] = true
			rList = append(rList, r)
		}
	}
	sort.Strings(rList)

	if len(rList) > MaxCapabilityFeatures {
		return nil, ErrCapRequiredFeatures
	}
	for _, req := range rList {
		idx := sort.SearchStrings(adv.Features, req)
		if idx >= len(adv.Features) || adv.Features[idx] != req {
			return nil, ErrCapRequiredFeatures
		}
	}

	return &CapabilityRequirement{
		ID:                adv.ID,
		Versions:          adv.Versions,
		SupportedFeatures: adv.Features,
		RequiredFeatures:  rList,
	}, nil
}

type NegotiatedCapability struct {
	ID       string   `json:"id"`
	Version  uint32   `json:"version"`
	Features []string `json:"features"`
}

func (r *CapabilityRequirement) Negotiate(remote *CapabilityAdvertisement) *NegotiatedCapability {
	if err := remote.Validate(); err != nil || r.ID != remote.ID {
		return nil
	}
	remVer := make(map[uint32]bool)
	for _, v := range remote.Versions {
		remVer[v] = true
	}

	var matchVersion uint32
	found := false
	for i := len(r.Versions) - 1; i >= 0; i-- {
		if remVer[r.Versions[i]] {
			matchVersion = r.Versions[i]
			found = true
			break
		}
	}
	if !found {
		return nil
	}

	remFeat := make(map[string]bool)
	for _, f := range remote.Features {
		remFeat[f] = true
	}
	for _, req := range r.RequiredFeatures {
		if !remFeat[req] {
			return nil
		}
	}

	var commonFeatures []string
	for _, f := range r.SupportedFeatures {
		if remFeat[f] {
			commonFeatures = append(commonFeatures, f)
		}
	}

	return &NegotiatedCapability{
		ID:       r.ID,
		Version:  matchVersion,
		Features: commonFeatures,
	}
}

func isStrictlySortedU32(vals []uint32) bool {
	for i := 1; i < len(vals); i++ {
		if vals[i-1] >= vals[i] {
			return false
		}
	}
	return true
}

func isStrictlySortedStr(vals []string) bool {
	for i := 1; i < len(vals); i++ {
		if vals[i-1] >= vals[i] {
			return false
		}
	}
	return true
}

func validToken(val string, maxLen int, allowSlash bool) bool {
	if len(val) == 0 || len(val) > maxLen {
		return false
	}
	for i := 0; i < len(val); i++ {
		b := val[i]
		isAlnum := (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
		isAllowedPunct := b == '.' || b == '-' || b == '_' || (allowSlash && b == '/')
		if !isAlnum && !isAllowedPunct {
			return false
		}
	}
	first := val[0]
	last := val[len(val)-1]
	firstAlnum := (first >= 'a' && first <= 'z') || (first >= '0' && first <= '9')
	lastAlnum := (last >= 'a' && last <= 'z') || (last >= '0' && last <= '9')
	return firstAlnum && lastAlnum
}
