package auth

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
)

// v2Credential is the webauthn-rs credential document the Rust-era database
// stored per passkey.
type v2Credential struct {
	Cred struct {
		CredID string `json:"cred_id"`
		Cred   struct {
			Type string `json:"type_"`
			Key  struct {
				EC2 *struct {
					Curve string `json:"curve"`
					X     string `json:"x"`
					Y     string `json:"y"`
				} `json:"EC_EC2"`
			} `json:"key"`
		} `json:"cred"`
	} `json:"cred"`
}

// readV2 reads the Rust-era rc-v2.sqlite3 database. Sessions, pending
// ceremonies, and enrollment tokens are not imported; they are short-lived.
func readV2(path string, data *legacyData) error {
	db, err := openReadOnly(path)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx := context.Background()
	users, err := readUsers(ctx, db, `SELECT id, name, created_at FROM users ORDER BY id;`)
	if err != nil {
		return err
	}
	data.users = append(data.users, users...)
	passkeys, err := readV2Passkeys(ctx, db)
	if err != nil {
		return err
	}
	data.passkeys = append(data.passkeys, passkeys...)
	if err := readWorkspaces(ctx, db, data); err != nil {
		return err
	}
	devices, err := readDevices(ctx, db, `SELECT id, workspace_id, name, hostname, platform, arch,
		version, capabilities, identity_public_key, transport_public_key, lock_hash, lock_generation,
		last_seen, created_at FROM devices ORDER BY id;`)
	if err != nil {
		return err
	}
	data.devices = append(data.devices, devices...)
	revoked, err := readRevoked(ctx, db, `SELECT id, identity_public_key, revoked_at FROM revoked_devices ORDER BY id;`)
	if err != nil {
		return err
	}
	data.revoked = append(data.revoked, revoked...)
	return nil
}

func readV2Passkeys(ctx context.Context, db *sql.DB) ([]Passkey, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, user_id, name, credential_json, created_at, last_used FROM passkeys ORDER BY id;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var passkeys []Passkey
	for rows.Next() {
		var id, userID, name, document string
		var createdMs int64
		var lastUsed sql.NullInt64
		if err := rows.Scan(&id, &userID, &name, &document, &createdMs, &lastUsed); err != nil {
			return nil, err
		}
		credentialID, key, err := parseV2Credential(document)
		if err != nil {
			return nil, fmt.Errorf("passkey %s: %w", id, err)
		}
		passkey := Passkey{
			ID:           id,
			UserID:       userID,
			Name:         name,
			CredentialID: credentialID,
			PublicKey:    key,
			Algorithm:    AlgorithmES256,
			CreatedAt:    timeFromMs(createdMs),
		}
		if lastUsed.Valid {
			used := timeFromMs(lastUsed.Int64)
			passkey.LastUsedAt = &used
		}
		passkeys = append(passkeys, passkey)
	}
	return passkeys, rows.Err()
}

// parseV2Credential extracts the raw credential ID and the canonical COSE key
// from a Rust-era credential document. Only ES256 P-256 keys are accepted.
func parseV2Credential(document string) ([]byte, []byte, error) {
	var credential v2Credential
	if err := json.Unmarshal([]byte(document), &credential); err != nil {
		return nil, nil, err
	}
	inner := credential.Cred.Cred
	if inner.Type != "ES256" || inner.Key.EC2 == nil || inner.Key.EC2.Curve != "SECP256R1" {
		return nil, nil, errors.New("unsupported credential algorithm or curve")
	}
	credentialID, err := base64.RawURLEncoding.DecodeString(credential.Cred.CredID)
	if err != nil {
		return nil, nil, errors.New("credential id is not base64url")
	}
	x, err := base64.RawURLEncoding.DecodeString(inner.Key.EC2.X)
	if err != nil {
		return nil, nil, errors.New("public key x is not base64url")
	}
	y, err := base64.RawURLEncoding.DecodeString(inner.Key.EC2.Y)
	if err != nil {
		return nil, nil, errors.New("public key y is not base64url")
	}
	key, err := webauthn.ES256CoseKey(x, y)
	if err != nil {
		return nil, nil, err
	}
	return credentialID, key, nil
}
