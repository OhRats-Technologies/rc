package main

import "time"

type pendingSecureStart struct {
	sessionID string
	userID    string
	message   wireMessage
	expires   time.Time
}

func pendingStartKey(processID, userID string) string { return processID + "\x00" + userID }

func (manager *controlManager) queueSecureStart(sessionID, userID string, message wireMessage) {
	if message.ID == "" || message.Command == "" {
		return
	}
	now := time.Now()
	key := pendingStartKey(message.ID, userID)
	manager.mu.Lock()
	for id, pending := range manager.pendingStarts {
		if pending.expires.Before(now) {
			delete(manager.pendingStarts, id)
		}
	}
	manager.pendingStarts[key] = pendingSecureStart{sessionID: sessionID, userID: userID, message: message, expires: now.Add(15 * time.Second)}
	manager.mu.Unlock()
	if manager.send(wireMessage{Type: "process.start.request", ID: message.ID, UserID: userID}) != nil {
		manager.mu.Lock()
		delete(manager.pendingStarts, key)
		manager.mu.Unlock()
	}
}

func (manager *controlManager) permitSecureStart(message wireMessage) {
	key := pendingStartKey(message.ID, message.UserID)
	manager.mu.Lock()
	pending, ok := manager.pendingStarts[key]
	if ok {
		delete(manager.pendingStarts, key)
	}
	_, sessionAlive := manager.sessions[pending.sessionID]
	manager.mu.Unlock()
	if !ok || !sessionAlive || pending.expires.Before(time.Now()) {
		return
	}
	manager.processes.start(pending.message, pending.sessionID, pending.userID)
}

func (manager *controlManager) discardPendingSession(sessionID string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	for key, pending := range manager.pendingStarts {
		if pending.sessionID == sessionID {
			delete(manager.pendingStarts, key)
		}
	}
}
