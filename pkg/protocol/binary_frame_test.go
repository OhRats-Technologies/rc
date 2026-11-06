package protocol

import (
	"bytes"
	"testing"
)

func TestBinaryFrameRoundTrip(t *testing.T) {
	payload := []byte("streamlined binary control payload")
	seq := uint64(42)
	dir := byte(1)

	encoded, err := EncodeBinaryFrame(dir, seq, payload)
	if err != nil {
		t.Fatalf("EncodeBinaryFrame failed: %v", err)
	}

	if !IsBinaryFrame(encoded) {
		t.Fatal("expected IsBinaryFrame to return true")
	}

	decoded, err := DecodeBinaryFrame(encoded)
	if err != nil {
		t.Fatalf("DecodeBinaryFrame failed: %v", err)
	}

	if decoded.Direction != dir {
		t.Fatalf("direction mismatch: got %d, want %d", decoded.Direction, dir)
	}
	if decoded.Sequence != seq {
		t.Fatalf("sequence mismatch: got %d, want %d", decoded.Sequence, seq)
	}
	if !bytes.Equal(decoded.Payload, payload) {
		t.Fatalf("payload mismatch: got %s, want %s", string(decoded.Payload), string(payload))
	}
}

func TestBinaryFrameRejectsInvalidData(t *testing.T) {
	if IsBinaryFrame([]byte("not a binary frame")) {
		t.Fatal("expected IsBinaryFrame to return false")
	}

	if _, err := DecodeBinaryFrame([]byte("short")); err == nil {
		t.Fatal("expected error decoding truncated frame")
	}

	corrupt := []byte("RC\x01\x01\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x10missing_bytes")
	if _, err := DecodeBinaryFrame(corrupt); err == nil {
		t.Fatal("expected error for frame with truncated payload")
	}
}
