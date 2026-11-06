package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

const (
	maxArchiveEntries = 256
	maxLockBytes      = 1 << 20
)

// Profile is a validated component bundle.
type Profile struct {
	Lock       string
	Components map[string][]byte
	Digests    map[string]string
}

// ExtractSingle returns the only regular file in a gzip tar archive.
func ExtractSingle(archive []byte, expected string, limit int64) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("read %s archive: %w", expected, err)
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	var value []byte
	entries := 0
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read %s archive: %w", expected, err)
		}
		entries++
		if entries > maxArchiveEntries {
			return nil, errors.New("archive contains too many entries")
		}
		clean := path.Clean(header.Name)
		if clean != header.Name || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
			return nil, fmt.Errorf("archive entry %q is unsafe", header.Name)
		}
		if header.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("archive entry %q is not a regular file", header.Name)
		}
		if clean != expected || value != nil {
			return nil, fmt.Errorf("archive must contain only %s", expected)
		}
		if header.Size < 0 || header.Size > limit {
			return nil, fmt.Errorf("archive entry %q exceeds %d bytes", header.Name, limit)
		}
		data, err := io.ReadAll(io.LimitReader(tarReader, limit+1))
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > limit {
			return nil, fmt.Errorf("archive entry %q exceeds %d bytes", header.Name, limit)
		}
		value = data
	}
	if value == nil {
		return nil, fmt.Errorf("archive does not contain %s", expected)
	}
	return value, nil
}

// ExtractProfile validates a core profile bundle and its lock.
func ExtractProfile(archive []byte, requireLock bool) (*Profile, error) {
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("read component archive: %w", err)
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	profile := &Profile{
		Components: make(map[string][]byte),
		Digests:    make(map[string]string),
	}
	size := int64(0)
	entries := 0
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read component archive: %w", err)
		}
		entries++
		if entries > maxArchiveEntries {
			return nil, errors.New("component archive contains too many entries")
		}
		clean := path.Clean(header.Name)
		isClean := clean == header.Name || (header.Typeflag == tar.TypeDir && clean+"/" == header.Name)
		if !isClean || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
			return nil, fmt.Errorf("component archive entry %q is unsafe", header.Name)
		}
		switch {
		case clean == "components":
			if header.Typeflag != tar.TypeDir {
				return nil, errors.New("components entry must be a directory")
			}
		case clean == "profile.lock":
			if header.Typeflag != tar.TypeReg || header.Size > maxLockBytes {
				return nil, errors.New("profile.lock is not a bounded regular file")
			}
			data, err := io.ReadAll(io.LimitReader(tarReader, maxLockBytes+1))
			if err != nil {
				return nil, err
			}
			profile.Lock = string(data)
		case strings.HasPrefix(clean, "components/"):
			if header.Typeflag != tar.TypeReg {
				return nil, fmt.Errorf("component entry %q is not a regular file", header.Name)
			}
			name := strings.TrimSuffix(strings.TrimPrefix(clean, "components/"), ".wasm")
			if !safeComponentName(name) || !strings.HasSuffix(clean, ".wasm") {
				return nil, fmt.Errorf("component entry %q has an invalid name", header.Name)
			}
			if _, exists := profile.Components[name]; exists {
				return nil, fmt.Errorf("duplicate component %q", name)
			}
			if header.Size < 0 || header.Size > MaxComponent {
				return nil, fmt.Errorf("component %q exceeds %d bytes", name, MaxComponent)
			}
			data, err := io.ReadAll(io.LimitReader(tarReader, MaxComponent+1))
			if err != nil {
				return nil, err
			}
			if int64(len(data)) > MaxComponent {
				return nil, fmt.Errorf("component %q exceeds %d bytes", name, MaxComponent)
			}
			size += int64(len(data))
			if size > MaxBundle {
				return nil, errors.New("component archive exceeds its size bound")
			}
			profile.Components[name] = data
		default:
			return nil, fmt.Errorf("component archive entry %q is not allowed", header.Name)
		}
	}
	if len(profile.Components) == 0 {
		return nil, errors.New("component archive contains no components")
	}
	if requireLock {
		if profile.Lock == "" {
			return nil, errors.New("component archive is missing profile.lock")
		}
		if err := verifyProfileLock(profile); err != nil {
			return nil, err
		}
	}
	return profile, nil
}

// ParseProfileLock parses the line-oriented profile lock.
func ParseProfileLock(text string) (profileID string, digests map[string]string, err error) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "schema 1" {
		return "", nil, errors.New("profile.lock has an unsupported schema")
	}
	header := strings.Fields(strings.TrimSpace(lines[1]))
	if len(header) != 2 || header[0] != "profile" || header[1] == "" {
		return "", nil, errors.New("profile.lock is missing its profile id")
	}
	digests = make(map[string]string)
	for _, line := range lines[2:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != "component" || !safeComponentName(fields[1]) {
			return "", nil, fmt.Errorf("profile.lock has an invalid entry %q", line)
		}
		if _, err := ParseDigest(fields[2]); err != nil {
			return "", nil, fmt.Errorf("profile.lock entry %q: %w", fields[1], err)
		}
		if _, exists := digests[fields[1]]; exists {
			return "", nil, fmt.Errorf("profile.lock has a duplicate entry %q", fields[1])
		}
		digests[fields[1]] = fields[2]
	}
	if len(digests) == 0 {
		return "", nil, errors.New("profile.lock declares no components")
	}
	return header[1], digests, nil
}

func verifyProfileLock(profile *Profile) error {
	_, digests, err := ParseProfileLock(profile.Lock)
	if err != nil {
		return err
	}
	if len(digests) != len(profile.Components) {
		return errors.New("profile.lock and component archive disagree on the component set")
	}
	for name, data := range profile.Components {
		expected, ok := digests[name]
		if !ok {
			return fmt.Errorf("profile.lock does not declare %s", name)
		}
		if err := VerifyDigest(data, expected); err != nil {
			return fmt.Errorf("component %s: %w", name, err)
		}
		profile.Digests[name] = expected
	}
	return nil
}

func safeComponentName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return name[0] >= 'a' && name[0] <= 'z' || name[0] >= '0' && name[0] <= '9'
}
