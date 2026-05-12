package bleephub

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Phase 154 — long-tail GitHub API surfaces gh CLI / octokit / probot hit.
//   P154.7  Users API extras  (keys, gpg_keys, emails, followers, following)
//   P154.8  Actions OIDC       (signed token + JWKS + discovery)
//   P154.9  GitHub Pages       (site CRUD + builds stubs)
//   P154.10 Branch protection  (rules CRUD)
//   P154.11 Org members + audit log
//   P154.12 Marketplace        (listing plans/accounts)
//
// Real-GH-shaped responses so callers don't 404; per-surface depth lands as
// later phases when a real consumer needs it.

func (s *Server) registerGHMiscEndpoints() {
	// Users keys + emails + follow
	s.mux.HandleFunc("GET /api/v3/user/keys", s.handleListUserKeys)
	s.mux.HandleFunc("POST /api/v3/user/keys", s.requirePerm("administration", permWrite, s.handleCreateUserKey))
	s.mux.HandleFunc("GET /api/v3/user/keys/{key_id}", s.handleGetUserKey)
	s.mux.HandleFunc("DELETE /api/v3/user/keys/{key_id}", s.requirePerm("administration", permWrite, s.handleDeleteUserKey))
	s.mux.HandleFunc("GET /api/v3/user/gpg_keys", s.handleListGPGKeys)
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
		s.requirePerm("administration", permWrite, s.handleOIDCCustomSubPut))

	// Pages
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/pages", s.handlePagesGet)
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/pages",
		s.requirePerm("administration", permWrite, s.handlePagesCreate))
	s.mux.HandleFunc("PUT /api/v3/repos/{owner}/{repo}/pages",
		s.requirePerm("administration", permWrite, s.handlePagesUpdate))
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/pages",
		s.requirePerm("administration", permWrite, s.handlePagesDelete))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/pages/builds", s.handlePagesListBuilds)
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/pages/builds",
		s.requirePerm("administration", permWrite, s.handlePagesTriggerBuild))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/pages/builds/latest", s.handlePagesLatestBuild)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/pages/builds/{build_id}", s.handlePagesGetBuild)

	// Branch protection
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/branches/{branch}/protection", s.handleBranchProtectionGet)
	s.mux.HandleFunc("PUT /api/v3/repos/{owner}/{repo}/branches/{branch}/protection",
		s.requirePerm("administration", permWrite, s.handleBranchProtectionPut))
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/branches/{branch}/protection",
		s.requirePerm("administration", permWrite, s.handleBranchProtectionDelete))

	// Orgs depth (members listing + memberships CRUD already covered in
	// gh_members_rest.go — Phase 130 implementation).
	s.mux.HandleFunc("GET /api/v3/orgs/{org}/audit-log", s.handleOrgAuditLog)

	// Marketplace
	s.mux.HandleFunc("GET /api/v3/marketplace_listing/plans", s.handleMarketplacePlans)
	s.mux.HandleFunc("GET /api/v3/marketplace_listing/accounts/{account_id}", s.handleMarketplaceAccount)
}

// --- Store ---

type UserKey struct {
	ID        int       `json:"id"`
	Key       string    `json:"key"`
	Title     string    `json:"title"`
	Verified  bool      `json:"verified"`
	UserID    int       `json:"-"`
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

type BranchProtection map[string]interface{}

type MiscStore struct {
	mu               sync.RWMutex
	userKeys         map[int]*UserKey
	keysByUser       map[int][]*UserKey
	follows          map[string]map[string]bool
	pagesByRepo      map[int]*PagesSite
	branchProtection map[string]BranchProtection
	nextKeyID        int
	oidcKey          *rsa.PrivateKey
}

func newMiscStore() *MiscStore {
	return &MiscStore{
		userKeys:         map[int]*UserKey{},
		keysByUser:       map[int][]*UserKey{},
		follows:          map[string]map[string]bool{},
		pagesByRepo:      map[int]*PagesSite{},
		branchProtection: map[string]BranchProtection{},
		nextKeyID:        1,
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
	k := &UserKey{ID: id, Title: req.Title, Key: req.Key, Verified: true, UserID: user.ID, CreatedAt: time.Now()}
	s.store.Misc.userKeys[id] = k
	s.store.Misc.keysByUser[user.ID] = append(s.store.Misc.keysByUser[user.ID], k)
	s.store.Misc.mu.Unlock()
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
	id, _ := strconv.Atoi(r.PathValue("key_id"))
	s.store.Misc.mu.Lock()
	defer s.store.Misc.mu.Unlock()
	k := s.store.Misc.userKeys[id]
	if k == nil {
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
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListGPGKeys(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
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

func (s *Server) handleListGPGKeysByLogin(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}

func (s *Server) handleListFollowers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}
func (s *Server) handleListFollowing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}
func (s *Server) handleListMyFollowers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}
func (s *Server) handleListMyFollowing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
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
		"issuer":                   base + "/",
		"jwks_uri":                 base + "/.well-known/jwks",
		"subject_types_supported":  []string{"public", "pairwise"},
		"response_types_supported": []string{"id_token"},
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
	writeJSON(w, http.StatusOK, map[string]interface{}{"include_claim_keys": []string{}})
}

func (s *Server) handleOIDCCustomSubPut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IncludeClaimKeys []string `json:"include_claim_keys"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
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
	_ = json.NewDecoder(r.Body).Decode(&req)
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
	s.store.Misc.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePagesListBuilds(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}
func (s *Server) handlePagesTriggerBuild(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusCreated, map[string]string{"status": "queued"})
}
func (s *Server) handlePagesLatestBuild(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "built", "duration": 0})
}
func (s *Server) handlePagesGetBuild(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "built", "duration": 0})
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
	_ = json.NewDecoder(r.Body).Decode(&raw)
	if raw == nil {
		raw = map[string]interface{}{}
	}
	branch := r.PathValue("branch")
	s.store.Misc.mu.Lock()
	s.store.Misc.branchProtection[bpKey(repo.ID, branch)] = BranchProtection(raw)
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
	s.store.Misc.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// --- Orgs depth ---

// handleOrgAuditLog — bleephub doesn't record an audit log; shape-only empty.
func (s *Server) handleOrgAuditLog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}

// --- Marketplace ---

func (s *Server) handleMarketplacePlans(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []map[string]interface{}{
		{
			"id":                     1,
			"name":                   "Free",
			"description":            "Free tier — sim default plan",
			"monthly_price_in_cents": 0,
			"yearly_price_in_cents":  0,
			"price_model":            "FREE",
			"bullets":                []string{"All features", "Sim mode"},
			"state":                  "published",
		},
	})
}

func (s *Server) handleMarketplaceAccount(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("account_id"))
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                         id,
		"type":                       "User",
		"marketplace_pending_change": nil,
		"marketplace_purchase": map[string]interface{}{
			"billing_cycle": "monthly",
			"plan":          map[string]interface{}{"id": 1, "name": "Free"},
		},
	})
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
