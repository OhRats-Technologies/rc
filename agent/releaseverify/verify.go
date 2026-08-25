package releaseverify

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

//go:embed release-public.pem
var releasePublicPEM []byte

type Artifact struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Version   string     `json:"version"`
	Artifacts []Artifact `json:"artifacts"`
}

var versionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

func Verify(manifestBytes, signatureBytes []byte) (Manifest, error) {
	block, _ := pem.Decode(releasePublicPEM)
	if block == nil {
		return Manifest{}, errors.New("invalid embedded release public key")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return Manifest{}, fmt.Errorf("parse release public key: %w", err)
	}
	publicKey, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return Manifest{}, errors.New("release public key is not Ed25519")
	}
	signature, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(signatureBytes)))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return Manifest{}, errors.New("invalid release signature encoding")
	}
	if !ed25519.Verify(publicKey, manifestBytes, signature) {
		return Manifest{}, errors.New("release signature verification failed")
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("invalid release manifest: %w", err)
	}
	if !versionPattern.MatchString(manifest.Version) || len(manifest.Artifacts) != 4 {
		return Manifest{}, errors.New("invalid release manifest contents")
	}
	seen := map[string]bool{}
	for _, artifact := range manifest.Artifacts {
		key := artifact.OS + "/" + artifact.Arch
		expectedName := "ohrats-rc-" + artifact.OS + "-" + artifact.Arch
		if seen[key] || artifact.Name != expectedName ||
			(artifact.OS != "linux" && artifact.OS != "darwin") ||
			(artifact.Arch != "amd64" && artifact.Arch != "arm64") || len(artifact.SHA256) != 64 {
			return Manifest{}, errors.New("invalid release artifact")
		}
		if _, err := hex.DecodeString(artifact.SHA256); err != nil {
			return Manifest{}, errors.New("invalid release artifact hash")
		}
		seen[key] = true
	}
	return manifest, nil
}

func ArtifactFor(manifest Manifest, osName, arch string) (Artifact, bool) {
	for _, artifact := range manifest.Artifacts {
		if artifact.OS == osName && artifact.Arch == arch {
			return artifact, true
		}
	}
	return Artifact{}, false
}

func CompareVersions(left, right string) (int, error) {
	l := versionPattern.FindStringSubmatch(left)
	r := versionPattern.FindStringSubmatch(right)
	if l == nil || r == nil {
		return 0, errors.New("invalid semantic version")
	}
	for i := 1; i <= 3; i++ {
		lv, _ := strconv.Atoi(l[i])
		rv, _ := strconv.Atoi(r[i])
		if lv < rv { return -1, nil }
		if lv > rv { return 1, nil }
	}
	return 0, nil
}

func FileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil { return "", err }
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil { return "", err }
	return hex.EncodeToString(hash.Sum(nil)), nil
}
