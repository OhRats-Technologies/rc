package protocol

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

type Fragment struct {
	Text string `json:"rcFragment"`
	Last bool   `json:"last"`
}

// Encode breaks a message larger than WireLimit into ordered JSON fragments.
func Encode(message string) ([]string, error) {
	if len(message) > NodeControlMessageLimit {
		return nil, errors.New("Node control message exceeds transport frame capacity")
	}
	if len(message) <= WireLimit {
		return []string{message}, nil
	}

	var result []string
	start := 0
	msgLen := len(message)
	for start < msgLen {
		end := start + FragmentBytes
		if end > msgLen {
			end = msgLen
		}
		if end < msgLen {
			for end > start && !utf8.RuneStart(message[end]) {
				end--
			}
		}
		frag := Fragment{
			Text: message[start:end],
			Last: end == msgLen,
		}
		encoded, err := json.Marshal(frag)
		if err != nil {
			return nil, err
		}
		result = append(result, string(encoded))
		start = end
	}
	return result, nil
}

// Decoder reassembles ordered JSON fragments.
type Decoder struct {
	pending strings.Builder
}

// Accept processes one raw wire frame. Returns (completed_bytes, done, error).
func (d *Decoder) Accept(b []byte) ([]byte, bool, error) {
	if len(b) > WireLimit {
		d.pending.Reset()
		return nil, false, errors.New("Node wire frame exceeds capacity")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, false, err
	}

	fragData, isFrag := raw["rcFragment"]
	if !isFrag {
		if d.pending.Len() > 0 {
			d.pending.Reset()
			return nil, false, errors.New("interleaved Node fragments")
		}
		return b, true, nil
	}

	var frag Fragment
	if err := json.Unmarshal(b, &frag); err != nil {
		return nil, false, err
	}
	_ = fragData

	if d.pending.Len()+len(frag.Text) > NodeControlMessageLimit {
		d.pending.Reset()
		return nil, false, errors.New("Node reassembled frame exceeds capacity")
	}

	d.pending.WriteString(frag.Text)
	if frag.Last {
		res := []byte(d.pending.String())
		d.pending.Reset()
		return res, true, nil
	}
	return nil, false, nil
}
