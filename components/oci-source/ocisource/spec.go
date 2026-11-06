package ocisource

import (
	"fmt"
	"strings"
)

// Spec is a parsed OCI reference: registry, repository, and tag or digest.
type Spec struct {
	Registry   string
	Repository string
	Reference  string
}

// ParseSpec parses registry/repository[:tag|@digest], rejecting references the
// registry protocol cannot address unambiguously.
func ParseSpec(value string) (*Spec, error) {
	trimmed := value
	for strings.HasPrefix(trimmed, "//") {
		trimmed = trimmed[2:]
	}
	registry, remainder, found := strings.Cut(trimmed, "/")
	if !found {
		return nil, fmt.Errorf("OCI source must include registry/repository")
	}
	if !validRegistry(registry) {
		return nil, fmt.Errorf("invalid OCI registry %q", registry)
	}
	var repository, reference string
	if at := strings.LastIndex(remainder, "@"); at >= 0 {
		repository = remainder[:at]
		reference = remainder[at+1:]
		if err := ValidateDigest(reference); err != nil {
			return nil, err
		}
	} else {
		slash := strings.LastIndex(remainder, "/")
		if slash < 0 {
			slash = 0
		}
		if offset := strings.LastIndex(remainder[slash:], ":"); offset >= 0 {
			index := slash + offset
			repository = remainder[:index]
			reference = remainder[index+1:]
		} else {
			repository = remainder
			reference = "latest"
		}
	}
	if !validRepository(repository) || !validReference(reference) {
		return nil, fmt.Errorf("invalid OCI reference %q", trimmed)
	}
	return &Spec{Registry: registry, Repository: repository, Reference: reference}, nil
}

// ManifestURL is the distribution manifest endpoint for the reference.
func (s *Spec) ManifestURL() string {
	return fmt.Sprintf("%s://%s/v2/%s/manifests/%s", s.scheme(), s.Registry, s.Repository, s.Reference)
}

// BlobURL is the distribution blob endpoint for a pinned layer digest.
func (s *Spec) BlobURL(digest string) string {
	return fmt.Sprintf("%s://%s/v2/%s/blobs/%s", s.scheme(), s.Registry, s.Repository, digest)
}

// Source renders the canonical package source for a resolved manifest digest.
func (s *Spec) Source(manifestDigest string) string {
	return fmt.Sprintf("oci:%s/%s@%s", s.Registry, s.Repository, manifestDigest)
}

// PackageName derives the artifact name from the final repository segment.
func (s *Spec) PackageName() string {
	if slash := strings.LastIndex(s.Repository, "/"); slash >= 0 {
		return s.Repository[slash+1:]
	}
	return s.Repository
}

// Scope is the pull scope used when the registry challenge omits one.
func (s *Spec) Scope() string {
	return fmt.Sprintf("repository:%s:pull", s.Repository)
}

func (s *Spec) scheme() string {
	if strings.HasPrefix(s.Registry, "localhost") || strings.HasPrefix(s.Registry, "127.0.0.1") {
		return "http"
	}
	return "https"
}

// ValidateDigest accepts only a lowercase sha256 digest of 64 hex digits.
func ValidateDigest(value string) error {
	hex, found := strings.CutPrefix(value, "sha256:")
	if found && len(hex) == 64 && isHex(hex) {
		return nil
	}
	return fmt.Errorf("invalid OCI digest %q", value)
}

func validRegistry(value string) bool {
	if value == "" || len(value) > 255 {
		return false
	}
	for index := 0; index < len(value); index++ {
		byte := value[index]
		if isASCIIAlphanumeric(byte) {
			continue
		}
		switch byte {
		case '.', '-', ':', '[', ']':
			continue
		}
		return false
	}
	return true
}

func validRepository(value string) bool {
	if value == "" || len(value) > 255 {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" {
			return false
		}
		for index := 0; index < len(part); index++ {
			byte := part[index]
			if (byte >= 'a' && byte <= 'z') || (byte >= '0' && byte <= '9') {
				continue
			}
			switch byte {
			case '.', '-', '_':
				continue
			}
			return false
		}
	}
	return true
}

func validReference(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	if strings.HasPrefix(value, "sha256:") {
		return true
	}
	for index := 0; index < len(value); index++ {
		byte := value[index]
		if isASCIIAlphanumeric(byte) {
			continue
		}
		switch byte {
		case '.', '-', '_':
			continue
		}
		return false
	}
	return true
}

func isASCIIAlphanumeric(byte byte) bool {
	return (byte >= 'a' && byte <= 'z') || (byte >= 'A' && byte <= 'Z') || (byte >= '0' && byte <= '9')
}

func isHex(value string) bool {
	for index := 0; index < len(value); index++ {
		byte := value[index]
		if (byte >= '0' && byte <= '9') || (byte >= 'a' && byte <= 'f') || (byte >= 'A' && byte <= 'F') {
			continue
		}
		return false
	}
	return true
}
