package auth

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
	`CREATE TABLE IF NOT EXISTS enrollment_tokens (
		token_hash BLOB PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		expires_ms INTEGER NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS enrollment_tokens_expires_ms ON enrollment_tokens(expires_ms);`,
}
