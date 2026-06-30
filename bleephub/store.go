package bleephub

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	gitStorage "github.com/go-git/go-git/v5/storage"
)

// AdminToken returns the seeded admin token, which MUST be supplied via
// BLEEPHUB_ADMIN_TOKEN. There is no default: the token is a credential, so the
// sim fails loudly rather than seeding a guessable value (and a hardcoded value
// would be GitHub-PAT-shaped, tripping secret scanners). Consumers and test
// harnesses set the env var explicitly.
func AdminToken() string {
	v := os.Getenv("BLEEPHUB_ADMIN_TOKEN")
	if v == "" {
		log.Fatal("bleephub: BLEEPHUB_ADMIN_TOKEN is required (the admin token has no default — set it explicitly)")
	}
	return v
}

// loadJSON is a thin wrapper to keep error wrapping uniform across persistence loaders.
func loadJSON(raw []byte, v interface{}) error { return json.Unmarshal(raw, v) }

// ReserveRunID hands out the next workflow run ID and persists the
// counter. Artifacts persist on disk keyed by their run ID, so the
// sequence must never restart from 1 after a reload — a new run #1
// would inherit the previous epoch's run-1 artifacts.
func (st *Store) ReserveRunID() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	id := st.NextRunID
	st.NextRunID++
	if st.persist != nil {
		if err := st.persist.SetCounter("next_run_id", int64(st.NextRunID)); err != nil {
			log.Fatalf("bleephub persistence counter next_run_id failed: %v", err)
		}
	}
	return id
}

// User represents a GitHub user account.
type User struct {
	ID           int             `json:"id"`
	NodeID       string          `json:"node_id"`
	Login        string          `json:"login"`
	Name         string          `json:"name"`
	Email        string          `json:"email"`
	AvatarURL    string          `json:"avatar_url"`
	Bio          string          `json:"bio"`
	Type         string          `json:"type"`
	SiteAdmin    bool            `json:"site_admin"`
	StarredRepos map[string]bool `json:"starred_repos,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
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

// LoginSession is a browser session created by POST /login.
// It binds a session cookie to a user and carries the CSRF token
// embedded in the OAuth authorize consent form.
type LoginSession struct {
	UserID    int
	CSRFToken string
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
	AuthCodes          map[string]*authCode     // OAuth web-flow codes
	LoginSessions      map[string]*LoginSession // _gh_sess cookie value → session
	Repos              map[int]*Repo
	ReposByName        map[string]*Repo                       // "owner/name" → repo
	GitStorages        map[string]gitStorage.Storer           // "owner/name" → go-git storage (memory or filesystem)
	Orgs               map[int]*Org                           // id → org
	OrgsByLogin        map[string]*Org                        // login → org
	Teams              map[int]*Team                          // id → team
	TeamsBySlug        map[string]*Team                       // "org/slug" → team
	Memberships        map[string]*Membership                 // "org/user" → membership
	Issues             map[int]*Issue                         // id → issue
	Labels             map[int]*IssueLabel                    // id → label
	Milestones         map[int]*Milestone                     // id → milestone
	Comments           map[int]*Comment                       // id → comment
	PullRequests       map[int]*PullRequest                   // id → PR
	PRReviews          map[int]*PullRequestReview             // id → review
	Workflows          map[string]*Workflow                   // id → workflow (run-level)
	WorkflowFiles      map[int64]*WorkflowFile                // id → workflow file (file-level)
	PendingMessages    []*TaskAgentMessage                    // messages awaiting delivery
	RepoSecrets        map[string]map[string]*Secret          // "owner/repo" → name → secret
	RepoVariables      map[string]map[string]*ActionsVariable // "owner/repo" → NAME → variable
	RepoCollaborators  map[string]map[string]string           // "owner/repo" → login → permission (pull/push/admin)
	OrgSecrets         map[string]map[string]*OrgSecret       // org login → NAME → org secret
	OrgVariables       map[string]map[string]*ActionsVariable // org login → NAME → org variable
	EnvSecrets         map[string]map[string]*Secret          // envScopeKey(repo, env) → NAME → secret
	EnvVariables       map[string]map[string]*ActionsVariable // envScopeKey(repo, env) → NAME → variable
	TimelineRecords    map[string][]*TimelineRecord           // planID → runner-uploaded timeline records
	LogFiles           map[int][]byte                         // logID → uploaded runner log content
	WorkflowAttempts   map[int][]*Workflow                    // runID → prior attempts (oldest first)
	RunnerGroups       map[int]*RunnerGroup                   // org runner groups (global pool overlay)
	NextRunnerGroupID  int
	Hooks              map[string][]*Webhook         // "owner/repo" → hooks
	OrgHooks           map[string][]*Webhook         // org login → org-level hooks
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
	Reactions          *ReactionStore                // reactions across all parent types
	Releases           *ReleaseStore                 // release CRUD
	Deployments        *DeploymentStore              // deployments + statuses + environments
	PRReviewComments   *PRReviewCommentStore         // PR review comments (inline / threads)
	Misc               *MiscStore                    // long-tail surfaces
	ProjectsV2         *ProjectV2Store               // GitHub Projects v2
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
	actionsKeyPair     *SecretsKeyPair // lazily generated sealed-box keypair (persisted)
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
	RunnerGroupID  int                 `json:"runnerGroupId,omitempty"`
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
	// Labels carries the job's runs-on requirements for broker routing;
	// JobID links the envelope to its engine job so delivery can record
	// which agent took it. Neither is serialized to the runner.
	Labels []string `json:"-"`
	JobID  string   `json:"-"`
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
		LoginSessions:      make(map[string]*LoginSession),
		Repos:              make(map[int]*Repo),
		ReposByName:        make(map[string]*Repo),
		GitStorages:        make(map[string]gitStorage.Storer),
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
		RepoVariables:      make(map[string]map[string]*ActionsVariable),
		RepoCollaborators:  make(map[string]map[string]string),
		OrgSecrets:         make(map[string]map[string]*OrgSecret),
		OrgVariables:       make(map[string]map[string]*ActionsVariable),
		EnvSecrets:         make(map[string]map[string]*Secret),
		EnvVariables:       make(map[string]map[string]*ActionsVariable),
		TimelineRecords:    make(map[string][]*TimelineRecord),
		LogFiles:           make(map[int][]byte),
		WorkflowAttempts:   make(map[int][]*Workflow),
		RunnerGroups:       make(map[int]*RunnerGroup),
		NextRunnerGroupID:  2,
		Hooks:              make(map[string][]*Webhook),
		OrgHooks:           make(map[string][]*Webhook),
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
		Reactions:          newReactionStore(nil),
		Releases:           newReleaseStore(nil),
		Deployments:        newDeploymentStore(nil),
		PRReviewComments:   newPRReviewCommentStore(nil),
		Misc:               newMiscStore(),
		ProjectsV2:         newProjectV2Store(nil),
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
// invariant: open-failure must be caught at the persistence-open
// site (MustNewPersistence) so the operator gets a fail-loud signal
// before we even get here.
func (st *Store) SetPersistence(p *Persistence) error {
	if p == nil {
		return nil
	}
	st.mu.Lock()
	st.persist = p
	st.Reactions.persist = p
	st.Releases.persist = p
	st.Deployments.persist = p
	st.PRReviewComments.persist = p
	st.ProjectsV2.persist = p
	st.Misc.persist = p
	st.mu.Unlock()
	return st.loadFromPersistence()
}

// loadFromPersistence repopulates the in-memory maps from disk.
//
// Loads buckets:
//
//	users, tokens, apps, oauth_apps, installations, installation_tokens,
//	user_to_server_tokens, refresh_tokens, repos, orgs, teams, memberships,
//	labels, milestones, issues, comments, pull_requests, pr_reviews,
//	hooks, org_hooks, hook_deliveries, app_hook_deliveries, repo_secrets,
//	check_suites, check_runs, check_suite_prefs, workflow_files,
//	releases, deployments, deployment_statuses, environments,
//	pr_review_comments, reactions, projects_v2, project_v2_items,
//	project_v2_fields, misc, user_keys, gpg_keys, pages_sites,
//	pages_builds, branch_protection, audit_log, marketplace_plans.
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
		// Owner (a *User) is relinked from the persisted OwnerID; users load
		// before repos. For org-owned repos the FullName segment is the org
		// login, so OwnerID (the creator's user ID) is the reliable key.
		// Legacy rows without owner_id fall back to the FullName user segment.
		// A repo without a resolvable owner is unusable (RBAC denies the
		// owner, refs handlers dereference Owner), so fail loud.
		var owner *User
		if r.OwnerID != 0 {
			owner = st.Users[r.OwnerID]
		} else {
			ownerLogin, _, _ := strings.Cut(r.FullName, "/")
			owner = st.UsersByLogin[ownerLogin]
		}
		if owner == nil {
			return fmt.Errorf("repo %s: owner (id=%d) not found in loaded users", r.FullName, r.OwnerID)
		}
		r.Owner = owner
		r.OwnerID = owner.ID
		// Per-repo number counters are recomputed from loaded issues/PRs/
		// milestones below (their loaders bump these past every seen number).
		r.NextIssueNumber = 1
		r.NextMilestoneNumber = 1
		st.Repos[r.ID] = &r
		st.ReposByName[r.FullName] = &r
		if r.ID >= st.NextRepo {
			st.NextRepo = r.ID + 1
		}
		// Re-open (or create) the git storage for this repo so git operations
		// work immediately after restart.
		stor, err := openOrInitGitStorage(context.Background(), r.FullName)
		if err != nil {
			return fmt.Errorf("reopen git storage %s: %w", r.FullName, err)
		}
		st.GitStorages[r.FullName] = stor
		return nil
	}); err != nil {
		return err
	}
	// Load orgs, teams, memberships.
	if err := st.loadBucket("orgs", func(raw []byte) error {
		var o Org
		if err := loadJSON(raw, &o); err != nil {
			return err
		}
		st.Orgs[o.ID] = &o
		st.OrgsByLogin[o.Login] = &o
		if o.ID >= st.NextOrg {
			st.NextOrg = o.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("teams", func(raw []byte) error {
		var t Team
		if err := loadJSON(raw, &t); err != nil {
			return err
		}
		st.Teams[t.ID] = &t
		// Rebuild TeamsBySlug by looking up the org.
		if org := st.Orgs[t.OrgID]; org != nil {
			st.TeamsBySlug[teamSlugKey(org.Login, t.Slug)] = &t
		}
		if t.ID >= st.NextTeam {
			st.NextTeam = t.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("memberships", func(raw []byte) error {
		var m Membership
		if err := loadJSON(raw, &m); err != nil {
			return err
		}
		if org := st.Orgs[m.OrgID]; org != nil {
			st.Memberships[membershipKey(org.Login, m.UserID)] = &m
		}
		return nil
	}); err != nil {
		return err
	}
	// Load issues, labels, milestones, comments.
	if err := st.loadBucket("labels", func(raw []byte) error {
		var l IssueLabel
		if err := loadJSON(raw, &l); err != nil {
			return err
		}
		st.Labels[l.ID] = &l
		if l.ID >= st.NextLabel {
			st.NextLabel = l.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("milestones", func(raw []byte) error {
		var m Milestone
		if err := loadJSON(raw, &m); err != nil {
			return err
		}
		st.Milestones[m.ID] = &m
		if m.ID >= st.NextMilestone {
			st.NextMilestone = m.ID + 1
		}
		if repo := st.Repos[m.RepoID]; repo != nil && m.Number >= repo.NextMilestoneNumber {
			repo.NextMilestoneNumber = m.Number + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("issues", func(raw []byte) error {
		var i Issue
		if err := loadJSON(raw, &i); err != nil {
			return err
		}
		st.Issues[i.ID] = &i
		if i.ID >= st.NextIssue {
			st.NextIssue = i.ID + 1
		}
		if repo := st.Repos[i.RepoID]; repo != nil && i.Number >= repo.NextIssueNumber {
			repo.NextIssueNumber = i.Number + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("comments", func(raw []byte) error {
		var c Comment
		if err := loadJSON(raw, &c); err != nil {
			return err
		}
		st.Comments[c.ID] = &c
		if c.ID >= st.NextComment {
			st.NextComment = c.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("pull_requests", func(raw []byte) error {
		var pr PullRequest
		if err := loadJSON(raw, &pr); err != nil {
			return err
		}
		st.PullRequests[pr.ID] = &pr
		if pr.ID >= st.NextPR {
			st.NextPR = pr.ID + 1
		}
		// PRs share the per-repo issue-number sequence with issues.
		if repo := st.Repos[pr.RepoID]; repo != nil && pr.Number >= repo.NextIssueNumber {
			repo.NextIssueNumber = pr.Number + 1
		}
		return nil
	}); err != nil {
		return err
	}

	for _, loadFn := range []struct {
		name string
		fn   func(string, []byte) error
	}{
		{"hooks", func(key string, raw []byte) error {
			var hooks []*Webhook
			if err := loadJSON(raw, &hooks); err != nil {
				return err
			}
			// RepoKey is json:"-" (it duplicates the bucket key), so
			// backfill it — deliveries and hook lookups key on it.
			for _, h := range hooks {
				h.RepoKey = key
				if h.ID >= st.NextHookID {
					st.NextHookID = h.ID + 1
				}
			}
			st.Hooks[key] = hooks
			return nil
		}},
		{"org_hooks", func(key string, raw []byte) error {
			var hooks []*Webhook
			if err := loadJSON(raw, &hooks); err != nil {
				return err
			}
			// OrgLogin is json:"-" (it duplicates the bucket key), so
			// backfill it — deliveries and hook lookups key on it.
			for _, h := range hooks {
				h.OrgLogin = key
				if h.ID >= st.NextHookID {
					st.NextHookID = h.ID + 1
				}
			}
			st.OrgHooks[key] = hooks
			return nil
		}},
		{"hook_deliveries", func(_ string, raw []byte) error {
			var deliveries []*WebhookDelivery
			if err := loadJSON(raw, &deliveries); err != nil {
				return err
			}
			for _, d := range deliveries {
				st.HookDeliveries[d.HookID] = append(st.HookDeliveries[d.HookID], d)
				if d.ID >= st.NextDeliveryID {
					st.NextDeliveryID = d.ID + 1
				}
			}
			return nil
		}},
		{"app_hook_deliveries", func(key string, raw []byte) error {
			// App deliveries are bucketed by app ID; a delivery's HookID is
			// NOT the app ID (app-level deliveries use a synthetic hook id),
			// so file the slice under the bucket key.
			appID, err := strconv.Atoi(key)
			if err != nil {
				return fmt.Errorf("app_hook_deliveries key %q: %w", key, err)
			}
			var deliveries []*WebhookDelivery
			if err := loadJSON(raw, &deliveries); err != nil {
				return err
			}
			for _, d := range deliveries {
				st.AppHookDeliveries[appID] = append(st.AppHookDeliveries[appID], d)
				if d.ID >= st.NextDeliveryID {
					st.NextDeliveryID = d.ID + 1
				}
			}
			return nil
		}},
		{"check_suite_prefs", func(key string, raw []byte) error {
			var prefs []*CheckSuitePref
			if err := loadJSON(raw, &prefs); err != nil {
				return err
			}
			st.CheckSuitePrefs[key] = prefs
			return nil
		}},
		{"repo_secrets", func(key string, raw []byte) error {
			var secrets map[string]*Secret
			if err := loadJSON(raw, &secrets); err != nil {
				return err
			}
			st.RepoSecrets[key] = secrets
			return nil
		}},
		{"repo_variables", func(key string, raw []byte) error {
			var vars map[string]*ActionsVariable
			if err := loadJSON(raw, &vars); err != nil {
				return err
			}
			st.RepoVariables[key] = vars
			return nil
		}},
		{"org_secrets", func(key string, raw []byte) error {
			var secrets map[string]*OrgSecret
			if err := loadJSON(raw, &secrets); err != nil {
				return err
			}
			st.OrgSecrets[key] = secrets
			return nil
		}},
		{"org_variables", func(key string, raw []byte) error {
			var vars map[string]*ActionsVariable
			if err := loadJSON(raw, &vars); err != nil {
				return err
			}
			st.OrgVariables[key] = vars
			return nil
		}},
		{"env_secrets", func(key string, raw []byte) error {
			var secrets map[string]*Secret
			if err := loadJSON(raw, &secrets); err != nil {
				return err
			}
			st.EnvSecrets[key] = secrets
			return nil
		}},
		{"env_variables", func(key string, raw []byte) error {
			var vars map[string]*ActionsVariable
			if err := loadJSON(raw, &vars); err != nil {
				return err
			}
			st.EnvVariables[key] = vars
			return nil
		}},
		{"runner_groups", func(_ string, raw []byte) error {
			var g RunnerGroup
			if err := loadJSON(raw, &g); err != nil {
				return err
			}
			st.RunnerGroups[g.ID] = &g
			if g.ID >= st.NextRunnerGroupID {
				st.NextRunnerGroupID = g.ID + 1
			}
			return nil
		}},
		{"actions_crypto", func(key string, raw []byte) error {
			if key != "keypair" {
				return nil
			}
			var kp SecretsKeyPair
			if err := loadJSON(raw, &kp); err != nil {
				return err
			}
			st.actionsKeyPair = &kp
			return nil
		}},
		{"check_suites", func(_ string, raw []byte) error {
			var s CheckSuite
			if err := loadJSON(raw, &s); err != nil {
				return err
			}
			st.CheckSuites[s.ID] = &s
			if s.ID >= st.NextCheckSuiteID {
				st.NextCheckSuiteID = s.ID + 1
			}
			return nil
		}},
		{"check_runs", func(_ string, raw []byte) error {
			var cr CheckRun
			if err := loadJSON(raw, &cr); err != nil {
				return err
			}
			st.CheckRuns[cr.ID] = &cr
			if cr.ID >= st.NextCheckRunID {
				st.NextCheckRunID = cr.ID + 1
			}
			return nil
		}},
		{"workflow_files", func(_ string, raw []byte) error {
			var wf WorkflowFile
			if err := loadJSON(raw, &wf); err != nil {
				return err
			}
			if st.WorkflowFiles == nil {
				st.WorkflowFiles = map[int64]*WorkflowFile{}
			}
			st.WorkflowFiles[wf.ID] = &wf
			return nil
		}},
		{"pr_reviews", func(_ string, raw []byte) error {
			var r PullRequestReview
			if err := loadJSON(raw, &r); err != nil {
				return err
			}
			st.PRReviews[r.ID] = &r
			if r.ID >= st.NextPRReview {
				st.NextPRReview = r.ID + 1
			}
			return nil
		}},
		{"releases", func(_ string, raw []byte) error {
			var r Release
			if err := loadJSON(raw, &r); err != nil {
				return err
			}
			st.Releases.byID[r.ID] = &r
			st.Releases.byRepo[r.RepoID] = append(st.Releases.byRepo[r.RepoID], &r)
			if r.ID >= st.Releases.nextID {
				st.Releases.nextID = r.ID + 1
			}
			return nil
		}},
		{"deployments", func(_ string, raw []byte) error {
			var d Deployment
			if err := loadJSON(raw, &d); err != nil {
				return err
			}
			st.Deployments.deployments[d.ID] = &d
			st.Deployments.byRepo[d.RepoID] = append(st.Deployments.byRepo[d.RepoID], &d)
			if d.ID >= st.Deployments.nextDepID {
				st.Deployments.nextDepID = d.ID + 1
			}
			return nil
		}},
		{"deployment_statuses", func(_ string, raw []byte) error {
			var s DeploymentStatus
			if err := loadJSON(raw, &s); err != nil {
				return err
			}
			st.Deployments.statuses[s.ID] = &s
			// Relink onto the owning deployment (Deployment.Statuses is
			// json:"-"; deployments load before statuses). Insertion order
			// is map-random here — a post-pass below sorts by ID.
			if d := st.Deployments.deployments[s.DeploymentID]; d != nil {
				d.Statuses = append(d.Statuses, &s)
			}
			if s.ID >= st.Deployments.nextStatusID {
				st.Deployments.nextStatusID = s.ID + 1
			}
			return nil
		}},
		{"environments", func(key string, raw []byte) error {
			var e Environment
			if err := loadJSON(raw, &e); err != nil {
				return err
			}
			// The bucket key IS the "repoID:name" map key — use it directly.
			st.Deployments.environments[key] = &e
			st.Deployments.envsByRepo[e.RepoID] = append(st.Deployments.envsByRepo[e.RepoID], &e)
			if e.ID >= st.Deployments.nextEnvID {
				st.Deployments.nextEnvID = e.ID + 1
			}
			return nil
		}},
		{"pr_review_comments", func(_ string, raw []byte) error {
			rec := prReviewCommentRecord{PRReviewComment: &PRReviewComment{}}
			if err := loadJSON(raw, &rec); err != nil {
				return err
			}
			c := rec.restore()
			if c.ThreadID == 0 && c.InReplyToID == 0 {
				// Row predates the thread-id record field: a root comment is
				// its own thread.
				c.ThreadID = c.ID
			}
			st.PRReviewComments.byID[c.ID] = c
			st.PRReviewComments.byPR[c.PullRequestID] = append(st.PRReviewComments.byPR[c.PullRequestID], c)
			if c.ThreadID != 0 {
				st.PRReviewComments.threadRoots[c.ID] = c.ThreadID
			}
			if c.ID >= st.PRReviewComments.nextID {
				st.PRReviewComments.nextID = c.ID + 1
			}
			return nil
		}},
		{"reactions", func(_ string, raw []byte) error {
			var reactions []*Reaction
			if err := loadJSON(raw, &reactions); err != nil {
				return err
			}
			for _, r := range reactions {
				st.Reactions.byID[r.ID] = r
				st.Reactions.byParent[reactionParentKey(r.ParentType, r.ParentID)] = append(st.Reactions.byParent[reactionParentKey(r.ParentType, r.ParentID)], r)
				if r.ID >= st.Reactions.nextID {
					st.Reactions.nextID = r.ID + 1
				}
			}
			return nil
		}},
		{"projects_v2", func(_ string, raw []byte) error {
			var p ProjectV2
			if err := loadJSON(raw, &p); err != nil {
				return err
			}
			st.ProjectsV2.projects[p.ID] = &p
			if p.ID >= st.ProjectsV2.nextProjectID {
				st.ProjectsV2.nextProjectID = p.ID + 1
			}
			return nil
		}},
		{"project_v2_items", func(_ string, raw []byte) error {
			var it ProjectV2Item
			if err := loadJSON(raw, &it); err != nil {
				return err
			}
			st.ProjectsV2.items[it.ID] = &it
			st.ProjectsV2.itemsByOwner[it.ContentID] = append(st.ProjectsV2.itemsByOwner[it.ContentID], &it)
			if it.ID >= st.ProjectsV2.nextItemID {
				st.ProjectsV2.nextItemID = it.ID + 1
			}
			return nil
		}},
		{"project_v2_fields", func(_ string, raw []byte) error {
			var f ProjectV2Field
			if err := loadJSON(raw, &f); err != nil {
				return err
			}
			st.ProjectsV2.fields[f.ID] = &f
			st.ProjectsV2.fieldsByProj[f.ProjectID] = append(st.ProjectsV2.fieldsByProj[f.ProjectID], &f)
			if f.ID >= st.ProjectsV2.nextFieldID {
				st.ProjectsV2.nextFieldID = f.ID + 1
			}
			// Option IDs are hex renderings of nextOptionSeed; resume the
			// seed past every loaded option so new options can't collide.
			for _, opt := range f.Options {
				if n, err := strconv.ParseInt(opt.ID, 16, 64); err == nil && int(n) >= st.ProjectsV2.nextOptionSeed {
					st.ProjectsV2.nextOptionSeed = int(n) + 1
				}
			}
			return nil
		}},
	} {
		rows, err := st.persist.List(loadFn.name)
		if err != nil {
			return fmt.Errorf("load %s: %w", loadFn.name, err)
		}
		for k, raw := range rows {
			if err := loadFn.fn(k, raw); err != nil {
				return fmt.Errorf("decode %s row: %w", loadFn.name, err)
			}
		}
	}

	// Deployment statuses were relinked in map-iteration order; restore
	// creation (ID) order, which is what AddStatus produces.
	for _, d := range st.Deployments.deployments {
		sort.Slice(d.Statuses, func(i, j int) bool { return d.Statuses[i].ID < d.Statuses[j].ID })
	}

	for _, loadFn := range []struct {
		name string
		fn   func(string, []byte) error
	}{
		{"misc", func(key string, raw []byte) error {
			switch key {
			case "oidc_claim_keys":
				var keys []string
				if err := loadJSON(raw, &keys); err != nil {
					return err
				}
				st.Misc.oidcClaimKeys = keys
			case "follows":
				var follows map[string]map[string]bool
				if err := loadJSON(raw, &follows); err != nil {
					return err
				}
				st.Misc.follows = follows
			}
			return nil
		}},
		{"user_keys", func(_ string, raw []byte) error {
			var k UserKey
			if err := loadJSON(raw, &k); err != nil {
				return err
			}
			st.Misc.userKeys[k.ID] = &k
			st.Misc.keysByUser[k.UserID] = append(st.Misc.keysByUser[k.UserID], &k)
			if k.ID >= st.Misc.nextKeyID {
				st.Misc.nextKeyID = k.ID + 1
			}
			return nil
		}},
		{"pages_sites", func(key string, raw []byte) error {
			repoID, err := strconv.Atoi(key)
			if err != nil {
				return fmt.Errorf("pages_sites key %q: %w", key, err)
			}
			var site PagesSite
			if err := loadJSON(raw, &site); err != nil {
				return err
			}
			st.Misc.pagesByRepo[repoID] = &site
			return nil
		}},
		{"branch_protection", func(key string, raw []byte) error {
			var bp BranchProtection
			if err := loadJSON(raw, &bp); err != nil {
				return err
			}
			st.Misc.branchProtection[key] = bp
			return nil
		}},
		{"gpg_keys", func(_ string, raw []byte) error {
			var k GPGKey
			if err := loadJSON(raw, &k); err != nil {
				return err
			}
			st.Misc.gpgKeys[k.ID] = &k
			st.Misc.gpgKeysByUser[k.UserID] = append(st.Misc.gpgKeysByUser[k.UserID], &k)
			if k.ID >= st.Misc.nextGPGKeyID {
				st.Misc.nextGPGKeyID = k.ID + 1
			}
			return nil
		}},
		{"pages_builds", func(key string, raw []byte) error {
			var builds []*PagesBuild
			if err := loadJSON(raw, &builds); err != nil {
				return err
			}
			st.Misc.pagesBuilds[key] = builds
			// nextAuditID is pre-incremented before use, so resume it AT the
			// highest seen ID (the next allocation bumps past it).
			for _, b := range builds {
				if b.ID > st.Misc.nextAuditID {
					st.Misc.nextAuditID = b.ID
				}
			}
			return nil
		}},
		{"audit_log", func(_ string, raw []byte) error {
			var e AuditEntry
			if err := loadJSON(raw, &e); err != nil {
				return err
			}
			st.Misc.auditLog = append(st.Misc.auditLog, &e)
			// nextAuditID is pre-incremented before use; resume AT the max.
			if e.ID > st.Misc.nextAuditID {
				st.Misc.nextAuditID = e.ID
			}
			return nil
		}},
		{"marketplace_plans", func(_ string, raw []byte) error {
			var p MarketplacePlan
			if err := loadJSON(raw, &p); err != nil {
				return err
			}
			st.Misc.marketplacePlans[p.ID] = &p
			return nil
		}},
	} {
		rows, err := st.persist.List(loadFn.name)
		if err != nil {
			return fmt.Errorf("load %s: %w", loadFn.name, err)
		}
		for k, raw := range rows {
			if err := loadFn.fn(k, raw); err != nil {
				return fmt.Errorf("decode %s row: %w", loadFn.name, err)
			}
		}
	}

	// Audit entries arrive in map-iteration order; the in-memory log is
	// newest-first (recordAuditEvent prepends), so sort by ID descending.
	sort.Slice(st.Misc.auditLog, func(i, j int) bool { return st.Misc.auditLog[i].ID > st.Misc.auditLog[j].ID })

	if v, err := st.persist.GetCounter("next_run_id"); err != nil {
		return fmt.Errorf("load counter next_run_id: %w", err)
	} else if int(v) > st.NextRunID {
		st.NextRunID = int(v)
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

	now := time.Now().UTC()
	u := &User{
		ID:           st.NextUser,
		NodeID:       "U_kgDOBdefault",
		Login:        "admin",
		Name:         "Admin",
		Email:        "admin@bleephub.local",
		AvatarURL:    "",
		Bio:          "",
		Type:         "User",
		SiteAdmin:    true,
		StarredRepos: map[string]bool{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	st.Users[u.ID] = u
	st.UsersByLogin[u.Login] = u
	st.NextUser++
	if st.persist != nil {
		st.persist.MustPut("users", strconv.Itoa(u.ID), u)
	}

	t := &Token{
		Value:     AdminToken(),
		UserID:    u.ID,
		Scopes:    "repo, workflow, read:org, admin:org, gist",
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

// GetUserByID returns the user with the given ID, or nil.
func (st *Store) GetUserByID(id int) *User {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Users[id]
}

// CountFollowers returns how many users follow the given login.
func (st *Store) CountFollowers(login string) int {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	n := 0
	for _, follows := range st.Misc.follows {
		if follows[login] {
			n++
		}
	}
	return n
}

// CountFollowing returns how many users the given login follows.
func (st *Store) CountFollowing(login string) int {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	return len(st.Misc.follows[login])
}

// CountPublicRepos returns the number of non-private repositories owned
// by the given account login (user or organization).
func (st *Store) CountPublicRepos(login string) int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	prefix := login + "/"
	n := 0
	for name, r := range st.ReposByName {
		if strings.HasPrefix(name, prefix) && !r.Private {
			n++
		}
	}
	return n
}

// CountOpenIssues returns the number of open issues plus open pull
// requests in a repository — GitHub's open_issues_count counts both
// because PRs are issues internally.
func (st *Store) CountOpenIssues(repoID int) int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	n := 0
	for _, issue := range st.Issues {
		if issue.RepoID == repoID && issue.State == "OPEN" {
			n++
		}
	}
	for _, pr := range st.PullRequests {
		if pr.RepoID == repoID && pr.State == "OPEN" {
			n++
		}
	}
	return n
}

// CreateToken generates a new token for the given user.
func (st *Store) CreateToken(userID int, scopes string) *Token {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.createTokenLocked(userID, scopes)
}

// generateTokenValue creates a ghp_-prefixed random token string (classic PAT).
// Real GitHub uses ghp_ for classic PATs; bleephub matches the prefix so SDK
// clients that branch on prefix recognise the token shape.
func generateTokenValue() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return fmt.Sprintf("ghp_%s", hex.EncodeToString(b))
}
