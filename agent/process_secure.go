package main

const secureBacklogLimit = 1 << 20

func (manager *processManager) queueSecure(process *managedProcess, message wireMessage) {
	size := messageSize(message)
	manager.mu.Lock()
	for process.backlogBytes+size > secureBacklogLimit && len(process.backlog) > 0 {
		process.backlogBytes -= messageSize(process.backlog[0])
		process.backlog = process.backlog[1:]
	}
	if size <= secureBacklogLimit {
		process.backlog = append(process.backlog, message)
		process.backlogBytes += size
	}
	manager.mu.Unlock()
}

func (manager *processManager) emit(process *managedProcess, message wireMessage) {
	if process == nil || !process.secure {
		manager.sendMessage(message)
		return
	}
	manager.mu.Lock()
	send, sessionID := manager.secureSend, process.sessionID
	manager.mu.Unlock()
	if send != nil && sessionID != "" && send(sessionID, message) {
		return
	}
	manager.queueSecure(process, message)
}

func (manager *processManager) setSecureSender(send func(string, wireMessage) bool) {
	manager.mu.Lock()
	manager.secureSend = send
	manager.mu.Unlock()
}

func (manager *processManager) attachSecure(process *managedProcess, sessionID string) {
	manager.mu.Lock()
	process.sessionID = sessionID
	backlog := append([]wireMessage(nil), process.backlog...)
	process.backlog = nil
	process.backlogBytes = 0
	manager.mu.Unlock()
	for _, message := range backlog {
		manager.emit(process, message)
	}
}

func (manager *processManager) detachSecureSession(sessionID string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	for _, process := range manager.processes {
		if process.secure && process.sessionID == sessionID {
			process.sessionID = ""
		}
	}
}

func (manager *processManager) secureHandle(sessionID, userID, role string, message wireMessage) {
	switch message.Type {
	case "process.attach", "process.input", "process.resize", "process.signal":
		manager.mu.Lock()
		process := manager.processes[message.ID]
		allowed := process != nil && (role == "owner" || process.userID == userID)
		manager.mu.Unlock()
		if !allowed {
			return
		}
		switch message.Type {
		case "process.attach":
			manager.attachSecure(process, sessionID)
		case "process.input":
			manager.input(message.ID, message.Input)
		case "process.resize":
			manager.resize(message.ID, message.Cols, message.Rows)
		case "process.signal":
			manager.signal(message.ID, message.Signal)
		}
	}
}
