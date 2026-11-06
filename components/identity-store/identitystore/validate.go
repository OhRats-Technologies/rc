package identitystore

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

const (
	maxIDBytes       = 128
	maxKindBytes     = 64
	maxNameBytes     = 120
	maxMetadataBytes = 64 * 1024
	maxStateBytes    = 512 * 1024
)

func validID(value string, label string) error {
	if value == "" || len(value) > maxIDBytes {
		return fmt.Errorf("invalid %s", label)
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if isASCIIAlphanumeric(character) || character == '.' || character == '-' ||
			character == '_' || character == ':' {
			continue
		}
		return fmt.Errorf("invalid %s", label)
	}
	return nil
}

func isASCIIAlphanumeric(character byte) bool {
	return (character >= 'a' && character <= 'z') ||
		(character >= 'A' && character <= 'Z') ||
		(character >= '0' && character <= '9')
}

func validKind(value string) error {
	if value == "" || len(value) > maxKindBytes {
		return errors.New("invalid ceremony kind")
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			character == '-' {
			continue
		}
		return errors.New("invalid ceremony kind")
	}
	return nil
}

func validDisplayName(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > maxNameBytes || hasControl(value) {
		return errors.New("invalid user display name")
	}
	return nil
}

func validPasskeyName(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > maxNameBytes || hasControl(value) {
		return errors.New("invalid passkey name")
	}
	return nil
}

func hasControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func validCeremonyPayload(metadata []byte, state []byte) error {
	if len(metadata) > maxMetadataBytes {
		return errors.New("ceremony metadata is too large")
	}
	if len(state) == 0 || len(state) > maxStateBytes {
		return errors.New("invalid ceremony state")
	}
	return nil
}

func validCredentialID(id []byte) error {
	if len(id) == 0 || len(id) > 1024 {
		return errors.New("invalid WebAuthn credential id")
	}
	return nil
}

func validStoredCredential(id []byte, algorithm string, publicKeyCose []byte) error {
	if err := validCredentialID(id); err != nil {
		return err
	}
	if algorithm == "" || len(algorithm) > 32 {
		return errors.New("invalid WebAuthn algorithm")
	}
	for index := 0; index < len(algorithm); index++ {
		character := algorithm[index]
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			character == '-' {
			continue
		}
		return errors.New("invalid WebAuthn algorithm")
	}
	if len(publicKeyCose) == 0 || len(publicKeyCose) > 16*1024 {
		return errors.New("invalid WebAuthn public key")
	}
	return nil
}
