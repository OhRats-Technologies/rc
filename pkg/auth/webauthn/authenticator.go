package webauthn

import "encoding/binary"

const (
	maxAuthenticatorDataBytes = 128 * 1024
	maxCredentialIDBytes      = 1023

	flagUserPresent            byte = 0x01
	flagUserVerified           byte = 0x04
	flagBackupEligible         byte = 0x08
	flagBackupState            byte = 0x10
	flagAttestedCredentialData byte = 0x40
	flagExtensionData          byte = 0x80
)

type attestedCredentialData struct {
	aaguid       []byte
	credentialID []byte
	publicKey    publicKey
}

type authenticatorData struct {
	rpIDHash  []byte
	flags     byte
	signCount uint32
	attested  *attestedCredentialData
}

type sliceCursor struct {
	data []byte
	off  int
}

func (c *sliceCursor) take(length int) ([]byte, bool) {
	if length < 0 || len(c.data)-c.off < length {
		return nil, false
	}
	raw := c.data[c.off : c.off+length]
	c.off += length
	return raw, true
}

func (c *sliceCursor) remaining() []byte {
	return c.data[c.off:]
}

func (c *sliceCursor) skip(length int) bool {
	if length < 0 || len(c.data)-c.off < length {
		return false
	}
	c.off += length
	return true
}

func parseAuthenticatorData(raw []byte) (authenticatorData, error) {
	if len(raw) > maxAuthenticatorDataBytes {
		return authenticatorData{}, ErrPayloadTooLarge
	}
	cursor := &sliceCursor{data: raw}
	rpIDHash, ok := cursor.take(32)
	if !ok {
		return authenticatorData{}, ErrInvalidAuthenticatorData
	}
	rawFlags, ok := cursor.take(1)
	if !ok {
		return authenticatorData{}, ErrInvalidAuthenticatorData
	}
	flags := rawFlags[0]
	if flags&flagBackupState != 0 && flags&flagBackupEligible == 0 {
		return authenticatorData{}, ErrInvalidAuthenticatorData
	}
	rawSignCount, ok := cursor.take(4)
	if !ok {
		return authenticatorData{}, ErrInvalidAuthenticatorData
	}
	result := authenticatorData{
		rpIDHash:  copyBytes(rpIDHash),
		flags:     flags,
		signCount: binary.BigEndian.Uint32(rawSignCount),
	}
	if flags&flagAttestedCredentialData != 0 {
		attested, err := parseAttestedCredentialData(cursor, flags)
		if err != nil {
			return authenticatorData{}, err
		}
		result.attested = attested
	}
	if flags&flagExtensionData != 0 {
		if err := validateExtensionData(cursor.remaining()); err != nil {
			return authenticatorData{}, err
		}
	} else if len(cursor.remaining()) != 0 {
		return authenticatorData{}, ErrInvalidAuthenticatorData
	}
	return result, nil
}

func parseAttestedCredentialData(cursor *sliceCursor, flags byte) (*attestedCredentialData, error) {
	aaguid, ok := cursor.take(16)
	if !ok {
		return nil, ErrInvalidAuthenticatorData
	}
	rawLength, ok := cursor.take(2)
	if !ok {
		return nil, ErrInvalidAuthenticatorData
	}
	length := int(binary.BigEndian.Uint16(rawLength))
	credentialID, ok := cursor.take(length)
	if !ok {
		return nil, ErrInvalidAuthenticatorData
	}
	if len(credentialID) == 0 {
		return nil, ErrEmptyCredentialID
	}
	if len(credentialID) > maxCredentialIDBytes {
		return nil, ErrCredentialIDTooLarge
	}
	key, consumed, err := parseCOSEKeyPrefix(cursor.remaining())
	if err != nil {
		return nil, err
	}
	if !cursor.skip(consumed) {
		return nil, ErrInvalidAuthenticatorData
	}
	if flags&flagExtensionData == 0 && len(cursor.remaining()) != 0 {
		return nil, ErrInvalidAuthenticatorData
	}
	return &attestedCredentialData{
		aaguid:       copyBytes(aaguid),
		credentialID: copyBytes(credentialID),
		publicKey:    key,
	}, nil
}

func validateExtensionData(input []byte) error {
	if len(input) == 0 {
		return ErrInvalidAuthenticatorData
	}
	value, err := decodeCBORValue(input)
	if err != nil {
		return ErrInvalidAuthenticatorData
	}
	if _, ok := value.(cborMap); !ok {
		return ErrInvalidAuthenticatorData
	}
	return nil
}

func (data authenticatorData) userPresent() bool {
	return data.flags&flagUserPresent != 0
}

func (data authenticatorData) userVerified() bool {
	return data.flags&flagUserVerified != 0
}

func (data authenticatorData) backupEligible() bool {
	return data.flags&flagBackupEligible != 0
}

func (data authenticatorData) backupState() bool {
	return data.flags&flagBackupState != 0
}

func copyBytes(value []byte) []byte {
	result := make([]byte, len(value))
	copy(result, value)
	return result
}
