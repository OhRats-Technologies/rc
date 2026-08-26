package main

import (
	"errors"
	"sync"

	"github.com/gorilla/websocket"
)

type controlTransport interface {
	sendFrame(deviceID, sessionID string, sequence uint64, ciphertext string) error
	readFrame(sessionID string) (uint64, string, error)
	close() error
}

type websocketControlTransport struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func (transport *websocketControlTransport) sendFrame(deviceID, sessionID string, sequence uint64, ciphertext string) error {
	return writeJSON(transport.conn, &transport.writeMu, map[string]any{"type": "control.frame", "deviceId": deviceID,
		"sessionId": sessionID, "sequence": sequence, "ciphertext": ciphertext})
}

func (transport *websocketControlTransport) readFrame(sessionID string) (uint64, string, error) {
	for {
		var outer map[string]any
		if err := transport.conn.ReadJSON(&outer); err != nil {
			return 0, "", err
		}
		if outer["type"] != "control.frame" || outer["sessionId"] != sessionID {
			continue
		}
		sequence, ok := outer["sequence"].(float64)
		ciphertext, cipherOK := outer["ciphertext"].(string)
		if !ok || !cipherOK {
			return 0, "", errors.New("invalid control frame")
		}
		return uint64(sequence), ciphertext, nil
	}
}

func (transport *websocketControlTransport) close() error { return transport.conn.Close() }
