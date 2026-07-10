package bleephub

import (
	"fmt"
	"time"
)

// UserToServerToken is an OAuth-derived token bearing a user identity.
//
// Two prefix variants:
//   - gho_  — classic OAuth-App user token, classic scopes (`repo`, `read:org`, …)
//   - ghu_  — GitHub-App user-to-server token, scoped to the app's installation permissions
//
// Both carry the user identity through the request middleware. The difference is the
// scope model and whether AppID is set (ghu_) vs OAuthAppClientID (gho_).
type UserToServerToken struct {
	Token             string
	UserID            int
	AppID             int    // set for ghu_ (GitHub App user-to-server)
	OAuthAppClientID  string // set for gho_ (OAuth App user token)
	Scopes            string // classic OAuth scopes when gho_
	InstallationIDs   []int  // optional: scoped to specific installations (ghu_ only)
	ExpiresAt         time.Time
	RefreshTokenValue string // if non-empty, this token has a paired ghr_ refresh
	CreatedAt         time.Time
}

// RefreshToken pairs with a UserToServerToken. Used to mint a fresh user token
// past the user token's expiry without re-running the OAuth flow.
type RefreshToken struct {
	Token            string
	UserID           int
	AppID            int
	OAuthAppClientID string
	Scopes           string
	ExpiresAt        time.Time // typically 6 months
	CreatedAt        time.Time
}

// CreateUserToServerToken mints a gho_/ghu_ token (+ optional ghr_ pair).
// Pass appID > 0 for ghu_ (GitHub-App user-to-server) or oauthClientID for gho_ (OAuth-App user).
// If withRefresh is true, also mints a ghr_ refresh token.
func (st *Store) CreateUserToServerToken(userID, appID int, oauthClientID, scopes string, ttl time.Duration, withRefresh bool) (*UserToServerToken, *RefreshToken) {
	tok, rt, err := st.CreateUserToServerTokenE(userID, appID, oauthClientID, scopes, ttl, withRefresh)
	if err != nil {
		panic(err)
	}
	return tok, rt
}

func (st *Store) CreateUserToServerTokenE(userID, appID int, oauthClientID, scopes string, ttl time.Duration, withRefresh bool) (*UserToServerToken, *RefreshToken, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.createUserToServerTokenLocked(userID, appID, oauthClientID, scopes, ttl, withRefresh)
}

func (st *Store) createUserToServerTokenLocked(userID, appID int, oauthClientID, scopes string, ttl time.Duration, withRefresh bool) (*UserToServerToken, *RefreshToken, error) {
	if st.UserToServerTokens == nil {
		st.UserToServerTokens = make(map[string]*UserToServerToken)
	}
	if st.RefreshTokens == nil {
		st.RefreshTokens = make(map[string]*RefreshToken)
	}

	now := time.Now()
	if ttl <= 0 {
		ttl = 8 * time.Hour
	}
	prefix := tokenPrefixOAuthUser
	if appID > 0 {
		prefix = tokenPrefixAppUser
	}
	h, err := randomHex(20)
	if err != nil {
		return nil, nil, fmt.Errorf("generate user-to-server token: %w", err)
	}
	tokenStr := prefix + h

	tok := &UserToServerToken{
		Token:            tokenStr,
		UserID:           userID,
		AppID:            appID,
		OAuthAppClientID: oauthClientID,
		Scopes:           scopes,
		ExpiresAt:        now.Add(ttl),
		CreatedAt:        now,
	}

	var rt *RefreshToken
	if withRefresh {
		refreshHex, err := randomHex(20)
		if err != nil {
			return nil, nil, fmt.Errorf("generate refresh token: %w", err)
		}
		rt = &RefreshToken{
			Token:            tokenPrefixRefresh + refreshHex,
			UserID:           userID,
			AppID:            appID,
			OAuthAppClientID: oauthClientID,
			Scopes:           scopes,
			ExpiresAt:        now.Add(6 * 30 * 24 * time.Hour),
			CreatedAt:        now,
		}
		tok.RefreshTokenValue = rt.Token
		st.RefreshTokens[rt.Token] = rt
		if st.persist != nil {
			st.persist.MustPut("refresh_tokens", rt.Token, rt)
		}
	}
	st.UserToServerTokens[tokenStr] = tok
	if st.persist != nil {
		st.persist.MustPut("user_to_server_tokens", tokenStr, tok)
	}
	return tok, rt, nil
}

// SetUserToServerTokenInstallations binds the token to a set of installation
// IDs (used when a token is scoped down to a specific installation target).
func (st *Store) SetUserToServerTokenInstallations(tokenStr string, installationIDs []int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	tok := st.UserToServerTokens[tokenStr]
	if tok == nil {
		return false
	}
	tok.InstallationIDs = append([]int(nil), installationIDs...)
	if st.persist != nil {
		st.persist.MustPut("user_to_server_tokens", tokenStr, tok)
	}
	return true
}

// LookupUserToServerToken returns the token + bearing user, or nil if not found/expired.
func (st *Store) LookupUserToServerToken(tokenStr string) (*UserToServerToken, *User) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	tok := st.UserToServerTokens[tokenStr]
	if tok == nil || time.Now().After(tok.ExpiresAt) {
		return nil, nil
	}
	return tok, st.Users[tok.UserID]
}

// RevokeUserToServerToken drops a user-to-server token. Returns true if it existed.
func (st *Store) RevokeUserToServerToken(tokenStr string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	tok := st.UserToServerTokens[tokenStr]
	if tok == nil {
		return false
	}
	delete(st.UserToServerTokens, tokenStr)
	if st.persist != nil {
		st.persist.MustDelete("user_to_server_tokens", tokenStr)
	}
	if tok.RefreshTokenValue != "" {
		delete(st.RefreshTokens, tok.RefreshTokenValue)
		if st.persist != nil {
			st.persist.MustDelete("refresh_tokens", tok.RefreshTokenValue)
		}
	}
	return true
}

// RotateUserToServerToken mints a fresh user-to-server token + refresh pair from a
// valid refresh token. Old token + refresh are revoked. Returns nil if refresh is invalid.
func (st *Store) RotateUserToServerToken(refreshTokenStr string) (*UserToServerToken, *RefreshToken) {
	tok, rt, err := st.RotateUserToServerTokenE(refreshTokenStr)
	if err != nil {
		panic(err)
	}
	return tok, rt
}

func (st *Store) RotateUserToServerTokenE(refreshTokenStr string) (*UserToServerToken, *RefreshToken, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	rt := st.RefreshTokens[refreshTokenStr]
	if rt == nil || time.Now().After(rt.ExpiresAt) {
		return nil, nil, nil
	}
	// Revoke the matching user token (find by RefreshTokenValue). The deletes
	// write through to disk so the rotated-out credentials stay dead after a
	// restart instead of resurrecting from the persisted buckets.
	for k, v := range st.UserToServerTokens {
		if v.RefreshTokenValue == refreshTokenStr {
			delete(st.UserToServerTokens, k)
			if st.persist != nil {
				st.persist.MustDelete("user_to_server_tokens", k)
			}
			break
		}
	}
	delete(st.RefreshTokens, refreshTokenStr)
	if st.persist != nil {
		st.persist.MustDelete("refresh_tokens", refreshTokenStr)
	}
	return st.createUserToServerTokenLocked(rt.UserID, rt.AppID, rt.OAuthAppClientID, rt.Scopes, 8*time.Hour, true)
}

// RevokeUserGrant deletes every user-to-server + refresh token for (clientID, userID).
// Mirrors GitHub's DELETE /applications/{client_id}/grant.
func (st *Store) RevokeUserGrant(clientID string, userID int) int {
	st.mu.Lock()
	defer st.mu.Unlock()
	n := 0
	for k, v := range st.UserToServerTokens {
		hit := false
		if v.UserID == userID {
			if v.OAuthAppClientID == clientID {
				hit = true
			} else if v.AppID > 0 {
				if app := st.AppsByClientID[clientID]; app != nil && app.ID == v.AppID {
					hit = true
				}
			}
		}
		if hit {
			delete(st.UserToServerTokens, k)
			if st.persist != nil {
				st.persist.MustDelete("user_to_server_tokens", k)
			}
			if v.RefreshTokenValue != "" {
				delete(st.RefreshTokens, v.RefreshTokenValue)
				if st.persist != nil {
					st.persist.MustDelete("refresh_tokens", v.RefreshTokenValue)
				}
			}
			n++
		}
	}
	return n
}
