package main

const maxControlPlaintext = 1 << 20
const maxControlCiphertext = 1_500_000
const maxControlFrameBytes = 2 << 20
const maxWireMessageBytes = 4 << 20

type controlTransport interface {
	sendFrame(deviceID, sessionID string, sequence uint64, ciphertext string) error
	readFrame(sessionID string) (uint64, string, error)
	close() error
}
