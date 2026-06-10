package bleephub

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// long-tail GitHub API surfaces gh CLI / octokit / probot hit.// Users API extras (keys, gpg_keys, emails, followers, following)
// Actions OIDC (signed token + JWKS + discovery)
// GitHub Pages (site CRUD + builds stubs)
// Branch protection (rules CRUD)
// Org members + audit log
// Marketplace (listing plans/accounts)
//
// Real-GH-shaped responses so callers don't 404; per-surface depth deepens
// when a real consumer needs it.

func (s *Server) registerGHMiscEndpoints() {
	// Users keys + emails + follow
	s.mux.HandleFunc("GET /api/v3/user/keys", s.handleListUserKeys)
	s.mux.HandleFunc("POST /api/v3/user/keys", s.requirePerm(scopeAdministration, permWrite, s.handleCreateUserKey))
	s.mux.HandleFunc("GET /api/v3/user/keys/{key_id}", s.handleGetUserKey)
	s.mux.HandleFunc("DELETE /api/v3/user/keys/{key_id}", s.requirePerm(scopeAdministration, permWrite, s.handleDeleteUserKey))
	s.mux.HandleFunc("GET /api/v3/user/gpg_keys", s.handleListGPGKeys)
	s.mux.HandleFunc("POST /api/v3/user/gpg_keys", s.requirePerm(scopeAdministration, permWrite, s.handleCreateGPGKey))
	s.mux.HandleFunc("GET /api/v3/user/gpg_keys/{gpg_key_id}", s.handleGetGPGKey)
	s.mux.HandleFunc("DELETE /api/v3/user/gpg_keys/{gpg_key_id}", s.requirePerm(scopeAdministration, permWrite, s.handleDeleteGPGKey))
	s.mux.HandleFunc("GET /api/v3/user/emails", s.handleListUserEmails)
	s.mux.HandleFunc("GET /api/v3/users/{username}/keys", s.handleListUserKeysByLogin)
	s.mux.HandleFunc("GET /api/v3/users/{username}/gpg_keys", s.handleListGPGKeysByLogin)
	s.mux.HandleFunc("GET /api/v3/users/{username}/followers", s.handleListFollowers)
	s.mux.HandleFunc("GET /api/v3/users/{username}/following", s.handleListFollowing)
	s.mux.HandleFunc("GET /api/v3/user/followers", s.handleListMyFollowers)
	s.mux.HandleFunc("GET /api/v3/user/following", s.handleListMyFollowing)
	s.mux.HandleFunc("PUT /api/v3/user/following/{username}", s.handleFollowUser)
	s.mux.HandleFunc("DELETE /api/v3/user/following/{username}", s.handleUnfollowUser)

	// Actions OIDC
	s.mux.HandleFunc("GET /token", s.handleActionsOIDCToken)
	s.mux.HandleFunc("GET /.well-known/openid-configuration", s.handleOIDCDiscovery)
	s.mux.HandleFunc("GET /.well-known/jwks", s.handleJWKS)
	s.mux.HandleFunc("GET /api/v3/actions/oidc/customization/sub", s.handleOIDCCustomSubGet)
	s.mux.HandleFunc("PUT /api/v3/actions/oidc/customization/sub",
		s.requirePerm(scopeAdministration, permWrite, s.handleOIDCCustomSubPut))

	// Pages
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/pages", s.handlePagesGet)
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/pages",
		s.requirePerm(scopeAdministration, permWrite, s.handlePagesCreate))
	s.mux.HandleFunc("PUT /api/v3/repos/{owner}/{repo}/pages",
		s.requirePerm(scopeAdministration, permWrite, s.handlePagesUpdate))
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/pages",
		s.requirePerm(scopeAdministration, permWrite, s.handlePagesDelete))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/pages/builds", s.handlePagesListBuilds)
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/pages/builds",
		s.requirePerm(scopeAdministration, permWrite, s.handlePagesTriggerBuild))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/pages/builds/latest", s.handlePagesLatestBuild)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/pages/builds/{build_id}", s.handlePagesGetBuild)

	// Branch protection
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/branches/{branch}/protection", s.handleBranchProtectionGet)
	s.mux.HandleFunc("PUT /api/v3/repos/{owner}/{repo}/branches/{branch}/protection",
		s.requirePerm(scopeAdministration, permWrite, s.handleBranchProtectionPut))
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/branches/{branch}/protection",
		s.requirePerm(scopeAdministration, permWrite, s.handleBranchProtectionDelete))

	// Orgs depth (members listing + memberships CRUD already covered in
	// gh_members_rest.go — implementation).
	s.mux.HandleFunc("GET /api/v3/orgs/{org}/audit-log", s.handleOrgAuditLog)

	// Marketplace
	s.mux.HandleFunc("GET /api/v3/marketplace_listing/plans", s.handleMarketplacePlans)
	s.mux.HandleFunc("GET /api/v3/marketplace_listing/accounts/{account_id}", s.handleMarketplaceAccount)
}

// --- Store ---

// Responses go through userKeyToJSON; the json tags here shape the
// persisted row, which must round-trip UserID to rebuild keysByUser.
type UserKey struct {
	ID        int       `json:"id"`
	Key       string    `json:"key"`
	Title     string    `json:"title"`
	Verified  bool      `json:"verified"`
	UserID    int       `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

type PagesSite struct {
	CNAME   string                 `json:"cname"`
	URL     string                 `json:"url"`
	HTMLURL string                 `json:"html_url"`
	Status  string                 `json:"status"`
	Source  map[string]interface{} `json:"source"`
	Public  bool                   `json:"public"`
}

type GPGKey struct {
	ID                int           `json:"id"`
	KeyID             string        `json:"key_id"`
	PublicKey         string        `json:"public_key"`
	Name              string        `json:"name,omitempty"`
	Emails            []GPGKeyEmail `json:"emails"`
	CanSign           bool          `json:"can_sign"`
	CanEncryptCommits bool          `json:"can_encrypt_commits"`
	CanCertify        bool          `json:"can_certify"`
	CreatedAt         time.Time     `json:"created_at"`
	ExpiresAt         *time.Time    `json:"expires_at,omitempty"`
	UserID            int           `json:"-"`
}

type GPGKeyEmail struct {
	Email    string `json:"email"`
	Verified bool   `json:"verified"`
	Primary  bool   `json:"primary"`
}

type PagesBuild struct {
	ID        int64          `json:"id"`
	URL       string         `json:"url"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Duration  int            `json:"duration"`
	Error     *PagesBuildErr `json:"error,omitempty"`
}

type PagesBuildErr struct {
	Message string `json:"message,omitempty"`
}

type AuditEntry struct {
	ID        int64                  `json:"_document_id"`
	Timestamp string                 `json:"@timestamp"`
	Action    string                 `json:"action"`
	Actor     string                 `json:"actor"`
	Org       string                 `json:"org,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Version   string                 `json:"version"`
}

type MarketplacePlan struct {
	ID                  int      `json:"id"`
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	MonthlyPriceInCents int      `json:"monthly_price_in_cents"`
	YearlyPriceInCents  int      `json:"yearly_price_in_cents"`
	PriceModel          string   `json:"price_model"`
	HasFreeTrial        bool     `json:"has_free_trial"`
	State               string   `json:"state"`
	Bullets             []string `json:"bullets"`
}

type MarketplacePurchase struct {
	AccountID     int        `json:"account_id"`
	BillingCycle  string     `json:"billing_cycle"`
	PlanID        int        `json:"plan_id"`
	PlanName      string     `json:"plan_name"`
	OnFreeTrial   bool       `json:"on_free_trial"`
	FreeTrialEnds *time.Time `json:"free_trial_ends_on,omitempty"`
}

type BranchProtection map[string]interface{}

type MiscStore struct {
	mu                   sync.RWMutex
	userKeys             map[int]*UserKey
	keysByUser           map[int][]*UserKey
	gpgKeys              map[int]*GPGKey
	gpgKeysByUser        map[int][]*GPGKey
	follows              map[string]map[string]bool
	pagesByRepo          map[int]*PagesSite
	pagesBuilds          map[string][]*PagesBuild
	branchProtection     map[string]BranchProtection
	auditLog             []*AuditEntry
	marketplacePlans     map[int]*MarketplacePlan
	marketplacePurchases map[int]*MarketplacePurchase
	oidcClaimKeys        []string
	nextKeyID            int
	nextGPGKeyID         int
	nextAuditID          int64
	oidcKey              *rsa.PrivateKey
	persist              *Persistence
}

func newMiscStore() *MiscStore {
	return &MiscStore{
		userKeys:             map[int]*UserKey{},
		keysByUser:           map[int][]*UserKey{},
		gpgKeys:              map[int]*GPGKey{},
		gpgKeysByUser:        map[int][]*GPGKey{},
		follows:              map[string]map[string]bool{},
		pagesByRepo:          map[int]*PagesSite{},
		pagesBuilds:          map[string][]*PagesBuild{},
		branchProtection:     map[string]BranchProtection{},
		marketplacePlans:     map[int]*MarketplacePlan{},
		marketplacePurchases: map[int]*MarketplacePurchase{},
		nextKeyID:            1,
		nextGPGKeyID:         1,
	}
}

// --- User keys ---

func (s *Server) handleListUserKeys(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	s.store.Misc.mu.RLock()
	defer s.store.Misc.mu.RUnlock()
	out := make([]map[string]interface{}, 0, len(s.store.Misc.keysByUser[user.ID]))
	for _, k := range s.store.Misc.keysByUser[user.ID] {
		out = append(out, userKeyToJSON(k))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateUserKey(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	var req struct {
		Title string `json:"title"`
		Key   string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		writeGHValidationError(w, "Key", "key", "missing_field")
		return
	}
	s.store.Misc.mu.Lock()
	id := s.store.Misc.nextKeyID
	s.store.Misc.nextKeyID++
	k := &UserKey{ID: id, Title: req.Title, Key: req.Key, Verified: true, UserID: user.ID, CreatedAt: time.Now().UTC()}
	s.store.Misc.userKeys[id] = k
	s.store.Misc.keysByUser[user.ID] = append(s.store.Misc.keysByUser[user.ID], k)
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("user_keys", strconv.Itoa(id), k)
	}
	s.store.Misc.mu.Unlock()
	s.recordAuditEvent("ssh_key.create", user.Login, "", map[string]interface{}{"key_id": k.ID})
	writeJSON(w, http.StatusCreated, userKeyToJSON(k))
}

func (s *Server) handleGetUserKey(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("key_id"))
	s.store.Misc.mu.RLock()
	k := s.store.Misc.userKeys[id]
	s.store.Misc.mu.RUnlock()
	if k == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, userKeyToJSON(k))
}

func (s *Server) handleDeleteUserKey(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	id, _ := strconv.Atoi(r.PathValue("key_id"))
	s.store.Misc.mu.Lock()
	k := s.store.Misc.userKeys[id]
	if k == nil {
		s.store.Misc.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	delete(s.store.Misc.userKeys, id)
	src := s.store.Misc.keysByUser[k.UserID]
	for i, x := range src {
		if x.ID == id {
			s.store.Misc.keysByUser[k.UserID] = append(src[:i], src[i+1:]...)
			break
		}
	}
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustDelete("user_keys", strconv.Itoa(id))
	}
	s.store.Misc.mu.Unlock()
	s.recordAuditEvent("ssh_key.delete", user.Login, "", map[string]interface{}{"key_id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListGPGKeys(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	s.store.Misc.mu.RLock()
	out := make([]map[string]interface{}, 0, len(s.store.Misc.gpgKeysByUser[user.ID]))
	for _, k := range s.store.Misc.gpgKeysByUser[user.ID] {
		out = append(out, gpgKeyToJSON(k))
	}
	s.store.Misc.mu.RUnlock()
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateGPGKey(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	var req struct {
		ArmoredPublicKey string `json:"armored_public_key"`
		Name             string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ArmoredPublicKey == "" {
		writeGHValidationError(w, "ArmoredPublicKey", "armored_public_key", "missing_field")
		return
	}
	s.store.Misc.mu.Lock()
	id := s.store.Misc.nextGPGKeyID
	s.store.Misc.nextGPGKeyID++
	email := ""
	if user.Email != "" {
		email = user.Email
	}
	k := &GPGKey{
		ID: id, PublicKey: req.ArmoredPublicKey, Name: req.Name, UserID: user.ID,
		CreatedAt: time.Now(), CanSign: true, CanEncryptCommits: true, CanCertify: true,
		Emails: []GPGKeyEmail{{Email: email, Verified: true, Primary: true}},
	}
	s.store.Misc.gpgKeys[id] = k
	s.store.Misc.gpgKeysByUser[user.ID] = append(s.store.Misc.gpgKeysByUser[user.ID], k)
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("gpg_keys", strconv.Itoa(id), k)
	}
	s.store.Misc.mu.Unlock()
	s.recordAuditEvent("gpg_key.create", user.Login, "", map[string]interface{}{"gpg_key_id": id})
	writeJSON(w, http.StatusCreated, gpgKeyToJSON(k))
}

func (s *Server) handleGetGPGKey(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("gpg_key_id"))
	s.store.Misc.mu.RLock()
	k := s.store.Misc.gpgKeys[id]
	s.store.Misc.mu.RUnlock()
	if k == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, gpgKeyToJSON(k))
}

func (s *Server) handleDeleteGPGKey(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	id, _ := strconv.Atoi(r.PathValue("gpg_key_id"))
	s.store.Misc.mu.Lock()
	k := s.store.Misc.gpgKeys[id]
	if k == nil || k.UserID != user.ID {
		s.store.Misc.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	delete(s.store.Misc.gpgKeys, id)
	src := s.store.Misc.gpgKeysByUser[user.ID]
	for i, x := range src {
		if x.ID == id {
			s.store.Misc.gpgKeysByUser[user.ID] = append(src[:i], src[i+1:]...)
			break
		}
	}
	if s.store.Misc.persist != nil {
		_ = s.store.Misc.persist.Delete("gpg_keys", strconv.Itoa(id))
	}
	s.store.Misc.mu.Unlock()
	s.recordAuditEvent("gpg_key.delete", user.Login, "", map[string]interface{}{"gpg_key_id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListGPGKeysByLogin(w http.ResponseWriter, r *http.Request) {
	user := s.store.LookupUserByLogin(r.PathValue("username"))
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.Misc.mu.RLock()
	out := make([]map[string]interface{}, 0, len(s.store.Misc.gpgKeysByUser[user.ID]))
	for _, k := range s.store.Misc.gpgKeysByUser[user.ID] {
		out = append(out, gpgKeyToJSON(k))
	}
	s.store.Misc.mu.RUnlock()
	writeJSON(w, http.StatusOK, out)
}

func gpgKeyToJSON(k *GPGKey) map[string]interface{} {
	m := map[string]interface{}{
		"id": k.ID, "key_id": k.KeyID, "public_key": k.PublicKey,
		"can_sign": k.CanSign, "can_encrypt_commits": k.CanEncryptCommits,
		"can_certify": k.CanCertify, "created_at": k.CreatedAt.UTC().Format(time.RFC3339),
	}
	if k.Name != "" {
		m["name"] = k.Name
	}
	if len(k.Emails) > 0 {
		m["emails"] = k.Emails
	}
	if k.ExpiresAt != nil {
		m["expires_at"] = k.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return m
}

func (s *Server) handleListUserEmails(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	writeJSON(w, http.StatusOK, []map[string]interface{}{
		{"email": user.Email, "primary": true, "verified": true, "visibility": "private"},
	})
}

func (s *Server) handleListUserKeysByLogin(w http.ResponseWriter, r *http.Request) {
	user := s.store.LookupUserByLogin(r.PathValue("username"))
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.Misc.mu.RLock()
	defer s.store.Misc.mu.RUnlock()
	out := make([]map[string]interface{}, 0, len(s.store.Misc.keysByUser[user.ID]))
	for _, k := range s.store.Misc.keysByUser[user.ID] {
		out = append(out, map[string]interface{}{"id": k.ID, "key": k.Key})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListFollowers(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("username")
	s.store.Misc.mu.RLock()
	followers := []map[string]interface{}{}
	if s.store.Misc.follows != nil {
		for user, follows := range s.store.Misc.follows {
			if follows[target] {
				followers = append(followers, map[string]interface{}{"login": user})
			}
		}
	}
	s.store.Misc.mu.RUnlock()
	writeJSON(w, http.StatusOK, followers)
}
func (s *Server) handleListFollowing(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("username")
	s.store.Misc.mu.RLock()
	following := []map[string]interface{}{}
	if s.store.Misc.follows != nil {
		if follows, ok := s.store.Misc.follows[target]; ok {
			for user := range follows {
				following = append(following, map[string]interface{}{"login": user})
			}
		}
	}
	s.store.Misc.mu.RUnlock()
	writeJSON(w, http.StatusOK, following)
}
func (s *Server) handleListMyFollowers(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	s.store.Misc.mu.RLock()
	followers := []map[string]interface{}{}
	if user != nil && s.store.Misc.follows != nil {
		for follower, follows := range s.store.Misc.follows {
			if follows[user.Login] {
				followers = append(followers, map[string]interface{}{"login": follower})
			}
		}
	}
	s.store.Misc.mu.RUnlock()
	writeJSON(w, http.StatusOK, followers)
}
func (s *Server) handleListMyFollowing(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	s.store.Misc.mu.RLock()
	following := []map[string]interface{}{}
	if user != nil && s.store.Misc.follows != nil {
		if follows, ok := s.store.Misc.follows[user.Login]; ok {
			for target := range follows {
				following = append(following, map[string]interface{}{"login": target})
			}
		}
	}
	s.store.Misc.mu.RUnlock()
	writeJSON(w, http.StatusOK, following)
}

func (s *Server) handleFollowUser(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	target := r.PathValue("username")
	s.store.Misc.mu.Lock()
	if s.store.Misc.follows[user.Login] == nil {
		s.store.Misc.follows[user.Login] = map[string]bool{}
	}
	s.store.Misc.follows[user.Login][target] = true
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("misc", "follows", s.store.Misc.follows)
	}
	s.store.Misc.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnfollowUser(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	target := r.PathValue("username")
	s.store.Misc.mu.Lock()
	if s.store.Misc.follows[user.Login] != nil {
		delete(s.store.Misc.follows[user.Login], target)
	}
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("misc", "follows", s.store.Misc.follows)
	}
	s.store.Misc.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// --- Actions OIDC ---

func (s *Server) handleActionsOIDCToken(w http.ResponseWriter, r *http.Request) {
	audience := r.URL.Query().Get("audience")
	if audience == "" {
		audience = "https://github.com/" + r.URL.Query().Get("repo")
	}
	token, err := s.mintOIDCToken(r, audience)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": token, "count": 1})
}

func (s *Server) handleOIDCDiscovery(w http.ResponseWriter, r *http.Request) {
	base := s.baseURL(r)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"issuer":   base + "/",
		"jwks_uri": base + "/.well-known/jwks",
		// bleephub is both a GitHub Actions OIDC token issuer (id_token) and a
		// standard OAuth2/OIDC provider with a web authorization-code flow.
		// Advertise the authorize/token/userinfo endpoints (all implemented —
		// see gh_oauth.go / gh_rest.go) so relying parties that auto-configure
		// from this document (Pomerium, Teleport, openid-client, …) can use
		// bleephub as an IdP instead of choking on the missing required fields.
		"authorization_endpoint":   base + "/login/oauth/authorize",
		"token_endpoint":           base + "/login/oauth/access_token",
		"userinfo_endpoint":        base + "/api/v3/user",
		"subject_types_supported":  []string{"public", "pairwise"},
		"response_types_supported": []string{"code", "id_token"},
		"response_modes_supported": []string{"query"},
		"grant_types_supported":    []string{"authorization_code"},
		"claims_supported": []string{
			"sub", "aud", "exp", "iat", "iss", "jti", "nbf",
			"ref", "repository", "repository_id", "repository_owner",
			"run_id", "run_number", "sha", "actor", "environment",
		},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid"},
	})
}

func (s *Server) handleJWKS(w http.ResponseWriter, r *http.Request) {
	key := s.oidcKey()
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"keys": []map[string]interface{}{
			{"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "bleephub-oidc", "n": n, "e": e},
		},
	})
}

func (s *Server) handleOIDCCustomSubGet(w http.ResponseWriter, r *http.Request) {
	s.store.Misc.mu.RLock()
	keys := s.store.Misc.oidcClaimKeys
	if keys == nil {
		keys = []string{}
	}
	s.store.Misc.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"include_claim_keys": keys})
}

func (s *Server) handleOIDCCustomSubPut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IncludeClaimKeys []string `json:"include_claim_keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	s.store.Misc.mu.Lock()
	s.store.Misc.oidcClaimKeys = req.IncludeClaimKeys
	if s.store.persist != nil {
		s.store.persist.MustPut("misc", "oidc_claim_keys", req.IncludeClaimKeys)
	}
	s.store.Misc.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]interface{}{"include_claim_keys": req.IncludeClaimKeys, "use_default": false})
}

func (s *Server) oidcKey() *rsa.PrivateKey {
	s.store.Misc.mu.Lock()
	defer s.store.Misc.mu.Unlock()
	if s.store.Misc.oidcKey != nil {
		return s.store.Misc.oidcKey
	}
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("oidc key gen: " + err.Error())
	}
	s.store.Misc.oidcKey = k
	return k
}

func (s *Server) mintOIDCToken(r *http.Request, audience string) (string, error) {
	now := time.Now()
	q := r.URL.Query()
	repoFull := q.Get("repo")
	if repoFull == "" {
		repoFull = "admin/unknown"
	}
	ref := q.Get("ref")
	if ref == "" {
		ref = "refs/heads/main"
	}
	sha := q.Get("sha")
	if sha == "" {
		sha = "0000000000000000000000000000000000000000"
	}
	actor := "bleephub-bot"
	if user := ghUserFromContext(r.Context()); user != nil {
		actor = user.Login
	}
	jti := make([]byte, 12)
	_, _ = rand.Read(jti)
	payload := map[string]interface{}{
		"iss":              s.baseURL(r),
		"aud":              audience,
		"sub":              "repo:" + repoFull + ":ref:" + ref,
		"iat":              now.Unix(),
		"nbf":              now.Unix(),
		"exp":              now.Add(5 * time.Minute).Unix(),
		"jti":              base64.RawURLEncoding.EncodeToString(jti),
		"ref":              ref,
		"repository":       repoFull,
		"repository_owner": "admin",
		"run_id":           "1",
		"run_number":       "1",
		"sha":              sha,
		"actor":            actor,
		"environment":      q.Get("environment"),
	}
	return signRS256JWT(payload, s.oidcKey(), "bleephub-oidc")
}

func signRS256JWT(payload map[string]interface{}, key *rsa.PrivateKey, kid string) (string, error) {
	header := map[string]string{"alg": "RS256", "typ": "JWT", "kid": kid}
	hb, _ := json.Marshal(header)
	pb, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	signing := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(pb)
	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// --- Pages ---

func (s *Server) handlePagesGet(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.Misc.mu.RLock()
	pages := s.store.Misc.pagesByRepo[repo.ID]
	s.store.Misc.mu.RUnlock()
	if pages == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, pages)
}

func (s *Server) handlePagesCreate(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Source struct {
			Branch string `json:"branch"`
			Path   string `json:"path"`
		} `json:"source"`
		CNAME string `json:"cname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	ownerLogin := "admin"
	if repo.Owner != nil {
		ownerLogin = repo.Owner.Login
	}
	pages := &PagesSite{
		CNAME:   req.CNAME,
		URL:     s.baseURL(r) + "/" + repo.FullName + "/pages",
		HTMLURL: "https://" + ownerLogin + ".github.io/" + repo.Name,
		Status:  "built",
		Source: map[string]interface{}{
			"branch": coalesceStr(req.Source.Branch, "main"),
			"path":   coalesceStr(req.Source.Path, "/"),
		},
		Public: !repo.Private,
	}
	s.store.Misc.mu.Lock()
	s.store.Misc.pagesByRepo[repo.ID] = pages
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("pages_sites", strconv.Itoa(repo.ID), pages)
	}
	s.store.Misc.mu.Unlock()
	writeJSON(w, http.StatusCreated, pages)
}

func (s *Server) handlePagesUpdate(w http.ResponseWriter, r *http.Request) { s.handlePagesCreate(w, r) }

func (s *Server) handlePagesDelete(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.Misc.mu.Lock()
	delete(s.store.Misc.pagesByRepo, repo.ID)
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustDelete("pages_sites", strconv.Itoa(repo.ID))
	}
	s.store.Misc.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePagesListBuilds(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.Misc.mu.RLock()
	builds := s.store.Misc.pagesBuilds[repo.FullName]
	s.store.Misc.mu.RUnlock()
	if builds == nil {
		builds = []*PagesBuild{}
	}
	writeJSON(w, http.StatusOK, builds)
}
func (s *Server) handlePagesTriggerBuild(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	actor := "bleephub-system"
	if user := ghUserFromContext(r.Context()); user != nil {
		actor = user.Login
	}
	now := time.Now()
	s.store.Misc.mu.Lock()
	s.store.Misc.nextAuditID++
	buildID := s.store.Misc.nextAuditID
	build := &PagesBuild{
		ID: buildID, Status: "queued", CreatedAt: now, UpdatedAt: now,
	}
	s.store.Misc.pagesBuilds[repo.FullName] = append([]*PagesBuild{build}, s.store.Misc.pagesBuilds[repo.FullName]...)
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("pages_builds", repo.FullName, s.store.Misc.pagesBuilds[repo.FullName])
	}
	s.store.Misc.mu.Unlock()
	s.recordAuditEvent("pages.build", actor, "", map[string]interface{}{"repo": repo.FullName, "build_id": buildID})
	writeJSON(w, http.StatusCreated, build)
}
func (s *Server) handlePagesLatestBuild(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.Misc.mu.RLock()
	builds := s.store.Misc.pagesBuilds[repo.FullName]
	s.store.Misc.mu.RUnlock()
	if len(builds) == 0 {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, builds[0])
}
func (s *Server) handlePagesGetBuild(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	buildID, _ := strconv.ParseInt(r.PathValue("build_id"), 10, 64)
	s.store.Misc.mu.RLock()
	for _, b := range s.store.Misc.pagesBuilds[repo.FullName] {
		if b.ID == buildID {
			s.store.Misc.mu.RUnlock()
			writeJSON(w, http.StatusOK, b)
			return
		}
	}
	s.store.Misc.mu.RUnlock()
	writeGHError(w, http.StatusNotFound, "Not Found")
}

// --- Branch protection ---

func (s *Server) handleBranchProtectionGet(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.Misc.mu.RLock()
	bp := s.store.Misc.branchProtection[bpKey(repo.ID, r.PathValue("branch"))]
	s.store.Misc.mu.RUnlock()
	if bp == nil {
		writeGHError(w, http.StatusNotFound, "Branch not protected")
		return
	}
	writeJSON(w, http.StatusOK, bp)
}

func (s *Server) handleBranchProtectionPut(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var raw map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if raw == nil {
		raw = map[string]interface{}{}
	}
	branch := r.PathValue("branch")
	s.store.Misc.mu.Lock()
	s.store.Misc.branchProtection[bpKey(repo.ID, branch)] = BranchProtection(raw)
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("branch_protection", bpKey(repo.ID, branch), raw)
	}
	s.store.Misc.mu.Unlock()
	writeJSON(w, http.StatusOK, raw)
}

func (s *Server) handleBranchProtectionDelete(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.Misc.mu.Lock()
	delete(s.store.Misc.branchProtection, bpKey(repo.ID, r.PathValue("branch")))
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustDelete("branch_protection", bpKey(repo.ID, r.PathValue("branch")))
	}
	s.store.Misc.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// --- Orgs depth ---

func (s *Server) handleOrgAuditLog(w http.ResponseWriter, r *http.Request) {
	orgName := r.PathValue("org")
	s.store.Misc.mu.RLock()
	entries := make([]*AuditEntry, 0, len(s.store.Misc.auditLog))
	for _, e := range s.store.Misc.auditLog {
		if e.Org != "" && e.Org != orgName {
			continue
		}
		if action := r.URL.Query().Get("phrase"); action != "" {
			if e.Action != action {
				continue
			}
		}
		if actorID := r.URL.Query().Get("actor_id"); actorID != "" {
			if e.Actor != actorID {
				continue
			}
		}
		entries = append(entries, e)
	}
	s.store.Misc.mu.RUnlock()
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) recordAuditEvent(action, actor, org string, data map[string]interface{}) {
	s.store.Misc.mu.Lock()
	defer s.store.Misc.mu.Unlock()
	s.store.Misc.nextAuditID++
	entry := &AuditEntry{
		ID:        s.store.Misc.nextAuditID,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Action:    action,
		Actor:     actor,
		Org:       org,
		Data:      data,
		Version:   "1.1",
	}
	s.store.Misc.auditLog = append([]*AuditEntry{entry}, s.store.Misc.auditLog...)
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("audit_log", fmt.Sprintf("%d", entry.ID), entry)
	}
}

func (s *Server) seedDefaultMarketplacePlans() {
	s.store.Misc.mu.Lock()
	defer s.store.Misc.mu.Unlock()
	if len(s.store.Misc.marketplacePlans) == 0 {
		free := &MarketplacePlan{
			ID: 1, Name: "Free", Description: "Free tier",
			PriceModel: "FREE", State: "published",
			Bullets: []string{"All features"},
		}
		s.store.Misc.marketplacePlans[free.ID] = free
		if s.store.Misc.persist != nil {
			s.store.Misc.persist.MustPut("marketplace_plans", strconv.Itoa(free.ID), free)
		}
	}
}

func (s *Server) handleMarketplacePlans(w http.ResponseWriter, r *http.Request) {
	s.store.Misc.mu.RLock()
	out := make([]map[string]interface{}, 0, len(s.store.Misc.marketplacePlans))
	for _, p := range s.store.Misc.marketplacePlans {
		out = append(out, map[string]interface{}{
			"id": p.ID, "name": p.Name, "description": p.Description,
			"monthly_price_in_cents": p.MonthlyPriceInCents,
			"yearly_price_in_cents":  p.YearlyPriceInCents,
			"price_model":            p.PriceModel, "has_free_trial": p.HasFreeTrial,
			"state": p.State, "bullets": p.Bullets,
		})
	}
	s.store.Misc.mu.RUnlock()
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleMarketplaceAccount(w http.ResponseWriter, r *http.Request) {
	accountID, _ := strconv.Atoi(r.PathValue("account_id"))
	s.store.Misc.mu.RLock()
	purchase := s.store.Misc.marketplacePurchases[accountID]
	s.store.Misc.mu.RUnlock()
	if purchase == nil {
		purchase = &MarketplacePurchase{
			AccountID: accountID, BillingCycle: "monthly", PlanID: 1, PlanName: "Free",
		}
	}
	resp := map[string]interface{}{
		"id":                         accountID,
		"type":                       "User",
		"marketplace_pending_change": nil,
		"marketplace_purchase": map[string]interface{}{
			"billing_cycle": purchase.BillingCycle,
			"on_free_trial": purchase.OnFreeTrial,
			"plan":          map[string]interface{}{"id": purchase.PlanID, "name": purchase.PlanName},
		},
	}
	if purchase.FreeTrialEnds != nil {
		resp["marketplace_pending_change"] = map[string]interface{}{
			"old_plan": map[string]interface{}{"id": purchase.PlanID, "name": purchase.PlanName},
			"new_plan": map[string]interface{}{"id": purchase.PlanID, "name": purchase.PlanName},
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- Helpers ---

func userKeyToJSON(k *UserKey) map[string]interface{} {
	return map[string]interface{}{
		"id":         k.ID,
		"key":        k.Key,
		"title":      k.Title,
		"verified":   k.Verified,
		"created_at": k.CreatedAt.UTC().Format(time.RFC3339),
		"read_only":  false,
	}
}

func bpKey(repoID int, branch string) string {
	return strconv.Itoa(repoID) + ":" + branch
}
