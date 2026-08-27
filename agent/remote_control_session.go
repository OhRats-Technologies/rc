package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
)

type x25519Pair struct{ Private, Public string }

func generateX25519() (x25519Pair, error) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return x25519Pair{}, err
	}
	return x25519Pair{Private: base64.RawURLEncoding.EncodeToString(privateKey.Bytes()), Public: base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes())}, nil
}

func (control *remoteControl) send(message wireMessage) error {
	plaintext, _ := json.Marshal(message)
	if len(plaintext) > maxControlPlaintext {
		return errors.New("control message is too large")
	}
	control.sendSeq++
	sequence := control.sendSeq
	ciphertext := control.aead.Seal(nil, frameNonce(1, sequence), plaintext, frameAAD(control.sessionID, sequence, "c2n"))
	return control.transport.sendFrame(control.deviceID, control.sessionID, sequence, base64.RawURLEncoding.EncodeToString(ciphertext))
}

func (control *remoteControl) read() (wireMessage, error) {
	next, encoded, err := control.transport.readFrame(control.sessionID)
	if err != nil {
		return wireMessage{}, err
	}
	if next != control.recvSeq+1 {
		return wireMessage{}, errors.New("invalid encrypted frame sequence")
	}
	if len(encoded) == 0 || len(encoded) > maxControlCiphertext {
		return wireMessage{}, errors.New("invalid encrypted control frame")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return wireMessage{}, err
	}
	plaintext, err := control.aead.Open(nil, frameNonce(2, next), ciphertext, frameAAD(control.sessionID, next, "n2c"))
	if err != nil {
		return wireMessage{}, errors.New("encrypted frame authentication failed")
	}
	control.recvSeq = next
	var message wireMessage
	if err := json.Unmarshal(plaintext, &message); err != nil {
		return wireMessage{}, err
	}
	if message.Type == "control.revoked" {
		return wireMessage{}, errors.New("control authorization changed; reopen the session")
	}
	return message, nil
}

func (control *remoteControl) request(message wireMessage) error {
	message.RequestID = randomURLBytes(12)
	if err := control.send(message); err != nil {
		return err
	}
	for {
		response, err := control.read()
		if err != nil {
			return err
		}
		if response.Type == "control.result" && response.RequestID == message.RequestID {
			if response.Output == "ok" {
				return nil
			}
			return errors.New(response.Output)
		}
	}
}

func (control *remoteControl) close() {
	_ = control.transport.close()
	resp, err := accountRequest(control.server, control.token, "DELETE", "/api/v1/control/"+url.PathEscape(control.sessionID))
	if err == nil {
		_ = resp.Body.Close()
	}
}

func waitForProcess(control *remoteControl, processID string) error {
	stdinClosed := false
	for {
		message, err := control.read()
		if err != nil {
			return err
		}
		if message.ID != processID {
			continue
		}
		if message.Type == "process.started" && !stdinClosed {
			if err := control.send(wireMessage{Type: "process.stdin.close", ID: processID}); err != nil {
				return err
			}
			stdinClosed = true
		}
		if message.Type == "process.stdout" || message.Type == "process.stderr" {
			data, decodeErr := base64.RawURLEncoding.DecodeString(message.Data)
			if decodeErr != nil {
				return decodeErr
			}
			writer := io.Writer(os.Stdout)
			if message.Type == "process.stderr" {
				writer = os.Stderr
			}
			if _, err := writer.Write(data); err != nil {
				return err
			}
		}
		if message.Type == "process.exit" {
			if message.ExitCode != nil && *message.ExitCode != 0 {
				return fmt.Errorf("process exited %d", *message.ExitCode)
			}
			return nil
		}
	}
}
