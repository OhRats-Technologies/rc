package main

import (
	"encoding/json"
	"errors"
)

func verifySshProcessStart(stateDir string, message wireMessage) (string, error) {
	lock, err := loadLock(stateDir)
	if err != nil {
		return "", errors.New("RC Lock is not initialized")
	}
	var snapshot authoritySnapshot
	if json.Unmarshal([]byte(lock.Snapshot), &snapshot) != nil {
		return "", errors.New("invalid RC Lock state")
	}
	proof := controlProof{Grant: message.Grant, CredentialID: message.CredentialID, Assertion: message.Assertion}
	grant, role, err := verifyControlProof(snapshot, proof, lock.Origin, lock.RPID)
	if err != nil || grant.UserID != message.UserID || role == "viewer" {
		return "", errors.New("SSH execution requires an active Operator or Owner control grant")
	}
	if message.ID == "" || message.SessionID == "" {
		return "", errors.New("invalid SSH process session")
	}
	return grant.UserID, nil
}

func handleSshProcess(stateDir string, manager *processManager, message wireMessage) error {
	if message.Type == "ssh.process.start" {
		userID, err := verifySshProcessStart(stateDir, message)
		if err != nil {
			return err
		}
		manager.startSsh(message, userID, message.SessionID)
		return nil
	}
	manager.mu.Lock()
	process := manager.processes[message.ID]
	valid := process != nil && process.edgeSession != "" && process.edgeSession == message.SessionID
	manager.mu.Unlock()
	if !valid {
		return errors.New("SSH process session rejected")
	}
	switch message.Type {
	case "ssh.process.stdin":
		manager.input(message.ID, message.Data)
	case "ssh.process.stdin.close":
		manager.closeInput(message.ID)
	case "ssh.process.resize":
		manager.resize(message.ID, message.Cols, message.Rows)
	case "ssh.process.signal":
		manager.signal(message.ID, message.Signal)
	default:
		return errors.New("unsupported SSH process command")
	}
	return nil
}
