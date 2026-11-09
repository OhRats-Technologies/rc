package server

import "net/http"

// handleAccountDelete removes the authenticated human account after a fresh
// passkey ceremony. Owned workspaces and their devices are deleted; non-owned
// memberships cascade through the account foreign key.
func (s *Server) handleAccountDelete(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if err := s.consumeStepUp(req, user.ID); err != nil {
		writeError(w, http.StatusUnauthorized, "fresh passkey verification required")
		return
	}
	devices, err := s.auth.DeleteAccount(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "account could not be deleted")
		return
	}
	for _, deviceID := range devices {
		s.registry.forget(deviceID)
		s.nodes.Disconnect(deviceID)
		s.control.releaseDevice(deviceID)
	}
	clearSessionCookie(w, sessionCookieName)
	clearSessionCookie(w, stepUpCookieName)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "setupRequired": s.userCount() == 0,
	})
}
