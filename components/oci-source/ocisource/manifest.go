package ocisource

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type rawManifest struct {
	Layers *[]rawLayer `json:"layers"`
}

type rawLayer struct {
	MediaType   *string           `json:"mediaType"`
	Digest      *string           `json:"digest"`
	Size        *uint64           `json:"size"`
	Annotations map[string]string `json:"annotations"`
}

type layer struct {
	mediaType   string
	digest      string
	size        uint64
	annotations map[string]string
}

// decodeLayers parses the manifest layer list, rejecting manifests that omit
// fields the OCI schema requires.
func decodeLayers(body []byte) ([]layer, error) {
	var manifest rawManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("invalid OCI manifest: %s", err)
	}
	if manifest.Layers == nil {
		return nil, errors.New("invalid OCI manifest: missing field `layers`")
	}
	layers := make([]layer, 0, len(*manifest.Layers))
	for index, raw := range *manifest.Layers {
		if raw.MediaType == nil {
			return nil, fmt.Errorf("invalid OCI manifest: missing field `mediaType` in layer %d", index)
		}
		if raw.Digest == nil {
			return nil, fmt.Errorf("invalid OCI manifest: missing field `digest` in layer %d", index)
		}
		if raw.Size == nil {
			return nil, fmt.Errorf("invalid OCI manifest: missing field `size` in layer %d", index)
		}
		layers = append(layers, layer{
			mediaType:   *raw.MediaType,
			digest:      *raw.Digest,
			size:        *raw.Size,
			annotations: raw.Annotations,
		})
	}
	return layers, nil
}

// selectLayer picks the single WebAssembly layer, falling back to a sole
// layer of unknown media type.
func selectLayer(layers []layer) (*layer, error) {
	candidates := make([]*layer, 0, len(layers))
	for index := range layers {
		candidate := &layers[index]
		if strings.Contains(candidate.mediaType, "wasm") {
			candidates = append(candidates, candidate)
			continue
		}
		if title, found := candidate.annotations["org.opencontainers.image.title"]; found && strings.HasSuffix(title, ".wasm") {
			candidates = append(candidates, candidate)
		}
	}
	switch {
	case len(candidates) == 1:
		return candidates[0], nil
	case len(candidates) == 0 && len(layers) == 1:
		return &layers[0], nil
	case len(candidates) == 0:
		return nil, errors.New("OCI manifest has no WebAssembly component layer")
	default:
		return nil, errors.New("OCI manifest has multiple WebAssembly component layers")
	}
}

// validName reports whether an artifact title is a safe component name.
func validName(value string) bool {
	if value == "" || len(value) > 96 {
		return false
	}
	for index := 0; index < len(value); index++ {
		byte := value[index]
		if isASCIIAlphanumeric(byte) {
			continue
		}
		switch byte {
		case '-', '_', '.':
			continue
		}
		return false
	}
	return true
}
