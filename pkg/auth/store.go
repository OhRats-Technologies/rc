package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/platform"
)

const (
	sqliteConstraintUnique     = 2067
	sqliteConstraintForeignKey = 787
)

// Store is the SQLite-backed account, passkey, ceremony, and session store.
type Store struct {
	db *platform.DB
}

// Open opens or creates a store at path. Empty databases receive the current
// schema; databases at schemaVersion are migrated in place. Legacy databases
// are rejected here and handled only by OpenDataDir.
func Open(path string) (*Store, error) {
	db, err := platform.OpenDB(path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

// Close releases the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// UserCount reports how many accounts exist.
func (s *Store) UserCount() (int, error) {
	var count int
	err := s.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM users;`).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CreateUser creates an account with a random identifier.
func (s *Store) CreateUser(name string) (*User, error) {
	id, err := randomToken(16)
	if err != nil {
		return nil, err
	}
	return s.CreateUserWithID(id, name)
}

// CreateUserWithID creates an account with a caller-selected identifier. The
// identifier must match a pending registration ceremony.
func (s *Store) CreateUserWithID(id, name string) (*User, error) {
	displayName, err := normalizeName(name)
	if err != nil {
		return nil, err
	}
	if id == "" || len(id) > 128 {
		return nil, ErrInvalidName
	}
	createdAt := nowMs()
	_, err = s.db.ExecContext(
		context.Background(),
		`INSERT INTO users (id, name, created_ms) VALUES (?, ?, ?);`,
		id,
		displayName,
		createdAt,
	)
	if err != nil {
		return nil, err
	}
	return &User{ID: id, Name: displayName, CreatedAt: timeFromMs(createdAt)}, nil
}

// GetUser returns one account.
func (s *Store) GetUser(id string) (*User, error) {
	var name string
	var createdAt int64
	err := s.db.QueryRowContext(
		context.Background(),
		`SELECT name, created_ms FROM users WHERE id = ?;`,
		id,
	).Scan(&name, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &User{ID: id, Name: name, CreatedAt: timeFromMs(createdAt)}, nil
}

// UpdateUserName changes an account display name.
func (s *Store) UpdateUserName(id, name string) (*User, error) {
	displayName, err := normalizeName(name)
	if err != nil {
		return nil, err
	}
	result, err := s.db.ExecContext(
		context.Background(),
		`UPDATE users SET name = ? WHERE id = ?;`,
		displayName,
		id,
	)
	if err != nil {
		return nil, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return nil, ErrUserNotFound
	}
	return s.GetUser(id)
}

// AddPasskey registers a WebAuthn credential for an account.
func (s *Store) AddPasskey(
	userID string,
	name string,
	credentialID []byte,
	publicKey []byte,
	algorithm int64,
) (*Passkey, error) {
	passkeyName, err := normalizeName(name)
	if err != nil {
		return nil, err
	}
	if len(credentialID) == 0 || len(credentialID) > maxCredentialBytes || len(publicKey) == 0 {
		return nil, ErrInvalidCredential
	}
	if _, err := s.GetUser(userID); err != nil {
		return nil, err
	}
	id, err := randomToken(18)
	if err != nil {
		return nil, err
	}
	createdAt := nowMs()
	_, err = s.db.ExecContext(
		context.Background(),
		`INSERT INTO passkeys (id, user_id, name, credential_id, public_key, algorithm, created_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?);`,
		id,
		userID,
		passkeyName,
		credentialID,
		publicKey,
		algorithm,
		createdAt,
	)
	if err != nil {
		if isConstraint(err, sqliteConstraintUnique) {
			return nil, ErrCredentialRegistered
		}
		return nil, err
	}
	return &Passkey{
		ID:           id,
		UserID:       userID,
		Name:         passkeyName,
		CredentialID: copyBytes(credentialID),
		PublicKey:    copyBytes(publicKey),
		Algorithm:    algorithm,
		CreatedAt:    timeFromMs(createdAt),
	}, nil
}

// PasskeysForUser lists an account's credentials in registration order.
func (s *Store) PasskeysForUser(userID string) ([]Passkey, error) {
	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT id, user_id, name, credential_id, public_key, algorithm, created_ms, last_used_ms
		 FROM passkeys WHERE user_id = ? ORDER BY created_ms, id;`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Passkey
	for rows.Next() {
		passkey, err := scanPasskey(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, passkey)
	}
	return result, rows.Err()
}

// PasskeyByCredentialID looks a credential up by its WebAuthn credential ID.
func (s *Store) PasskeyByCredentialID(credentialID []byte) (*Passkey, error) {
	row := s.db.QueryRowContext(
		context.Background(),
		`SELECT id, user_id, name, credential_id, public_key, algorithm, created_ms, last_used_ms
		 FROM passkeys WHERE credential_id = ?;`,
		credentialID,
	)
	passkey, err := scanPasskey(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPasskeyNotFound
	}
	if err != nil {
		return nil, err
	}
	return &passkey, nil
}

// UpdatePasskeyLastUsed records a successful authentication.
func (s *Store) UpdatePasskeyLastUsed(passkeyID string, at time.Time) error {
	result, err := s.db.ExecContext(
		context.Background(),
		`UPDATE passkeys SET last_used_ms = ? WHERE id = ?;`,
		msFromTime(at),
		passkeyID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrPasskeyNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPasskey(row scanner) (Passkey, error) {
	var passkey Passkey
	var createdAt int64
	var lastUsed sql.NullInt64
	if err := row.Scan(
		&passkey.ID,
		&passkey.UserID,
		&passkey.Name,
		&passkey.CredentialID,
		&passkey.PublicKey,
		&passkey.Algorithm,
		&createdAt,
		&lastUsed,
	); err != nil {
		return Passkey{}, err
	}
	passkey.CreatedAt = timeFromMs(createdAt)
	if lastUsed.Valid {
		value := timeFromMs(lastUsed.Int64)
		passkey.LastUsedAt = &value
	}
	return passkey, nil
}

// isConstraint reports whether err is a SQLite constraint failure with the
// given extended result code.
func isConstraint(err error, code int) bool {
	var sqliteError interface{ Code() int }
	if !errors.As(err, &sqliteError) {
		return false
	}
	return sqliteError.Code() == code
}

func copyBytes(value []byte) []byte {
	result := make([]byte, len(value))
	copy(result, value)
	return result
}
