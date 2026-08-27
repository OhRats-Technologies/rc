package main

const secureScrollbackLimit = 4 << 20

func secureScrollbackMessage(message wireMessage) bool {
	return message.Type == "process.stdout" || message.Type == "process.stderr"
}

func (manager *processManager) rememberSecureOutputLocked(process *managedProcess, message wireMessage) {
	if !secureScrollbackMessage(message) {
		return
	}
	size := messageSize(message)
	for process.scrollbackBytes+size > secureScrollbackLimit && len(process.scrollback) > 0 {
		process.scrollbackBytes -= messageSize(process.scrollback[0])
		process.scrollback = process.scrollback[1:]
	}
	if size <= secureScrollbackLimit {
		process.scrollback = append(process.scrollback, message)
		process.scrollbackBytes += size
	}
}

func (manager *processManager) emit(process *managedProcess, message wireMessage) {
	if process == nil || !process.secure {
		manager.sendMessage(message)
		return
	}
	if message.Type == "process.started" {
		manager.sendMessage(wireMessage{Type: "process.started", ID: message.ID})
	}
	if message.Type == "process.exit" {
		manager.sendMessage(wireMessage{Type: "process.exit", ID: message.ID, ExitCode: message.ExitCode, Signal: message.Signal})
	}
	manager.mu.Lock()
	manager.rememberSecureOutputLocked(process, message)
	send, sessionID := manager.secureSend, process.sessionID
	manager.mu.Unlock()
	if send != nil && sessionID != "" {
		_ = send(sessionID, message)
	}
}

func (manager *processManager) setSecureSender(send func(string, wireMessage) bool) {
	manager.mu.Lock()
	manager.secureSend = send
	manager.mu.Unlock()
}

func (manager *processManager) attachSecure(process *managedProcess, sessionID string) {
	manager.mu.Lock()
	process.sessionID = sessionID
	send := manager.secureSend
	if send != nil {
		for _, message := range process.scrollback {
			if !send(sessionID, message) {
				process.sessionID = ""
				break
			}
		}
	}
	manager.mu.Unlock()
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
	case "process.attach", "process.stdin", "process.stdin.close", "process.resize", "process.signal":
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
		case "process.stdin":
			manager.input(message.ID, message.Data)
		case "process.stdin.close":
			manager.closeInput(message.ID)
		case "process.resize":
			manager.resize(message.ID, message.Cols, message.Rows)
		case "process.signal":
			manager.signal(message.ID, message.Signal)
		}
	}
}
