package bleephub

import (
	"context"
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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// long-tail GitHub API surfaces gh CLI / octokit / probot hit.// Users API extras (keys, gpg_keys, emails, followers, following)
// Actions OIDC (signed token + JWKS + discovery)
// GitHub Pages (site CRUD + builds + deployments)
// Org members + audit log
// Marketplace (listing plans/accounts)
//
// Real-GH-shaped responses so callers don't 404; per-surface depth deepens
// when a real consumer needs it.

func (s *Server) registerGHMiscEndpoints() {
	// Users keys + emails + follow
	s.route("GET /api/v3/user/keys", s.handleListUserKeys)
	s.route("POST /api/v3/user/keys", s.requirePerm(scopeAdministration, permWrite, s.handleCreateUserKey))
	s.route("GET /api/v3/user/keys/{key_id}", s.handleGetUserKey)
	s.route("DELETE /api/v3/user/keys/{key_id}", s.requirePerm(scopeAdministration, permWrite, s.handleDeleteUserKey))
	s.route("GET /api/v3/user/gpg_keys", s.handleListGPGKeys)
	s.route("POST /api/v3/user/gpg_keys", s.requirePerm(scopeAdministration, permWrite, s.handleCreateGPGKey))
	s.route("GET /api/v3/user/gpg_keys/{gpg_key_id}", s.handleGetGPGKey)
	s.route("DELETE /api/v3/user/gpg_keys/{gpg_key_id}", s.requirePerm(scopeAdministration, permWrite, s.handleDeleteGPGKey))
	s.route("GET /api/v3/user/emails", s.handleListUserEmails)
	s.route("GET /api/v3/users/{username}/keys", s.handleListUserKeysByLogin)
	s.route("GET /api/v3/users/{username}/gpg_keys", s.handleListGPGKeysByLogin)
	s.route("GET /api/v3/users/{username}/followers", s.handleListFollowers)
	s.route("GET /api/v3/users/{username}/following", s.handleListFollowing)
	s.route("GET /api/v3/user/followers", s.handleListMyFollowers)
	s.route("GET /api/v3/user/following", s.handleListMyFollowing)
	s.route("PUT /api/v3/user/following/{username}", s.handleFollowUser)
	s.route("DELETE /api/v3/user/following/{username}", s.handleUnfollowUser)

	// Users extras
	s.route("GET /api/v3/users", s.handleListUsers)
	s.route("POST /api/v3/admin/users", s.handleAdminCreateUser)
	s.route("PATCH /api/v3/admin/users/{username}", s.handleAdminRenameUser)
	s.route("DELETE /api/v3/admin/users/{username}", s.handleAdminDeleteUser)
	s.route("PUT /api/v3/users/{username}/site_admin", s.handleAdminPromoteUser)
	s.route("DELETE /api/v3/users/{username}/site_admin", s.handleAdminDemoteUser)
	s.route("PUT /api/v3/users/{username}/suspended", s.handleAdminSuspendUser)
	s.route("DELETE /api/v3/users/{username}/suspended", s.handleAdminUnsuspendUser)
	s.route("GET /api/v3/users/{username}/gists", s.handleListUserGists)
	s.route("GET /api/v3/users/{username}/events", s.handleListUserEvents)
	s.route("GET /api/v3/users/{username}/events/public", s.handleListUserEventsPublic)
	s.route("GET /api/v3/users/{username}/events/orgs/{org}", s.handleListUserEventsForOrg)
	s.route("GET /api/v3/users/{username}/received_events", s.handleListUserReceivedEvents)
	s.route("GET /api/v3/users/{username}/received_events/public", s.handleListUserReceivedEventsPublic)
	s.route("GET /api/v3/users/{username}/following/{target_user}", s.handleCheckUserFollowing)
	s.route("GET /api/v3/users/{username}/social_accounts", s.handleListUserSocialAccounts)
	s.route("GET /api/v3/users/{username}/ssh_signing_keys", s.handleListUserSSHSigningKeys)
	s.route("GET /api/v3/users/{username}/subscriptions", s.handleListUserSubscriptions)
	s.route("GET /api/v3/user/blocks", s.handleListUserBlocks)
	s.route("GET /api/v3/user/blocks/{username}", s.handleCheckUserBlocked)
	s.route("PUT /api/v3/user/blocks/{username}", s.handleBlockUser)
	s.route("DELETE /api/v3/user/blocks/{username}", s.handleUnblockUser)
	s.route("GET /api/v3/user/following/{username}", s.handleCheckMyFollowing)
	s.route("GET /api/v3/user/social_accounts", s.handleListMySocialAccounts)
	s.route("POST /api/v3/user/social_accounts", s.handleCreateMySocialAccounts)
	s.route("DELETE /api/v3/user/social_accounts", s.handleDeleteMySocialAccounts)
	s.route("GET /api/v3/user/ssh_signing_keys", s.handleListMySSHSigningKeys)
	s.route("POST /api/v3/user/ssh_signing_keys", s.handleCreateMySSHSigningKey)
	s.route("DELETE /api/v3/user/ssh_signing_keys/{ssh_signing_key_id}", s.handleDeleteMySSHSigningKey)
	s.route("GET /api/v3/user/starred/{owner}/{repo}", s.handleCheckMyStarredRepo)
	s.route("GET /api/v3/user/subscriptions", s.handleListMySubscriptions)

	// Actions OIDC
	s.route("GET /token", s.handleActionsOIDCToken)
	s.route("GET /.well-known/openid-configuration", s.handleOIDCDiscovery)
	s.route("GET /.well-known/jwks", s.handleJWKS)
	// OIDC subject customization is scoped to a repo or an org on real GitHub.
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/oidc/customization/sub", s.handleOIDCCustomSubGet)
	s.route("PUT /api/v3/repos/{owner}/{repo}/actions/oidc/customization/sub",
		s.requirePerm(scopeAdministration, permWrite, s.handleOIDCCustomSubPut))
	s.route("GET /api/v3/orgs/{org}/actions/oidc/customization/sub", s.handleOIDCCustomSubGet)
	s.route("PUT /api/v3/orgs/{org}/actions/oidc/customization/sub",
		s.requirePerm(scopeAdministration, permWrite, s.handleOIDCCustomSubPut))

	// Pages
	s.route("GET /api/v3/repos/{owner}/{repo}/pages", s.requirePagesRead(s.handlePagesGet))
	s.route("POST /api/v3/repos/{owner}/{repo}/pages",
		s.requirePerm(scopePages, permWrite, s.handlePagesCreate))
	s.route("PUT /api/v3/repos/{owner}/{repo}/pages",
		s.requirePerm(scopePages, permWrite, s.handlePagesUpdate))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/pages",
		s.requirePerm(scopePages, permWrite, s.handlePagesDelete))
	s.route("GET /api/v3/repos/{owner}/{repo}/pages/builds", s.requirePagesRead(s.handlePagesListBuilds))
	s.route("POST /api/v3/repos/{owner}/{repo}/pages/builds",
		s.requirePerm(scopePages, permWrite, s.handlePagesTriggerBuild))
	s.route("GET /api/v3/repos/{owner}/{repo}/pages/builds/latest", s.requirePagesRead(s.handlePagesLatestBuild))
	s.route("GET /api/v3/repos/{owner}/{repo}/pages/builds/{build_id}", s.requirePagesRead(s.handlePagesGetBuild))

	// Orgs depth (members listing + memberships CRUD already covered in
	// gh_members_rest.go — implementation).
	s.route("GET /api/v3/orgs/{org}/audit-log", s.handleOrgAuditLog)

	// Marketplace. The stubbed variants serve the same real plan/purchase
	// state as the production routes, per the documented stubbed semantics.
	s.route("GET /api/v3/marketplace_listing/plans", s.handleMarketplacePlans)
	s.route("GET /api/v3/marketplace_listing/accounts/{account_id}", s.handleMarketplaceAccount)
	s.route("GET /api/v3/marketplace_listing/plans/{plan_id}/accounts", s.handleMarketplacePlanAccounts)
	s.route("GET /api/v3/marketplace_listing/stubbed/plans", s.handleMarketplacePlans)
	s.route("GET /api/v3/marketplace_listing/stubbed/plans/{plan_id}/accounts", s.handleMarketplacePlanAccounts)
	s.route("GET /api/v3/marketplace_listing/stubbed/accounts/{account_id}", s.handleMarketplaceAccount)

	// Meta — gh CLI's GHES feature detection resolves the host version from
	// GET /meta installed_version before search-backed listing commands
	// (gh issue list --label, gh pr status) and gh workflow run.
	s.route("GET /api/v3/meta", s.handleMeta)
}

// handleMeta serves GET /api/v3/meta in GHES shape. bleephub presents as
// GHES 3.21.0: gh gates the advanced-issue-search syntax at >= 3.18 (sent
// with the plain ISSUE search type when the SearchType enum has no
// ISSUE_ADVANCED member, which is bleephub's case), drops classic-projects
// fields at >= 3.17, and sends `return_run_details` on workflow dispatches
// at >= 3.21 (the dispatch handler ignores the extra member and answers 204,
// which gh handles). installed_version is a GHES-only member — it is
// documented in the GHES OpenAPI description, not the dotcom one this repo
// vendors for shape validation (see openapi-violation-allowlist.txt).
// verifiable_password_authentication is genuinely false: bleephub's API is
// token-only.
func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"verifiable_password_authentication": false,
		"installed_version":                  "3.21.0",
	})
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
	CNAME                string                 `json:"cname"`
	URL                  string                 `json:"url"`
	HTMLURL              string                 `json:"html_url"`
	Status               string                 `json:"status"`
	Source               map[string]interface{} `json:"source"`
	Public               bool                   `json:"public"`
	Custom404            bool                   `json:"custom_404"`
	ProtectedDomainState *string                `json:"protected_domain_state"`
	BuildType            *string                `json:"build_type"`
	HTTPSCertificate     *PagesHTTPSCertificate `json:"https_certificate,omitempty"`
	HTTPSEnforced        bool                   `json:"https_enforced"`
}

type PagesHTTPSCertificate struct {
	State       string   `json:"state"`
	Description string   `json:"description"`
	Domains     []string `json:"domains"`
	ExpiresAt   *string  `json:"expires_at"`
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
	// ID is the numeric build identifier used for path-based routing
	// (GET .../pages/builds/{build_id}). GitHub's build object carries no
	// top-level `id`; it is addressed via the trailing segment of `url`, so
	// the field is not serialized.
	ID        int64          `json:"-"`
	URL       string         `json:"url"`
	Status    string         `json:"status"`
	Pusher    *PagesPusher   `json:"pusher"`
	Commit    string         `json:"commit"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Duration  int            `json:"duration"`
	Error     *PagesBuildErr `json:"error"`
}

type PagesPusher struct {
	Login string `json:"login"`
	ID    int    `json:"id"`
	Type  string `json:"type"`
}

type PagesBuildErr struct {
	Message *string `json:"message"`
}

func pagesBuildIDFromURL(url string) int64 {
	idx := strings.LastIndex(url, "/")
	if idx < 0 || idx == len(url)-1 {
		return 0
	}
	id, err := strconv.ParseInt(url[idx+1:], 10, 64)
	if err != nil {
		return 0
	}
	return id
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

type AuditLogEvent struct {
	ID         int64                  `json:"id"`
	Timestamp  string                 `json:"timestamp"`
	Actor      string                 `json:"actor"`
	Action     string                 `json:"action"`
	TargetType string                 `json:"target_type"`
	TargetID   string                 `json:"target_id"`
	Org        string                 `json:"org,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
	createdAt  time.Time              `json:"-"`
}

type MarketplacePlan struct {
	ID                  int      `json:"id"`
	ListingSlug         string   `json:"listing_slug"`
	Number              int      `json:"number"`
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	MonthlyPriceInCents int      `json:"monthly_price_in_cents"`
	YearlyPriceInCents  int      `json:"yearly_price_in_cents"`
	PriceModel          string   `json:"price_model"`
	HasFreeTrial        bool     `json:"has_free_trial"`
	UnitName            string   `json:"unit_name"`
	State               string   `json:"state"`
	Bullets             []string `json:"bullets"`
}

type MarketplaceListing struct {
	Slug               string    `json:"slug"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	FullDescription    string    `json:"full_description"`
	SetupURL           string    `json:"setup_url,omitempty"`
	InstallationURL    string    `json:"installation_url,omitempty"`
	GitHubAppID        int       `json:"github_app_id,omitempty"`
	OAuthAppClientID   string    `json:"oauth_app_client_id,omitempty"`
	WebhookURL         string    `json:"webhook_url,omitempty"`
	WebhookSecret      string    `json:"webhook_secret,omitempty"`
	WebhookContentType string    `json:"webhook_content_type,omitempty"`
	WebhookActive      bool      `json:"webhook_active"`
	WebhookID          int       `json:"webhook_id,omitempty"`
	Published          bool      `json:"published"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type MarketplacePendingChange struct {
	PlanID        int       `json:"plan_id,omitempty"`
	BillingCycle  string    `json:"billing_cycle,omitempty"`
	UnitCount     *int      `json:"unit_count,omitempty"`
	EffectiveDate time.Time `json:"effective_date"`
	Cancellation  bool      `json:"cancellation,omitempty"`
	ActorID       int       `json:"actor_id"`
}

type MarketplacePurchase struct {
	ListingSlug   string     `json:"listing_slug"`
	AccountID     int        `json:"account_id"`
	AccountType   string     `json:"account_type"`
	BillingCycle  string     `json:"billing_cycle"`
	PlanID        int        `json:"plan_id"`
	PlanName      string     `json:"plan_name"`
	OnFreeTrial   bool       `json:"on_free_trial"`
	FreeTrialEnds *time.Time `json:"free_trial_ends_on,omitempty"`
	// user-marketplace-purchase members surfaced by GET /user/marketplace_purchases.
	UnitCount       *int                      `json:"unit_count,omitempty"`
	NextBillingDate *time.Time                `json:"next_billing_date,omitempty"`
	UpdatedAt       *time.Time                `json:"updated_at,omitempty"`
	InstallationID  *int                      `json:"installation_id,omitempty"`
	PendingChange   *MarketplacePendingChange `json:"pending_change,omitempty"`
}

type MiscStore struct {
	mu                        sync.RWMutex
	userKeys                  map[int]*UserKey
	keysByUser                map[int][]*UserKey
	gpgKeys                   map[int]*GPGKey
	gpgKeysByUser             map[int][]*GPGKey
	follows                   map[string]map[string]bool
	pagesByRepo               map[int]*PagesSite
	pagesBuilds               map[string][]*PagesBuild
	branchProtection          map[string]*BranchProtection
	auditLog                  []*AuditEntry
	auditLogEvents            []*AuditLogEvent
	marketplaceListings       map[string]*MarketplaceListing
	marketplacePlans          map[int]*MarketplacePlan
	marketplacePurchases      map[string]*MarketplacePurchase
	marketplaceDeliveries     map[string][]*WebhookDelivery
	nextMarketplaceDeliveryID int
	nextMarketplacePlanID     int
	oidcClaimKeys             []string
	nextKeyID                 int
	nextGPGKeyID              int
	nextPagesBuildID          int64
	nextAuditID               int64
	nextAdminAuditID          int64
	oidcKey                   *rsa.PrivateKey
	persist                   *Persistence
	blockedUsers              map[int]map[int]bool // userID -> targetID -> blocked
	socialAccounts            map[int][]map[string]interface{}
	sshSigningKeys            map[int][]map[string]interface{}
	nextSSHSigningKeyID       int
}

func newMiscStore() *MiscStore {
	return &MiscStore{
		userKeys:                  map[int]*UserKey{},
		keysByUser:                map[int][]*UserKey{},
		gpgKeys:                   map[int]*GPGKey{},
		gpgKeysByUser:             map[int][]*GPGKey{},
		follows:                   map[string]map[string]bool{},
		pagesByRepo:               map[int]*PagesSite{},
		pagesBuilds:               map[string][]*PagesBuild{},
		branchProtection:          map[string]*BranchProtection{},
		marketplaceListings:       map[string]*MarketplaceListing{},
		marketplacePlans:          map[int]*MarketplacePlan{},
		marketplacePurchases:      map[string]*MarketplacePurchase{},
		marketplaceDeliveries:     map[string][]*WebhookDelivery{},
		nextMarketplaceDeliveryID: 1,
		nextMarketplacePlanID:     1,
		auditLogEvents:            []*AuditLogEvent{},
		nextKeyID:                 1,
		nextGPGKeyID:              1,
		nextPagesBuildID:          1,
		blockedUsers:              map[int]map[int]bool{},
		socialAccounts:            map[int][]map[string]interface{}{},
		sshSigningKeys:            map[int][]map[string]interface{}{},
		nextSSHSigningKeyID:       1,
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
		out = append(out, userKeyToJSON(k, s.baseURL(r)))
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
	writeJSON(w, http.StatusCreated, userKeyToJSON(k, s.baseURL(r)))
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
	writeJSON(w, http.StatusOK, userKeyToJSON(k, s.baseURL(r)))
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
	emails := s.store.ListUserEmails(user.ID)
	out := make([]map[string]interface{}, len(emails))
	for i, e := range emails {
		out[i] = userEmailJSON(e)
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
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

// resolveLoginsJSON converts a list of logins to user JSON, skipping logins
// with no store user. Must not be called with Misc.mu held: LookupUserByLogin
// takes Store.mu, and Store.mu is never acquired under Misc.mu (the lock
// order is Store.mu before Misc.mu).
func (s *Server) resolveLoginsJSON(logins []string) []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, login := range logins {
		if u := s.store.LookupUserByLogin(login); u != nil {
			out = append(out, userToJSON(u))
		}
	}
	return out
}

// followerLogins returns the logins that follow target. Gathered under
// Misc.mu; the caller resolves logins to users after release.
func (s *Server) followerLogins(target string) []string {
	s.store.Misc.mu.RLock()
	defer s.store.Misc.mu.RUnlock()
	var logins []string
	for user, follows := range s.store.Misc.follows {
		if follows[target] {
			logins = append(logins, user)
		}
	}
	return logins
}

// followingLogins returns the logins that login follows. Gathered under
// Misc.mu; the caller resolves logins to users after release.
func (s *Server) followingLogins(login string) []string {
	s.store.Misc.mu.RLock()
	defer s.store.Misc.mu.RUnlock()
	var logins []string
	for target := range s.store.Misc.follows[login] {
		logins = append(logins, target)
	}
	return logins
}

func (s *Server) handleListFollowers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.resolveLoginsJSON(s.followerLogins(r.PathValue("username"))))
}
func (s *Server) handleListFollowing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.resolveLoginsJSON(s.followingLogins(r.PathValue("username"))))
}
func (s *Server) handleListMyFollowers(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusOK, []map[string]interface{}{})
		return
	}
	writeJSON(w, http.StatusOK, s.resolveLoginsJSON(s.followerLogins(user.Login)))
}
func (s *Server) handleListMyFollowing(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusOK, []map[string]interface{}{})
		return
	}
	writeJSON(w, http.StatusOK, s.resolveLoginsJSON(s.followingLogins(user.Login)))
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
		// Missing/unresolvable run context is a client-side error, not a panic.
		writeGHError(w, http.StatusBadRequest, err.Error())
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
	key, err := s.oidcKeyE()
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
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

func (s *Server) oidcKeyE() (*rsa.PrivateKey, error) {
	s.store.Misc.mu.Lock()
	defer s.store.Misc.mu.Unlock()
	if s.store.Misc.oidcKey != nil {
		return s.store.Misc.oidcKey, nil
	}
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate OpenID Connect signing key: %w", err)
	}
	s.store.Misc.oidcKey = k
	return k, nil
}

func (s *Server) mintOIDCToken(r *http.Request, audience string) (string, error) {
	now := time.Now()
	q := r.URL.Query()
	// An OIDC token is minted FOR a specific workflow run; GitHub derives every
	// claim from that run and never invents one. bleephub conveys the run
	// context on the token request — a missing field means there is no real run
	// to mint for, so we fail loudly rather than fabricate a placeholder claim
	// (a fabricated repository/ref/run_id would silently defeat OIDC trust
	// policies, which is worse than an error).
	repoFull := q.Get("repo")
	if repoFull == "" {
		return "", fmt.Errorf("oidc: 'repo' (owner/name) is required")
	}
	owner, repoName := splitRepoFull(repoFull)
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		return "", fmt.Errorf("oidc: repository %q not found", repoFull)
	}
	ref := q.Get("ref")
	if ref == "" {
		return "", fmt.Errorf("oidc: 'ref' is required")
	}
	sha := q.Get("sha")
	if sha == "" {
		return "", fmt.Errorf("oidc: 'sha' is required")
	}
	runID := q.Get("run_id")
	if runID == "" {
		return "", fmt.Errorf("oidc: 'run_id' is required")
	}
	runNumber := q.Get("run_number")
	if runNumber == "" {
		return "", fmt.Errorf("oidc: 'run_number' is required")
	}
	workflowName := q.Get("workflow")
	if workflowName == "" {
		return "", fmt.Errorf("oidc: 'workflow' is required")
	}
	workflowFile := q.Get("workflow_file")
	if workflowFile == "" {
		return "", fmt.Errorf("oidc: 'workflow_file' is required")
	}
	eventName := q.Get("event_name")
	if eventName == "" {
		return "", fmt.Errorf("oidc: 'event_name' is required")
	}
	// The actor is the authenticated user who triggered the run. /token sits
	// outside the /api middleware, so resolve the caller's token directly.
	user := ghUserFromContext(s.authenticateRequest(r))
	if user == nil {
		return "", fmt.Errorf("oidc: unauthenticated — no actor for the token")
	}
	actor := user.Login
	actorID := user.ID

	repoID := repo.ID
	ownerID := repo.OwnerID
	visibility := repo.Visibility
	if visibility == "" {
		if repo.Private {
			visibility = "private"
		} else {
			visibility = "public"
		}
	}

	// run_attempt: "1" is the real value for a first (non-rerun) attempt, not a
	// placeholder — GitHub omits it from no run.
	runAttempt := q.Get("run_attempt")
	if runAttempt == "" {
		runAttempt = "1"
	}
	headRef := q.Get("head_ref")
	baseRef := q.Get("base_ref")

	// ref_type derives from the ref form (refs/heads → branch, refs/tags → tag).
	refType := "branch"
	switch {
	case strings.HasPrefix(ref, "refs/tags/"):
		refType = "tag"
	case strings.HasPrefix(ref, "refs/heads/"):
		refType = "branch"
	}

	env := q.Get("environment")

	// sub reflects the environment when one is supplied, else the ref form —
	// matching real GitHub's OIDC subject construction.
	var sub string
	if env != "" {
		sub = "repo:" + repoFull + ":environment:" + env
	} else if eventName == "pull_request" {
		sub = "repo:" + repoFull + ":pull_request"
	} else {
		sub = "repo:" + repoFull + ":ref:" + ref
	}

	workflowRef := repoFull + "/.github/workflows/" + workflowFile + "@" + ref
	jobWorkflowRef := workflowRef

	jtiBytes, err := randomBytes(12)
	if err != nil {
		return "", fmt.Errorf("generate OpenID Connect token id: %w", err)
	}

	payload := map[string]interface{}{
		"iss":                   s.baseURL(r),
		"aud":                   audience,
		"sub":                   sub,
		"iat":                   now.Unix(),
		"nbf":                   now.Unix(),
		"exp":                   now.Add(5 * time.Minute).Unix(),
		"jti":                   base64.RawURLEncoding.EncodeToString(jtiBytes),
		"ref":                   ref,
		"ref_type":              refType,
		"repository":            repoFull,
		"repository_id":         strconv.Itoa(repoID),
		"repository_owner":      owner,
		"repository_owner_id":   strconv.Itoa(ownerID),
		"repository_visibility": visibility,
		"run_id":                runID,
		"run_number":            runNumber,
		"run_attempt":           runAttempt,
		"sha":                   sha,
		"actor":                 actor,
		"actor_id":              strconv.Itoa(actorID),
		"workflow":              workflowName,
		"workflow_ref":          workflowRef,
		"workflow_sha":          sha,
		"job_workflow_ref":      jobWorkflowRef,
		"job_workflow_sha":      sha,
		"head_ref":              headRef,
		"base_ref":              baseRef,
		"event_name":            eventName,
		"runner_environment":    "github-hosted",
		"environment":           env,
	}
	key, err := s.oidcKeyE()
	if err != nil {
		return "", err
	}
	return signRS256JWT(payload, key, "bleephub-oidc")
}

// splitRepoFull splits an "owner/repo" full name into its owner and repo
// segments. A bare value (no slash) is treated as the repo with no owner.
func splitRepoFull(full string) (owner, repo string) {
	if i := strings.IndexByte(full, '/'); i >= 0 {
		return full[:i], full[i+1:]
	}
	return "", full
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

func (st *Store) HasPagesSite(repoID int) bool {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	return st.Misc.pagesByRepo[repoID] != nil
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
		CNAME     string `json:"cname"`
		BuildType string `json:"build_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	buildType := coalesceStr(req.BuildType, "legacy")
	sourcePath := coalesceStr(req.Source.Path, "/")
	if err := s.validatePagesConfiguration(repo, buildType, req.Source.Branch, sourcePath); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	// repo.Owner is an invariant (set at create, relinked on load); use it
	// directly rather than guessing an owner.
	ownerLogin := repo.Owner.Login
	pages := &PagesSite{
		CNAME:   req.CNAME,
		URL:     s.baseURL(r) + "/" + repo.FullName + "/pages",
		HTMLURL: s.baseURL(r) + "/pages/" + ownerLogin + "/" + repo.Name + "/",
		Status:  "building",
		Source: map[string]interface{}{
			// branch is required+validated for legacy/branch builds above; for
			// a workflow build it is legitimately empty (not a fabricated "main").
			"branch": req.Source.Branch,
			"path":   sourcePath,
		},
		Public:    !repo.Private,
		Custom404: false,
		BuildType: &buildType,
	}
	s.store.Misc.mu.Lock()
	s.store.Misc.pagesByRepo[repo.ID] = pages
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("pages_sites", strconv.Itoa(repo.ID), pages)
	}
	s.store.Misc.mu.Unlock()
	writeJSON(w, http.StatusCreated, pages)
}

// handlePagesUpdate persists the documented update params and returns 204 No
// Content (GitHub's PUT /pages response), unlike create which returns 201+body.
func (s *Server) handlePagesUpdate(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		CNAME         *string `json:"cname"`
		HTTPSEnforced *bool   `json:"https_enforced"`
		BuildType     *string `json:"build_type"`
		Public        *bool   `json:"public"`
		Source        *struct {
			Branch string `json:"branch"`
			Path   string `json:"path"`
		} `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	s.store.Misc.mu.Lock()
	pages := s.store.Misc.pagesByRepo[repo.ID]
	if pages == nil {
		s.store.Misc.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	buildType := "legacy"
	if pages.BuildType != nil {
		buildType = *pages.BuildType
	}
	if req.BuildType != nil {
		buildType = *req.BuildType
	}
	branch, _ := pages.Source["branch"].(string)
	sourcePath, _ := pages.Source["path"].(string)
	if req.Source != nil {
		branch = req.Source.Branch
		sourcePath = req.Source.Path
	}
	s.store.Misc.mu.Unlock()
	if err := s.validatePagesConfiguration(repo, buildType, branch, sourcePath); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	s.store.Misc.mu.Lock()
	pages = s.store.Misc.pagesByRepo[repo.ID]
	if pages == nil {
		s.store.Misc.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if req.CNAME != nil {
		pages.CNAME = *req.CNAME
	}
	if req.HTTPSEnforced != nil {
		pages.HTTPSEnforced = *req.HTTPSEnforced
	}
	if req.BuildType != nil {
		bt := *req.BuildType
		pages.BuildType = &bt
	}
	if req.Public != nil {
		pages.Public = *req.Public
	}
	if req.Source != nil {
		if pages.Source == nil {
			pages.Source = map[string]interface{}{}
		}
		if req.Source.Branch != "" {
			pages.Source["branch"] = req.Source.Branch
		}
		if req.Source.Path != "" {
			pages.Source["path"] = req.Source.Path
		}
	}
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("pages_sites", strconv.Itoa(repo.ID), pages)
	}
	s.store.Misc.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) validatePagesConfiguration(repo *Repo, buildType, branch, sourcePath string) error {
	if buildType != "legacy" && buildType != "workflow" {
		return fmt.Errorf("invalid request: build_type must be legacy or workflow")
	}
	if sourcePath == "" {
		sourcePath = "/"
	}
	if sourcePath != "/" && sourcePath != "/docs" {
		return fmt.Errorf("invalid request: source.path must be / or /docs")
	}
	if buildType == "legacy" {
		if branch == "" {
			return fmt.Errorf("invalid request: source.branch is required for legacy Pages builds")
		}
		if resolveBranchSha(s.store.GetGitStorage(repo.Owner.Login, repo.Name), branch) == "" {
			return fmt.Errorf("invalid request: Pages source branch %q does not exist", branch)
		}
	}
	return nil
}

func (s *Server) handlePagesDelete(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if err := s.store.deletePagesPublicationData(r.Context(), repo.ID); err != nil {
		writeGHError(w, http.StatusInternalServerError, "Pages deletion failed: "+err.Error())
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
	var pusher *PagesPusher
	if user := ghUserFromContext(r.Context()); user != nil {
		actor = user.Login
		pusher = &PagesPusher{Login: user.Login, ID: user.ID, Type: coalesceStr(user.Type, "User")}
	}
	_, ok := s.runPagesBuild(r.Context(), repo, pusher, actor, s.baseURL(r))
	if !ok {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	// GitHub's request-a-build response is exactly {status, url}.
	writeJSON(w, http.StatusCreated, map[string]interface{}{"status": "queued", "url": s.baseURL(r) + "/api/v3/repos/" + repo.FullName + "/pages/builds/latest"})
}

func (s *Server) runPagesBuild(ctx context.Context, repo *Repo, pusher *PagesPusher, actor, baseURL string) (*PagesBuild, bool) {
	now := time.Now()
	s.store.Misc.mu.Lock()
	pages := s.store.Misc.pagesByRepo[repo.ID]
	if pages == nil {
		s.store.Misc.mu.Unlock()
		return nil, false
	}
	buildID := s.store.Misc.nextPagesBuildID
	s.store.Misc.nextPagesBuildID++
	buildURL := baseURL + "/api/v3/repos/" + repo.FullName + "/pages/builds/" + strconv.FormatInt(buildID, 10)
	build := &PagesBuild{
		ID:        buildID,
		URL:       buildURL,
		Status:    "queued",
		Pusher:    pusher,
		CreatedAt: now,
		UpdatedAt: now,
		Error:     &PagesBuildErr{},
	}
	s.store.Misc.pagesBuilds[repo.FullName] = append([]*PagesBuild{build}, s.store.Misc.pagesBuilds[repo.FullName]...)
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("pages_builds", repo.FullName, s.store.Misc.pagesBuilds[repo.FullName])
	}
	branch, sourcePath, sourceErr := pagesLegacySource(pages)
	s.store.Misc.mu.Unlock()
	buildStarted := time.Now()
	commitSHA := ""
	custom404 := false
	buildErr := sourceErr
	if buildErr == nil {
		commitSHA, custom404, buildErr = s.buildPagesBranch(ctx, repo, branch, sourcePath)
	}
	finishedAt := time.Now()
	s.store.Misc.mu.Lock()
	build.Commit = commitSHA
	build.UpdatedAt = finishedAt
	build.Duration = int(finishedAt.Sub(buildStarted).Milliseconds())
	if buildErr != nil {
		message := buildErr.Error()
		build.Status = "errored"
		build.Error.Message = &message
		pages.Status = "errored"
	} else {
		build.Status = "built"
		pages.Status = "built"
		pages.Custom404 = custom404
	}
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("pages_builds", repo.FullName, s.store.Misc.pagesBuilds[repo.FullName])
		s.store.Misc.persist.MustPut("pages_sites", strconv.Itoa(repo.ID), pages)
	}
	s.store.Misc.mu.Unlock()
	s.recordAuditEvent("pages.build", actor, "", map[string]interface{}{"repo": repo.FullName, "build_id": buildID})
	return build, true
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

// --- Orgs depth ---

func (s *Server) handleOrgAuditLog(w http.ResponseWriter, r *http.Request) {
	orgName := r.PathValue("org")

	// The audit log exposes secret-name changes, hook-config edits and actor
	// identities; real GitHub restricts it to org owners (read:audit_log /
	// admin:org). Anyone else gets 404 (GitHub hides existence from non-admins).
	user := ghUserFromContext(r.Context())
	org := s.store.GetOrg(orgName)
	if user == nil || org == nil || !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.Misc.mu.RLock()
	entries := make([]*AuditEntry, 0, len(s.store.Misc.auditLog))
	order := r.URL.Query().Get("order")
	if order != "" && order != "desc" && order != "asc" {
		s.store.Misc.mu.RUnlock()
		writeGHValidationError(w, "AuditLog", "order", "invalid")
		return
	}
	for _, e := range s.store.Misc.auditLog {
		if e.Org != "" && e.Org != orgName {
			continue
		}
		if phrase := r.URL.Query().Get("phrase"); phrase != "" {
			if !auditEntryMatchesPhrase(e, phrase) {
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
	if order == "asc" {
		for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
			entries[i], entries[j] = entries[j], entries[i]
		}
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, entries))
}

func auditEntryMatchesPhrase(e *AuditEntry, phrase string) bool {
	terms := strings.Fields(strings.ToLower(phrase))
	if len(terms) == 0 {
		return true
	}
	text := strings.ToLower(strings.Join([]string{e.Action, e.Actor, e.Org}, " "))
	if len(e.Data) > 0 {
		if b, err := json.Marshal(e.Data); err == nil {
			text += " " + strings.ToLower(string(b))
		}
	}
	for _, term := range terms {
		if !strings.Contains(text, term) {
			return false
		}
	}
	return true
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
	if len(s.store.Misc.auditLog) > maxAuditLogEntries {
		s.store.Misc.auditLog = s.store.Misc.auditLog[:maxAuditLogEntries]
	}
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("audit_log", fmt.Sprintf("%d", entry.ID), entry)
	}
}

// maxAuditLogEntries bounds the in-memory audit log. Real GitHub retains audit
// events for a finite window (Enterprise default ≈6 months); an uncapped
// prepend-only slice both grows without limit and makes each write O(n). The
// cap keeps the newest entries and bounds both.
const maxAuditLogEntries = 5000

// marketplacePlanToJSON renders the spec `marketplace-listing-plan` shape.
func marketplacePlanToJSON(p *MarketplacePlan, baseURL string) map[string]interface{} {
	api := baseURL + "/api/v3/marketplace_listing/plans/" + strconv.Itoa(p.ID)
	return map[string]interface{}{
		"url":                    api,
		"accounts_url":           api + "/accounts",
		"id":                     p.ID,
		"number":                 p.Number,
		"name":                   p.Name,
		"description":            p.Description,
		"monthly_price_in_cents": p.MonthlyPriceInCents,
		"yearly_price_in_cents":  p.YearlyPriceInCents,
		"price_model":            p.PriceModel,
		"has_free_trial":         p.HasFreeTrial,
		"unit_name":              nullOrString(p.UnitName),
		"state":                  p.State,
		"bullets":                append([]string{}, p.Bullets...),
	}
}

// marketplaceAccountJSON renders the spec `marketplace-purchase` shape.
// The account is a real user or organization from the store.
func (s *Server) marketplaceAccountJSON(purchase *MarketplacePurchase, plan *MarketplacePlan, baseURL string) map[string]interface{} {
	accountType := purchase.AccountType
	login := ""
	var email interface{}
	if accountType == "Organization" {
		if org := s.store.GetOrgByID(purchase.AccountID); org != nil {
			login = org.Login
			email = nullOrString(org.Email)
		}
	} else if u := s.store.GetUserByID(purchase.AccountID); u != nil {
		login = u.Login
		email = nullOrString(u.Email)
	}
	var freeTrialEnds interface{}
	if purchase.FreeTrialEnds != nil {
		freeTrialEnds = purchase.FreeTrialEnds.UTC().Format(time.RFC3339)
	}
	var nextBillingDate, updatedAt interface{}
	if purchase.NextBillingDate != nil {
		nextBillingDate = purchase.NextBillingDate.UTC().Format(time.RFC3339)
	}
	if purchase.UpdatedAt != nil {
		updatedAt = purchase.UpdatedAt.UTC().Format(time.RFC3339)
	}
	var pendingChange interface{}
	if purchase.PendingChange != nil {
		pendingRow := map[string]interface{}{
			"effective_date": purchase.PendingChange.EffectiveDate.UTC().Format(time.RFC3339),
			"billing_cycle":  nullOrString(purchase.PendingChange.BillingCycle),
			"unit_count":     purchase.PendingChange.UnitCount,
			"cancellation":   purchase.PendingChange.Cancellation,
		}
		if purchase.PendingChange.PlanID != 0 {
			if pendingPlan := s.store.GetMarketplacePlanForListing(purchase.ListingSlug, purchase.PendingChange.PlanID); pendingPlan != nil {
				pendingRow["plan"] = marketplacePlanToJSON(pendingPlan, baseURL)
			}
		}
		pendingChange = pendingRow
	}
	return map[string]interface{}{
		"url":                        baseURL + "/api/v3/users/" + login,
		"type":                       accountType,
		"id":                         purchase.AccountID,
		"login":                      login,
		"email":                      email,
		"marketplace_pending_change": pendingChange,
		"marketplace_purchase": map[string]interface{}{
			"billing_cycle":      purchase.BillingCycle,
			"next_billing_date":  nextBillingDate,
			"is_installed":       purchase.InstallationID != nil,
			"unit_count":         purchase.UnitCount,
			"on_free_trial":      purchase.OnFreeTrial,
			"free_trial_ends_on": freeTrialEnds,
			"updated_at":         updatedAt,
			"plan":               marketplacePlanToJSON(plan, baseURL),
		},
	}
}

func (s *Server) handleMarketplacePlans(w http.ResponseWriter, r *http.Request) {
	listing := s.marketplaceListingForPublisher(w, r)
	if listing == nil {
		return
	}
	base := s.baseURL(r)
	plans := s.store.ListMarketplacePlans(listing.Slug, false)
	out := make([]map[string]interface{}, 0, len(plans))
	for _, p := range plans {
		out = append(out, marketplacePlanToJSON(p, base))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleMarketplaceAccount(w http.ResponseWriter, r *http.Request) {
	listing := s.marketplaceListingForPublisher(w, r)
	if listing == nil {
		return
	}
	s.reconcileMarketplacePurchases(listing.Slug)
	accountID, _ := strconv.Atoi(r.PathValue("account_id"))
	var purchase *MarketplacePurchase
	for _, candidate := range s.store.ListMarketplacePurchasesForListing(listing.Slug) {
		if candidate.AccountID == accountID {
			if purchase != nil {
				writeGHError(w, http.StatusConflict, "Multiple account types share this identifier")
				return
			}
			purchase = candidate
		}
	}
	var plan *MarketplacePlan
	if purchase != nil {
		plan = s.store.GetMarketplacePlanForListing(listing.Slug, purchase.PlanID)
	}
	if purchase == nil || plan == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.marketplaceAccountJSON(purchase, plan, s.baseURL(r)))
}

// handleMarketplacePlanAccounts implements GET
// /marketplace_listing/plans/{plan_id}/accounts (and its stubbed variant):
// the accounts holding an active purchase of the plan.
func (s *Server) handleMarketplacePlanAccounts(w http.ResponseWriter, r *http.Request) {
	listing := s.marketplaceListingForPublisher(w, r)
	if listing == nil {
		return
	}
	s.reconcileMarketplacePurchases(listing.Slug)
	planID, _ := strconv.Atoi(r.PathValue("plan_id"))
	plan := s.store.GetMarketplacePlanForListing(listing.Slug, planID)
	purchases := make([]*MarketplacePurchase, 0)
	for _, pu := range s.store.ListMarketplacePurchasesForListing(listing.Slug) {
		if pu.PlanID == planID {
			purchases = append(purchases, pu)
		}
	}
	if plan == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	direction := r.URL.Query().Get("direction")
	sort.Slice(purchases, func(i, j int) bool {
		var ti, tj time.Time
		if purchases[i].UpdatedAt != nil {
			ti = *purchases[i].UpdatedAt
		}
		if purchases[j].UpdatedAt != nil {
			tj = *purchases[j].UpdatedAt
		}
		if direction == "desc" {
			return ti.After(tj)
		}
		return ti.Before(tj)
	})

	page := paginateAndLink(w, r, purchases)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(page))
	for _, pu := range page {
		out = append(out, s.marketplaceAccountJSON(pu, plan, base))
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Helpers ---

func userKeyToJSON(k *UserKey, baseURL string) map[string]interface{} {
	return map[string]interface{}{
		"id":         k.ID,
		"url":        baseURL + "/api/v3/user/keys/" + strconv.Itoa(k.ID),
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
