// Package auth stores RC human accounts, passkeys, WebAuthn ceremonies,
// browser sessions, and MCP OAuth authorization grants in SQLite.
//
// The store persists only public credential material and hashes: users,
// credential IDs, COSE public keys, single-use ceremony state, hashed session
// tokens, and hashed OAuth codes and tokens. It never stores private keys or
// plaintext credentials.
package auth

import "errors"

var (
	ErrUserNotFound         = errors.New("auth: user not found")
	ErrPasskeyNotFound      = errors.New("auth: passkey not found")
	ErrCredentialRegistered = errors.New("auth: credential is already registered")
	ErrInvalidName          = errors.New("auth: name is invalid")
	ErrInvalidCredential    = errors.New("auth: credential is invalid")
	ErrInvalidCeremony      = errors.New("auth: ceremony is invalid")
	ErrInvalidSessionToken  = errors.New("auth: session token is invalid")
	ErrSessionNotFound      = errors.New("auth: session not found")
	ErrSessionExpired       = errors.New("auth: session expired")
	ErrInvalidLifetime      = errors.New("auth: session lifetime is invalid")

	ErrOAuthClientNotFound   = errors.New("auth: oauth client not found")
	ErrOAuthInvalidClient    = errors.New("auth: oauth client metadata is invalid")
	ErrOAuthInvalidRedirect  = errors.New("auth: oauth redirect URI is invalid")
	ErrOAuthInvalidGrant     = errors.New("auth: oauth grant is invalid")
	ErrOAuthInvalidCode      = errors.New("auth: oauth authorization code is invalid or expired")
	ErrOAuthInvalidVerifier  = errors.New("auth: oauth code verifier does not match")
	ErrOAuthTokenNotFound    = errors.New("auth: oauth token not found")
	ErrOAuthTokenExpired     = errors.New("auth: oauth token expired")
	ErrOAuthTokenRevoked     = errors.New("auth: oauth token revoked")
	ErrOAuthResourceMismatch = errors.New("auth: oauth token is bound to another resource")
	ErrControlClientNotFound = errors.New("auth: control client not found or expired")
	ErrControlClientConflict = errors.New("auth: control client belongs to another account")
)
