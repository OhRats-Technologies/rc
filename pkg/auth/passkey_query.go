package auth

import "context"

// AllPasskeys returns every registered credential, used to build login
// allow-lists for discoverable passkeys.
func (s *Store) AllPasskeys() ([]Passkey, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT id, user_id, name, credential_id, public_key, algorithm, created_ms, last_used_ms FROM passkeys ORDER BY created_ms;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []Passkey
	for rows.Next() {
		value, err := scanPasskey(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}
