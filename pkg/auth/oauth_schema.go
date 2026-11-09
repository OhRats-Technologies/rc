package auth

// oauthSchema declares the MCP OAuth 2.1 authorization-server tables. Only
// hashes of authorization codes, access tokens, and refresh tokens are stored.
var oauthSchema = []string{
	`CREATE TABLE IF NOT EXISTS oauth_clients (
		client_id TEXT PRIMARY KEY,
		client_name TEXT NOT NULL,
		redirect_uris TEXT NOT NULL,
		created_ms INTEGER NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS oauth_codes (
		code_hash BLOB PRIMARY KEY,
		client_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		redirect_uri TEXT NOT NULL,
		code_challenge TEXT NOT NULL,
		resource TEXT NOT NULL,
		scopes TEXT NOT NULL DEFAULT 'mcp:observe',
		grant_id TEXT NOT NULL DEFAULT '',
		expires_ms INTEGER NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS oauth_codes_expires_ms ON oauth_codes(expires_ms);`,
	`CREATE INDEX IF NOT EXISTS oauth_codes_user_id ON oauth_codes(user_id);`,
	`CREATE TABLE IF NOT EXISTS oauth_requests (
		id TEXT PRIMARY KEY,
		client_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		redirect_uri TEXT NOT NULL,
		state TEXT NOT NULL,
		code_challenge TEXT NOT NULL,
		resource TEXT NOT NULL,
		scopes TEXT NOT NULL,
		prepared_grant TEXT NOT NULL DEFAULT '',
		expires_ms INTEGER NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS oauth_requests_expires_ms ON oauth_requests(expires_ms);`,
	`CREATE INDEX IF NOT EXISTS oauth_requests_user_id ON oauth_requests(user_id);`,
	`CREATE TABLE IF NOT EXISTS oauth_tokens (
		access_hash BLOB PRIMARY KEY,
		refresh_hash BLOB NOT NULL UNIQUE,
		client_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		resource TEXT NOT NULL,
		scopes TEXT NOT NULL,
		grant_id TEXT NOT NULL DEFAULT '',
		access_expires_ms INTEGER NOT NULL,
		refresh_expires_ms INTEGER NOT NULL,
		revoked_ms INTEGER
	);`,
	`CREATE INDEX IF NOT EXISTS oauth_tokens_user_id ON oauth_tokens(user_id);`,
	`CREATE INDEX IF NOT EXISTS oauth_tokens_refresh_expires_ms ON oauth_tokens(refresh_expires_ms);`,
}
