package node

// ProcessOwner returns the authenticated principal and current secure writer
// for one process.
func (m *NativeExecutionManager) ProcessOwner(id string) (string, string, bool) {
	m.mu.RLock()
	p, ok := m.processes[id]
	m.mu.RUnlock()
	if !ok {
		return "", "", false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.spec.UserID, p.attachedSession, true
}
