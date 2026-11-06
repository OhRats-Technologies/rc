package auth

import "time"

// OAuth token and code lifetimes. Access tokens are short-lived; refresh
// tokens rotate on every exchange.
const (
	OAuthAccessTokenLifetime  = 15 * time.Minute
	OAuthRefreshTokenLifetime = 30 * 24 * time.Hour
	OAuthCodeLifetime         = 5 * time.Minute

	// OAuthTokenBytes is the entropy of every issued code, access token, and
	// refresh token.
	OAuthTokenBytes = 32
)

// OAuthClient is a dynamically registered MCP OAuth client.
type OAuthClient struct {
	ID           string
	Name         string
	RedirectURIs []string
	CreatedAt    time.Time
}

// OAuthGrant is the resource-bound authorization carried by an access token.
type OAuthGrant struct {
	ClientID         string
	UserID           string
	Resource         string
	Scopes           string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}
