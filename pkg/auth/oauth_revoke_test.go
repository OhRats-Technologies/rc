package auth

import (
	"errors"
	"testing"
)

func TestOAuthTokenRevocationRevokesFamily(t *testing.T) {
	store := openStore(t)
	clientID, _ := seedOAuthClient(t, store)
	user, _ := store.CreateUser("Alice")
	access, refresh, _, _, err := store.IssueOAuthTokens(clientID, user.ID, oauthTestResource, OAuthScopeObserve, "grant_test")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeOAuthToken(access); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LookupOAuthAccess(access, oauthTestResource); !errors.Is(err, ErrOAuthTokenRevoked) {
		t.Fatalf("access after revocation = %v", err)
	}
	if _, _, _, _, _, err := store.ExchangeOAuthRefresh(refresh, clientID, oauthTestResource); !errors.Is(err, ErrOAuthTokenRevoked) {
		t.Fatalf("refresh after revocation = %v", err)
	}
	if err := store.RevokeOAuthToken("not-a-token"); err != nil {
		t.Fatalf("unknown token revocation = %v", err)
	}
}
