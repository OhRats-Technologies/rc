package server

func (s *Server) ensureDefaultAuthority() error {
	if s.auth == nil {
		return nil
	}
	if err := s.auth.EnsureWorkspace(defaultWorkspaceID, "Personal"); err != nil {
		return err
	}
	users, err := s.auth.ListUsers()
	if err != nil {
		return err
	}
	for _, user := range users {
		if err := s.auth.EnsureWorkspaceMembership(defaultWorkspaceID, user.ID, "owner"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) ensureDefaultMembership(userID string) error {
	if s.auth == nil {
		return nil
	}
	if err := s.auth.EnsureWorkspace(defaultWorkspaceID, "Personal"); err != nil {
		return err
	}
	return s.auth.EnsureWorkspaceMembership(defaultWorkspaceID, userID, "owner")
}

// Close releases the account store and tears down live execution trees.
