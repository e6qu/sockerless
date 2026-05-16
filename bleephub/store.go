package bleephub

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/storage/memory"
)

// loadJSON is a thin wrapper to keep error wrapping uniform across persistence loaders.
func loadJSON(raw []byte, v interface{}) error { return json.Unmarshal(raw, v) }

// User represents a GitHub user account.
type User struct {
	ID        int       `json:"id"`
	NodeID    string    `json:"node_id"`
	Login     string    `json:"login"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	AvatarURL string    `json:"avatar_url"`
	Bio       string    `json:"bio"`
	Type      string    `json:"type"`
	SiteAdmin bool      `json:"site_admin"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Token represents a personal access token.
type Token struct {
	Value     string
	UserID    int
	Scopes    string
	CreatedAt time.Time
}

// DeviceCode represents a pending device authorization flow.
type DeviceCode struct {
	Code      string
	UserCode  string
	Scopes    string
	Token     string // pre-generated token value
	UserID    int
	ExpiresAt time.Time
}

// Store holds all in-memory state for bleephub.
type Store struct {
	Agents             map[int]*Agent
	Sessions           map[string]*Session
	Jobs               map[string]*Job
	Users              map[int]*User
	UsersByLogin       map[string]*User
	Tokens             map[string]*Token
	DeviceCodes        map[string]*DeviceCode
	AuthCodes          map[string]*authCode // OAuth web-flow codes (Phase 132)
	Repos              map[int]*Repo
	ReposByName        map[string]*Repo              // "owner/name" → repo
	GitStorages        map[string]*memory.Storage    // "owner/name" → go-git memory storage
	Orgs               map[int]*Org                  // id → org
	OrgsByLogin        map[string]*Org               // login → org
	Teams              map[int]*Team                 // id → team
	TeamsBySlug        map[string]*Team              // "org/slug" → team
	Memberships        map[string]*Membership        // "org/user" → membership
	Issues             map[int]*Issue                // id → issue
	Labels             map[int]*IssueLabel           // id → label
	Milestones         map[int]*Milestone            // id → milestone
	Comments           map[int]*Comment              // id → comment
	PullRequests       map[int]*PullRequest          // id → PR
	PRReviews          map[int]*PullRequestReview    // id → review
	Workflows          map[string]*Workflow          // id → workflow (run-level)
	WorkflowFiles      map[int64]*WorkflowFile       // id → workflow file (file-level, Phase 131)
	PendingMessages    []*TaskAgentMessage           // messages awaiting delivery
	RepoSecrets        map[string]map[string]*Secret // "owner/repo" → name → secret
	Hooks              map[string][]*Webhook         // "owner/repo" → hooks
	HookDeliveries     map[int][]*WebhookDelivery    // hookID → deliveries
	Apps               map[int]*App                  // id → app
	AppsBySlug         map[string]*App               // slug → app
	AppsByClientID     map[string]*App               // OAuth client_id → app
	OAuthApps          map[string]*OAuthApp          // OAuth client_id → OAuth app (distinct from GitHub App)
	Installations      map[int]*Installation         // id → installation
	InstallationTokens map[string]*InstallationToken // token value → token
	UserToServerTokens map[string]*UserToServerToken // gho_/ghu_ token value → token
	RefreshTokens      map[string]*RefreshToken      // ghr_ token value → refresh token
	AppHookDeliveries  map[int][]*WebhookDelivery    // appID → app-level webhook deliveries
	ManifestCodes      map[string]int                // code → appID (one-time-use)
	CheckRuns          map[int64]*CheckRun           // id → check run
	CheckSuites        map[int64]*CheckSuite         // id → check suite
	CheckSuitePrefs    map[string][]*CheckSuitePref  // repoKey → autoTrigger prefs
	Reactions          *ReactionStore                // P154.1 — reactions across all parent types
	Releases           *ReleaseStore                 // P154.2 — release CRUD
	Deployments        *DeploymentStore              // P154.4 — deployments + statuses + environments
	PRReviewComments   *PRReviewCommentStore         // P154.5 — PR review comments (inline / threads)
	Misc               *MiscStore                    // P154.7-12 — long-tail surfaces
	LogLines           map[string][]string           // jobID → captured console log lines
	NextAgent          int
	NextMsg            int64
	NextLog            int
	NextReqID          int64
	NextUser           int
	NextRepo           int
	NextOrg            int
	NextTeam           int
	NextIssue          int
	NextLabel          int
	NextMilestone      int
	NextComment        int
	NextPR             int
	NextPRReview       int
	NextRunID          int
	NextHookID         int
	NextDeliveryID     int
	NextAppID          int
	NextInstallationID int
	NextCheckRunID     int64
	NextCheckSuiteID   int64
	persist            *Persistence
	mu                 sync.RWMutex
}

// Agent represents a registered runner agent.
type Agent struct {
	ID             int                 `json:"id"`
	Name           string              `json:"name"`
	Version        string              `json:"version"`
	Enabled        bool                `json:"enabled"`
	Status         string              `json:"status"`
	OSDescription  string              `json:"osDescription"`
	Labels         []Label             `json:"labels"`
	Authorization  *AgentAuthorization `json:"authorization,omitempty"`
	Ephemeral      bool                `json:"ephemeral,omitempty"`
	MaxParallelism int                 `json:"maxParallelism,omitempty"`
	ProvisionState string              `json:"provisioningState,omitempty"`
	CreatedOn      time.Time           `json:"createdOn"`
}

// Label is an agent label.
type Label struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// AgentAuthorization holds the agent's RSA public key and auth URL.
type AgentAuthorization struct {
	AuthorizationURL string          `json:"authorizationUrl,omitempty"`
	ClientID         string          `json:"clientId,omitempty"`
	PublicKey        *AgentPublicKey `json:"publicKey,omitempty"`
}

// AgentPublicKey is the RSA public key components.
type AgentPublicKey struct {
	Exponent string `json:"exponent"`
	Modulus  string `json:"modulus"`
}

// Session represents a runner's active session.
type Session struct {
	SessionID string                 `json:"sessionId"`
	OwnerName string                 `json:"ownerName"`
	Agent     *Agent                 `json:"agent"`
	MsgCh     chan *TaskAgentMessage `json:"-"`
}

// TaskAgentMessage is the message envelope sent to the runner.
type TaskAgentMessage struct {
	MessageID   int64  `json:"messageId"`
	MessageType string `json:"messageType"`
	IV          string `json:"iv,omitempty"`
	Body        string `json:"body"`
}

// Job represents a queued/running/completed job.
type Job struct {
	ID          string    `json:"id"`
	RequestID   int64     `json:"requestId"`
	PlanID      string    `json:"planId"`
	TimelineID  string    `json:"timelineId"`
	Status      string    `json:"status"` // queued, running, completed
	Result      string    `json:"result"` // Succeeded, Failed, Cancelled
	Message     string    `json:"-"`      // JSON-encoded job request message
	LockedUntil time.Time `json:"lockedUntil"`
	AgentID     int       `json:"agentId"`
}

// NewStore creates an initialized store.
func NewStore() *Store {
	return &Store{
		Agents:             make(map[int]*Agent),
		Sessions:           make(map[string]*Session),
		Jobs:               make(map[string]*Job),
		Users:              make(map[int]*User),
		UsersByLogin:       make(map[string]*User),
		Tokens:             make(map[string]*Token),
		DeviceCodes:        make(map[string]*DeviceCode),
		AuthCodes:          make(map[string]*authCode),
		Repos:              make(map[int]*Repo),
		ReposByName:        make(map[string]*Repo),
		GitStorages:        make(map[string]*memory.Storage),
		Orgs:               make(map[int]*Org),
		OrgsByLogin:        make(map[string]*Org),
		Teams:              make(map[int]*Team),
		TeamsBySlug:        make(map[string]*Team),
		Memberships:        make(map[string]*Membership),
		Issues:             make(map[int]*Issue),
		Labels:             make(map[int]*IssueLabel),
		Milestones:         make(map[int]*Milestone),
		Comments:           make(map[int]*Comment),
		PullRequests:       make(map[int]*PullRequest),
		PRReviews:          make(map[int]*PullRequestReview),
		Workflows:          make(map[string]*Workflow),
		WorkflowFiles:      make(map[int64]*WorkflowFile),
		RepoSecrets:        make(map[string]map[string]*Secret),
		Hooks:              make(map[string][]*Webhook),
		HookDeliveries:     make(map[int][]*WebhookDelivery),
		Apps:               make(map[int]*App),
		AppsBySlug:         make(map[string]*App),
		AppsByClientID:     make(map[string]*App),
		OAuthApps:          make(map[string]*OAuthApp),
		Installations:      make(map[int]*Installation),
		InstallationTokens: make(map[string]*InstallationToken),
		UserToServerTokens: make(map[string]*UserToServerToken),
		RefreshTokens:      make(map[string]*RefreshToken),
		AppHookDeliveries:  make(map[int][]*WebhookDelivery),
		ManifestCodes:      make(map[string]int),
		CheckRuns:          make(map[int64]*CheckRun),
		CheckSuites:        make(map[int64]*CheckSuite),
		CheckSuitePrefs:    make(map[string][]*CheckSuitePref),
		Reactions:          newReactionStore(),
		Releases:           newReleaseStore(),
		Deployments:        newDeploymentStore(),
		PRReviewComments:   newPRReviewCommentStore(),
		Misc:               newMiscStore(),
		LogLines:           make(map[string][]string),
		NextAgent:          1,
		NextMsg:            1,
		NextLog:            1,
		NextReqID:          1,
		NextUser:           1,
		NextRepo:           1,
		NextOrg:            1,
		NextTeam:           1,
		NextIssue:          1,
		NextLabel:          1,
		NextMilestone:      1,
		NextComment:        1,
		NextPR:             1,
		NextPRReview:       1,
		NextRunID:          1,
		NextHookID:         1,
		NextDeliveryID:     1,
		NextAppID:          1,
		NextInstallationID: 1,
		NextCheckRunID:     1,
		NextCheckSuiteID:   1,
	}
}

// SetPersistence wires a Persistence layer onto the Store. Call once at
// startup before any concurrent access; subsequent Create/Update/Delete
// mutations will write through to the underlying SQLite db.
//
// If persist is non-nil, this also loads existing rows from disk into the
// in-memory maps. Idempotent — safe to call against an empty database.
//
// BUG-985 invariant: open-failure must be caught at the persistence-open
// site (MustNewPersistence) so the operator gets a fail-loud signal
// before we even get here.
func (st *Store) SetPersistence(p *Persistence) error {
	if p == nil {
		return nil
	}
	st.mu.Lock()
	st.persist = p
	st.mu.Unlock()
	return st.loadFromPersistence()
}

// loadFromPersistence repopulates the in-memory maps from disk.
//
// Loads buckets:
//
//	users, tokens, apps, oauth_apps, installations, installation_tokens,
//	user_to_server_tokens, refresh_tokens, repos.
//
// Other state (workflows, sessions, agents, ephemeral codes) deliberately
// stays in-memory only — operator restart implies abandoning in-flight runs.
func (st *Store) loadFromPersistence() error {
	if st.persist == nil {
		return nil
	}
	if err := st.loadBucket("users", func(raw []byte) error {
		var u User
		if err := loadJSON(raw, &u); err != nil {
			return err
		}
		st.Users[u.ID] = &u
		st.UsersByLogin[u.Login] = &u
		if u.ID >= st.NextUser {
			st.NextUser = u.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("tokens", func(raw []byte) error {
		var t Token
		if err := loadJSON(raw, &t); err != nil {
			return err
		}
		st.Tokens[t.Value] = &t
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("apps", func(raw []byte) error {
		var a App
		if err := loadJSON(raw, &a); err != nil {
			return err
		}
		st.Apps[a.ID] = &a
		st.AppsBySlug[a.Slug] = &a
		st.AppsByClientID[a.ClientID] = &a
		if a.ID >= st.NextAppID {
			st.NextAppID = a.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("oauth_apps", func(raw []byte) error {
		var a OAuthApp
		if err := loadJSON(raw, &a); err != nil {
			return err
		}
		st.OAuthApps[a.ClientID] = &a
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("installations", func(raw []byte) error {
		var inst Installation
		if err := loadJSON(raw, &inst); err != nil {
			return err
		}
		st.Installations[inst.ID] = &inst
		if inst.ID >= st.NextInstallationID {
			st.NextInstallationID = inst.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("installation_tokens", func(raw []byte) error {
		var t InstallationToken
		if err := loadJSON(raw, &t); err != nil {
			return err
		}
		st.InstallationTokens[t.Token] = &t
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("user_to_server_tokens", func(raw []byte) error {
		var t UserToServerToken
		if err := loadJSON(raw, &t); err != nil {
			return err
		}
		st.UserToServerTokens[t.Token] = &t
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("refresh_tokens", func(raw []byte) error {
		var t RefreshToken
		if err := loadJSON(raw, &t); err != nil {
			return err
		}
		st.RefreshTokens[t.Token] = &t
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("repos", func(raw []byte) error {
		var r Repo
		if err := loadJSON(raw, &r); err != nil {
			return err
		}
		// Owner is encoded as User pointer; ensure linkage to the loaded user.
		if r.Owner != nil {
			if owner := st.Users[r.Owner.ID]; owner != nil {
				r.Owner = owner
			}
		}
		st.Repos[r.ID] = &r
		st.ReposByName[r.FullName] = &r
		if r.ID >= st.NextRepo {
			st.NextRepo = r.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (st *Store) loadBucket(name string, fn func(raw []byte) error) error {
	rows, err := st.persist.List(name)
	if err != nil {
		return fmt.Errorf("load %s: %w", name, err)
	}
	for _, raw := range rows {
		if err := fn(raw); err != nil {
			return fmt.Errorf("decode %s row: %w", name, err)
		}
	}
	return nil
}

// SeedDefaultUser creates the default admin user and token.
func (st *Store) SeedDefaultUser() {
	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now()
	u := &User{
		ID:        st.NextUser,
		NodeID:    "U_kgDOBdefault",
		Login:     "admin",
		Name:      "Admin",
		Email:     "admin@bleephub.local",
		AvatarURL: "",
		Bio:       "",
		Type:      "User",
		SiteAdmin: true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	st.Users[u.ID] = u
	st.UsersByLogin[u.Login] = u
	st.NextUser++
	if st.persist != nil {
		st.persist.MustPut("users", fmt.Sprintf("%d", u.ID), u)
	}

	t := &Token{
		Value:     "bph_0000000000000000000000000000000000000000",
		UserID:    u.ID,
		Scopes:    "repo, read:org, gist",
		CreatedAt: now,
	}
	st.Tokens[t.Value] = t
	if st.persist != nil {
		st.persist.MustPut("tokens", t.Value, t)
	}
}

// LookupToken returns the token and associated user, or nil if not found.
func (st *Store) LookupToken(tokenStr string) (*Token, *User) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	t, ok := st.Tokens[tokenStr]
	if !ok {
		return nil, nil
	}
	return t, st.Users[t.UserID]
}

// LookupUserByLogin returns the user with the given login, or nil.
func (st *Store) LookupUserByLogin(login string) *User {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.UsersByLogin[login]
}

// CreateToken generates a new token for the given user.
func (st *Store) CreateToken(userID int, scopes string) *Token {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.createTokenLocked(userID, scopes)
}

// generateTokenValue creates a ghp_-prefixed random token string (classic PAT).
// Real GitHub uses ghp_ for classic PATs; bleephub matches the prefix so SDK
// clients that branch on prefix recognise the token shape. The seeded admin
// user keeps its bph_-prefixed token (see SeedDefaultUser) for backwards
// compatibility with existing tests + integrations.
func generateTokenValue() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return fmt.Sprintf("ghp_%s", hex.EncodeToString(b))
}
