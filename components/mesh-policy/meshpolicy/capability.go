package meshpolicy

import (
	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_mesh_types"
)

// Negotiate selects the highest common version and the feature intersection.
func Negotiate(local ohrats_rc_mesh_types.CapabilityRequirement, remote ohrats_rc_mesh_types.CapabilityAdvertisement) witTypes.Result[witTypes.Option[ohrats_rc_mesh_types.NegotiatedCapability], ohrats_rc_mesh_types.Rejection] {
	kind := func() witTypes.Result[witTypes.Option[ohrats_rc_mesh_types.NegotiatedCapability], ohrats_rc_mesh_types.Rejection] {
		return witTypes.Ok[witTypes.Option[ohrats_rc_mesh_types.NegotiatedCapability], ohrats_rc_mesh_types.Rejection](witTypes.None[ohrats_rc_mesh_types.NegotiatedCapability]())
	}
	if err := capability(ohrats_rc_mesh_types.CapabilityAdvertisement{
		Id:       local.Id,
		Versions: local.Versions,
		Features: local.SupportedFeatures,
	}); err != nil {
		return witTypes.Err[witTypes.Option[ohrats_rc_mesh_types.NegotiatedCapability], ohrats_rc_mesh_types.Rejection](rejectionOf(err))
	}
	if err := capability(remote); err != nil {
		return witTypes.Err[witTypes.Option[ohrats_rc_mesh_types.NegotiatedCapability], ohrats_rc_mesh_types.Rejection](rejectionOf(err))
	}
	if len(local.RequiredFeatures) > maxCapabilityFeatures ||
		!strictlySortedString(local.RequiredFeatures) ||
		!allSupported(local.RequiredFeatures, local.SupportedFeatures) {
		return witTypes.Err[witTypes.Option[ohrats_rc_mesh_types.NegotiatedCapability], ohrats_rc_mesh_types.Rejection](ohrats_rc_mesh_types.MakeRejectionInvalidCapability())
	}
	if local.Id != remote.Id {
		return kind()
	}
	remoteVersions := make(map[uint32]struct{}, len(remote.Versions))
	for _, version := range remote.Versions {
		remoteVersions[version] = struct{}{}
	}
	version := uint32(0)
	found := false
	for index := len(local.Versions) - 1; index >= 0; index-- {
		if _, common := remoteVersions[local.Versions[index]]; common {
			version = local.Versions[index]
			found = true
			break
		}
	}
	if !found {
		return kind()
	}
	remoteFeatures := make(map[string]struct{}, len(remote.Features))
	for _, feature := range remote.Features {
		remoteFeatures[feature] = struct{}{}
	}
	for _, feature := range local.RequiredFeatures {
		if _, present := remoteFeatures[feature]; !present {
			return kind()
		}
	}
	features := make([]string, 0, len(local.SupportedFeatures))
	for _, feature := range local.SupportedFeatures {
		if _, present := remoteFeatures[feature]; present {
			features = append(features, feature)
		}
	}
	return witTypes.Ok[witTypes.Option[ohrats_rc_mesh_types.NegotiatedCapability], ohrats_rc_mesh_types.Rejection](
		witTypes.Some(ohrats_rc_mesh_types.NegotiatedCapability{
			Id:       local.Id,
			Version:  version,
			Features: features,
		}),
	)
}

func allSupported(required []string, supported []string) bool {
	for _, feature := range required {
		present := false
		for _, candidate := range supported {
			if candidate == feature {
				present = true
				break
			}
		}
		if !present {
			return false
		}
	}
	return true
}
