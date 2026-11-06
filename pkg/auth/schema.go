package auth

// schemaVersion is the user_version written by the current schema. Databases
// with a different version are never opened in place.
const schemaVersion = 1

var schema = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		created_ms INTEGER NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS passkeys (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		credential_id BLOB NOT NULL UNIQUE,
		public_key BLOB NOT NULL,
		algorithm INTEGER NOT NULL,
		created_ms INTEGER NOT NULL,
		last_used_ms INTEGER
	);`,
	`CREATE INDEX IF NOT EXISTS passkeys_user_id ON passkeys(user_id);`,
	`CREATE TABLE IF NOT EXISTS ceremonies (
		id TEXT PRIMARY KEY,
		kind TEXT NOT NULL,
		user_id TEXT,
		state BLOB NOT NULL,
		expires_ms INTEGER NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS ceremonies_expires_ms ON ceremonies(expires_ms);`,
	`CREATE TABLE IF NOT EXISTS sessions (
		token_hash BLOB PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_ms INTEGER NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS sessions_user_id ON sessions(user_id);`,
	`CREATE INDEX IF NOT EXISTS sessions_expires_ms ON sessions(expires_ms);`,
	`CREATE TABLE IF NOT EXISTS workspaces (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
		created_ms INTEGER NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS workspace_members (
		workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		role TEXT NOT NULL CHECK (role IN ('owner', 'operator', 'viewer')),
		joined_ms INTEGER NOT NULL,
		PRIMARY KEY (workspace_id, user_id)
	);`,
	`CREATE INDEX IF NOT EXISTS workspace_members_user_id ON workspace_members(user_id);`,
	`CREATE TABLE IF NOT EXISTS enrollment_tokens (
		token_hash BLOB PRIMARY KEY,
		workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
		expires_ms INTEGER NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS enrollment_tokens_expires_ms ON enrollment_tokens(expires_ms);`,
	`CREATE TABLE IF NOT EXISTS devices (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		hostname TEXT NOT NULL,
		platform TEXT NOT NULL,
		arch TEXT NOT NULL,
		version TEXT NOT NULL,
		capabilities TEXT NOT NULL DEFAULT '[]',
		identity_public_key TEXT NOT NULL UNIQUE,
		transport_public_key TEXT NOT NULL,
		lock_hash TEXT NOT NULL DEFAULT '',
		lock_generation INTEGER NOT NULL DEFAULT 0,
		last_seen_ms INTEGER,
		created_ms INTEGER NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS devices_workspace_id ON devices(workspace_id);`,
	`CREATE TABLE IF NOT EXISTS revoked_devices (
		id TEXT PRIMARY KEY,
		identity_public_key TEXT NOT NULL,
		revoked_ms INTEGER NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS revoked_devices_identity ON revoked_devices(identity_public_key);`,
}
