package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLargeUnicodeFramesRoundTrip(t *testing.T) {
	data := strings.Repeat("雪\\\"", 30000)
	msgMap := map[string]string{"data": data}
	msgBytes, err := json.Marshal(msgMap)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	message := string(msgBytes)

	frames, err := Encode(message)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if len(frames) <= 1 {
		t.Fatalf("expected multiple frames, got %d", len(frames))
	}

	decoder := &Decoder{}
	for index, frame := range frames {
		if len(frame) >= WireLimit {
			t.Fatalf("frame %d exceeds WireLimit (%d >= %d)", index, len(frame), WireLimit)
		}
		result, done, err := decoder.Accept([]byte(frame))
		if err != nil {
			t.Fatalf("Accept failed at frame %d: %v", index, err)
		}
		if index+1 == len(frames) {
			if !done || string(result) != message {
				t.Fatalf("reassembled message mismatch at final frame")
			}
		} else {
			if done {
				t.Fatalf("done prematurely at frame %d", index)
			}
		}
	}

	// Interleaving non-fragment message while fragment pending should fail
	_, _, err = decoder.Accept([]byte(frames[0]))
	if err != nil {
		t.Fatalf("Accept first frame failed: %v", err)
	}
	_, _, err = decoder.Accept([]byte("{}"))
	if err == nil {
		t.Fatal("expected error on interleaved non-fragment message")
	}

	// Message exceeding capacity should fail Encode
	huge := strings.Repeat("x", NodeControlMessageLimit+1)
	if _, err := Encode(huge); err == nil {
		t.Fatal("expected Encode to fail for message exceeding NodeControlMessageLimit")
	}
}
