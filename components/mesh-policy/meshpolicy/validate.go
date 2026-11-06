package meshpolicy

import "wit_component/ohrats_rc_mesh_types"

const (
	maxCapabilities       = 64
	maxCapabilityVersions = 16
	maxCapabilityFeatures = 32
	maxNeighbors          = 256
	maxServices           = 64
	maxCiphertextBytes    = 1_500_000
)

// identifier accepts 1..=128 bytes of printable ASCII without whitespace.
func identifier(value string) error {
	if len(value) == 0 || len(value) > 128 {
		return reject(ohrats_rc_mesh_types.MakeRejectionInvalidIdentifier())
	}
	for index := 0; index < len(value); index++ {
		if value[index] < 0x21 || value[index] > 0x7e {
			return reject(ohrats_rc_mesh_types.MakeRejectionInvalidIdentifier())
		}
	}
	return nil
}

func capability(value ohrats_rc_mesh_types.CapabilityAdvertisement) error {
	if !token(value.Id, 128, true) ||
		len(value.Versions) == 0 ||
		len(value.Versions) > maxCapabilityVersions ||
		containsZero(value.Versions) ||
		!strictlySortedUint32(value.Versions) ||
		len(value.Features) > maxCapabilityFeatures ||
		!strictlySortedString(value.Features) ||
		!allTokens(value.Features, 64) {
		return reject(ohrats_rc_mesh_types.MakeRejectionInvalidCapability())
	}
	return nil
}

func advertisement(value ohrats_rc_mesh_types.LinkAdvertisement) error {
	if err := identifier(value.RealmId); err != nil {
		return err
	}
	if err := identifier(value.OriginPeerId); err != nil {
		return err
	}
	capabilities := make(map[string]struct{}, len(value.Capabilities))
	neighbors := make(map[string]struct{}, len(value.Neighbors))
	services := make(map[string]struct{}, len(value.Services))
	if value.Version != 1 ||
		value.ExpiresAtMs <= value.IssuedAtMs ||
		len(value.Capabilities) > maxCapabilities ||
		len(value.Neighbors) > maxNeighbors ||
		len(value.Services) > maxServices {
		return reject(ohrats_rc_mesh_types.MakeRejectionInvalidAdvertisement())
	}
	for _, neighbor := range value.Neighbors {
		if identifier(neighbor.PeerId) != nil ||
			neighbor.Cost == 0 ||
			neighbor.PeerId == value.OriginPeerId {
			return reject(ohrats_rc_mesh_types.MakeRejectionInvalidAdvertisement())
		}
		if _, duplicate := neighbors[neighbor.PeerId]; duplicate {
			return reject(ohrats_rc_mesh_types.MakeRejectionInvalidAdvertisement())
		}
		neighbors[neighbor.PeerId] = struct{}{}
	}
	for _, service := range value.Services {
		if len(service.Name) == 0 || len(service.Name) > 128 || service.Cost == 0 {
			return reject(ohrats_rc_mesh_types.MakeRejectionInvalidAdvertisement())
		}
		if _, duplicate := services[service.Name]; duplicate {
			return reject(ohrats_rc_mesh_types.MakeRejectionInvalidAdvertisement())
		}
		services[service.Name] = struct{}{}
	}
	for _, item := range value.Capabilities {
		if err := capability(item); err != nil {
			return err
		}
		if _, duplicate := capabilities[item.Id]; duplicate {
			return reject(ohrats_rc_mesh_types.MakeRejectionInvalidAdvertisement())
		}
		capabilities[item.Id] = struct{}{}
	}
	return nil
}

func strictlySortedUint32(values []uint32) bool {
	for index := 1; index < len(values); index++ {
		if values[index-1] >= values[index] {
			return false
		}
	}
	return true
}

func strictlySortedString(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index-1] >= values[index] {
			return false
		}
	}
	return true
}

func containsZero(values []uint32) bool {
	for _, value := range values {
		if value == 0 {
			return true
		}
	}
	return false
}

func allTokens(values []string, maximum int) bool {
	for _, value := range values {
		if !token(value, maximum, false) {
			return false
		}
	}
	return true
}

// token accepts lowercase alphanumeric segments joined by '.', '-', '_' and,
// when slash is set, '/'.
func token(value string, maximum int, slash bool) bool {
	if len(value) == 0 || len(value) > maximum {
		return false
	}
	for index := 0; index < len(value); index++ {
		byteValue := value[index]
		if byteValue >= 'a' && byteValue <= 'z' ||
			byteValue >= '0' && byteValue <= '9' ||
			byteValue == '.' || byteValue == '-' || byteValue == '_' ||
			slash && byteValue == '/' {
			continue
		}
		return false
	}
	return asciiAlphanumeric(value[0]) && asciiAlphanumeric(value[len(value)-1])
}

func asciiAlphanumeric(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9'
}
