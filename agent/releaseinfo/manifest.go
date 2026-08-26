package releaseinfo

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

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

func Parse(data []byte) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("invalid release manifest: %w", err)
	}
	if !versionPattern.MatchString(manifest.Version) || len(manifest.Artifacts) != 4 {
		return Manifest{}, errors.New("invalid release manifest contents")
	}
	seen := map[string]bool{}
	for _, artifact := range manifest.Artifacts {
		key := artifact.OS + "/" + artifact.Arch
		expectedName := "rc-" + artifact.OS + "-" + artifact.Arch
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
		if lv < rv {
			return -1, nil
		}
		if lv > rv {
			return 1, nil
		}
	}
	return 0, nil
}
