package protocol

import (
	"encoding/binary"
	"errors"
)

const (
	BinaryFrameMagic byte = 0x52 // 'R'
	BinaryFrameSub   byte = 0x43 // 'C'
	BinaryFrameVer1  byte = 0x01 // version 1
	BinaryHeaderLen       = 16   // 3 magic + 1 direction + 8 seq + 4 len (16 bytes)
)

var (
	ErrBinaryFrameInvalidHeader = errors.New("invalid binary frame header")
	ErrBinaryFrameTooLarge      = errors.New("binary frame exceeds capacity")
	ErrBinaryFrameTruncated     = errors.New("binary frame truncated")
)

// BinaryFrame represents one streamlined binary-framed control envelope.
type BinaryFrame struct {
	Direction byte
	Sequence  uint64
	Payload   []byte
}

// EncodeBinaryFrame serializes a binary control frame.
func EncodeBinaryFrame(direction byte, seq uint64, payload []byte) ([]byte, error) {
	if len(payload) > NodeControlMessageLimit {
		return nil, ErrBinaryFrameTooLarge
	}
	buf := make([]byte, BinaryHeaderLen+len(payload))
	buf[0] = BinaryFrameMagic
	buf[1] = BinaryFrameSub
	buf[2] = BinaryFrameVer1
	buf[3] = direction
	binary.BigEndian.PutUint64(buf[4:12], seq)
	binary.BigEndian.PutUint32(buf[12:16], uint32(len(payload)))
	copy(buf[16:], payload)
	return buf, nil
}

// DecodeBinaryFrame parses one binary control frame.
func DecodeBinaryFrame(data []byte) (*BinaryFrame, error) {
	if len(data) < 16 {
		return nil, ErrBinaryFrameTruncated
	}
	if data[0] != BinaryFrameMagic || data[1] != BinaryFrameSub || data[2] != BinaryFrameVer1 {
		return nil, ErrBinaryFrameInvalidHeader
	}
	direction := data[3]
	seq := binary.BigEndian.Uint64(data[4:12])
	payloadLen := binary.BigEndian.Uint32(data[12:16])
	if int(payloadLen) > NodeControlMessageLimit {
		return nil, ErrBinaryFrameTooLarge
	}
	if len(data[16:]) < int(payloadLen) {
		return nil, ErrBinaryFrameTruncated
	}
	payload := make([]byte, payloadLen)
	copy(payload, data[16:16+payloadLen])
	return &BinaryFrame{
		Direction: direction,
		Sequence:  seq,
		Payload:   payload,
	}, nil
}

// IsBinaryFrame checks if the raw bytes start with the RC binary frame magic.
func IsBinaryFrame(data []byte) bool {
	return len(data) >= 3 && data[0] == BinaryFrameMagic && data[1] == BinaryFrameSub && data[2] == BinaryFrameVer1
}
