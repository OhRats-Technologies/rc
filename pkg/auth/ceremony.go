package auth

import (
	"context"
	"database/sql"
	"errors"
)

// Ceremony kinds. A registration ceremony binds a pending account; a login
// ceremony binds only the challenge.
const (
	CeremonyRegistration = "registration"
	CeremonyLogin        = "login"
)

// PutCeremony stores single-use ceremony state until expiresMs.
func (s *Store) PutCeremony(id string, kind string, userID string, state []byte, expiresMs int64) error {
	if id == "" || kind == "" || len(state) == 0 {
		return ErrInvalidCeremony
	}
	var owner any
	if userID != "" {
		owner = userID
	}
	_, err := s.db.ExecContext(
		context.Background(),
		`INSERT INTO ceremonies (id, kind, user_id, state, expires_ms) VALUES (?, ?, ?, ?, ?);`,
		id,
		kind,
		owner,
		state,
		expiresMs,
	)
	return err
}

// TakeCeremony consumes a ceremony exactly once. It reports ok=false when the
// ceremony is unknown, already used, or expired; expired state is never
// returned.
func (s *Store) TakeCeremony(
	id string,
	kind string,
) (state []byte, userID string, ok bool, err error) {
	var owner sql.NullString
	var expiresMs int64
	err = s.db.QueryRowContext(
		context.Background(),
		`DELETE FROM ceremonies WHERE id = ? AND kind = ?
		 RETURNING user_id, state, expires_ms;`,
		id,
		kind,
	).Scan(&owner, &state, &expiresMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, err
	}
	if expiresMs <= nowMs() {
		return nil, "", false, nil
	}
	return state, owner.String, true, nil
}
