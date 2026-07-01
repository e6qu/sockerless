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

// RepoAutolink represents a GitHub autolink reference configured on a repository.
type RepoAutolink struct {
	ID             int       `json:"id"`
	NodeID         string    `json:"node_id"`
	RepoKey        string    `json:"-"`
	KeyPrefix      string    `json:"key_prefix"`
	URLTemplate    string    `json:"url_template"`
	IsAlphanumeric bool      `json:"is_alphanumeric"`
	CreatedAt      time.Time `json:"created_at"`
}

// RepoInvitation represents a pending invitation to collaborate on a repository.
type RepoInvitation struct {
	ID           int       `json:"id"`
	NodeID       string    `json:"node_id"`
	RepoKey      string    `json:"-"`
	InviteeLogin string    `json:"invitee_login,omitempty"`
	InviteeEmail string    `json:"invitee_email,omitempty"`
	InviterID    int       `json:"inviter_id"`
	Permissions  string    `json:"permissions"`
	CreatedAt    time.Time `json:"created_at"`
	Status       string    `json:"status"`
}

// LoginSession is a browser session created by POST /login.
// It binds a session cookie to a user and carries the CSRF token
// embedded in the OAuth authorize consent form.
type LoginSession struct {
	UserID    int
	CSRFToken string
	ExpiresAt time.Time
}

// GistFile is a single file inside a gist.
type GistFile struct {
	Filename string `json:"filename"`
	Type     string `json:"type"`
	Language string `json:"language"`
	RawURL   string `json:"raw_url"`
	Size     int    `json:"size"`
	Content  string `json:"content,omitempty"`
}

// GistHistory captures one revision of a gist.
type GistHistory struct {
	Version      string         `json:"version"`
	CommittedAt  time.Time      `json:"committed_at"`
	ChangeStatus map[string]int `json:"change_status"`
	URL          string         `json:"url"`
}

// Gist is a GitHub gist.
type Gist struct {
	ID          string               `json:"id"`
	NodeID      string               `json:"node_id"`
	Description string               `json:"description"`
	Public      bool                 `json:"public"`
	OwnerID     int                  `json:"-"`
	Files       map[string]*GistFile `json:"files"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	Comments    int                  `json:"comments"`
	CommentsURL string               `json:"comments_url"`
	HTMLURL     string               `json:"html_url"`
	URL         string               `json:"url"`
	ForksURL    string               `json:"forks_url"`
	CommitsURL  string               `json:"commits_url"`
	GitPullURL  string               `json:"git_pull_url"`
	GitPushURL  string               `json:"git_push_url"`
	History     []*GistHistory       `json:"history"`
	ForkOfID    string               `json:"-"`
	ForkIDs     []string             `json:"-"`
}

// GistComment is a comment on a gist.
type GistComment struct {
	ID                int       `json:"id"`
	NodeID            string    `json:"node_id"`
	GistID            string    `json:"-"`
	UserID            int       `json:"-"`
	Body              string    `json:"body"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	AuthorAssociation string    `json:"author_association"`
	URL               string    `json:"url"`
}

// Store holds all in-memory state for bleephub.
type Store struct {
	Agents                     map[int]*Agent
	Sessions                   map[string]*Session
	Jobs                       map[string]*Job
	Users                      map[int]*User
	UsersByLogin               map[string]*User
	Tokens                     map[string]*Token
	DeviceCodes                map[string]*DeviceCode
	AuthCodes                  map[string]*authCode     // OAuth web-flow codes
	LoginSessions              map[string]*LoginSession // _gh_sess cookie value → session
	Repos                      map[int]*Repo
	ReposByName                map[string]*Repo                       // "owner/name" → repo
	GitStorages                map[string]gitStorage.Storer           // "owner/name" → go-git storage (memory or filesystem)
	Orgs                       map[int]*Org                           // id → org
	OrgsByLogin                map[string]*Org                        // login → org
	Teams                      map[int]*Team                          // id → team
	TeamsBySlug                map[string]*Team                       // "org/slug" → team
	Memberships                map[string]*Membership                 // "org/user" → membership
	Issues                     map[int]*Issue                         // id → issue
	Labels                     map[int]*IssueLabel                    // id → label
	Milestones                 map[int]*Milestone                     // id → milestone
	Comments                   map[int]*Comment                       // id → comment
	PullRequests               map[int]*PullRequest                   // id → PR
	PRReviews                  map[int]*PullRequestReview             // id → review
	Workflows                  map[string]*Workflow                   // id → workflow (run-level)
	WorkflowFiles              map[int64]*WorkflowFile                // id → workflow file (file-level)
	PendingMessages            []*TaskAgentMessage                    // messages awaiting delivery
	RepoSecrets                map[string]map[string]*Secret          // "owner/repo" → name → secret
	RepoVariables              map[string]map[string]*ActionsVariable // "owner/repo" → NAME → variable
	RepoCollaborators          map[string]map[string]string           // "owner/repo" → login → permission (pull/push/admin)
	RepoAutolinks              map[string]map[int]*RepoAutolink       // "owner/repo" → id → autolink
	RepoInvitations            map[string]map[int]*RepoInvitation     // "owner/repo" → id → invitation
	OrgSecrets                 map[string]map[string]*OrgSecret       // org login → NAME → org secret
	OrgVariables               map[string]map[string]*ActionsVariable // org login → NAME → org variable
	EnvSecrets                 map[string]map[string]*Secret          // envScopeKey(repo, env) → NAME → secret
	EnvVariables               map[string]map[string]*ActionsVariable // envScopeKey(repo, env) → NAME → variable
	TimelineRecords            map[string][]*TimelineRecord           // planID → runner-uploaded timeline records
	LogFiles                   map[int][]byte                         // logID → uploaded runner log content
	WorkflowAttempts           map[int][]*Workflow                    // runID → prior attempts (oldest first)
	RunnerGroups               map[int]*RunnerGroup                   // org runner groups (global pool overlay)
	NextRunnerGroupID          int
	Hooks                      map[string][]*Webhook         // "owner/repo" → hooks
	OrgHooks                   map[string][]*Webhook         // org login → org-level hooks
	HookDeliveries             map[int][]*WebhookDelivery    // hookID → deliveries
	Apps                       map[int]*App                  // id → app
	AppsBySlug                 map[string]*App               // slug → app
	AppsByClientID             map[string]*App               // OAuth client_id → app
	OAuthApps                  map[string]*OAuthApp          // OAuth client_id → OAuth app (distinct from GitHub App)
	Installations              map[int]*Installation         // id → installation
	InstallationTokens         map[string]*InstallationToken // token value → token
	UserToServerTokens         map[string]*UserToServerToken // gho_/ghu_ token value → token
	RefreshTokens              map[string]*RefreshToken      // ghr_ token value → refresh token
	AppHookDeliveries          map[int][]*WebhookDelivery    // appID → app-level webhook deliveries
	ManifestCodes              map[string]int                // code → appID (one-time-use)
	CheckRuns                  map[int64]*CheckRun           // id → check run
	CheckSuites                map[int64]*CheckSuite         // id → check suite
	CheckSuitePrefs            map[string][]*CheckSuitePref  // repoKey → autoTrigger prefs
	CommitStatuses             *CommitStatusStore            // commit status contexts per repo+ref
	CommitComments             *CommitCommentStore           // commit comments per repo/commit
	Reactions                  *ReactionStore                // reactions across all parent types
	Releases                   *ReleaseStore                 // release CRUD
	Deployments                *DeploymentStore              // deployments + statuses + environments
	PRReviewComments           *PRReviewCommentStore         // PR review comments (inline / threads)
	Misc                       *MiscStore                    // long-tail surfaces
	ProjectsV2                 *ProjectV2Store               // GitHub Projects v2
	NotificationsState         map[int]*UserNotificationsState
	Rulesets                   map[int]*Ruleset
	ProjectClassic             map[int]*ProjectClassic                // id → project
	ProjectColumns             map[int]*ProjectColumn                 // id → column
	ProjectCards               map[int]*ProjectCard                   // id → card
	UserMigrations             map[int]*UserMigration                 // id → user migration
	OrgMigrations              map[int]*OrgMigration                  // id → org migration
	Codespaces                 map[int]*Codespace                     // id → codespace
	CodespacesByName           map[string]*Codespace                  // name → codespace
	CodespaceSecrets           map[string]map[string]*CodespaceSecret // scope\x1fname → secret
	NextCodespaceID            int
	NextCodespaceSecretID      int
	LogLines                   map[string][]string     // jobID → captured console log lines
	Gists                      map[string]*Gist        // id → gist
	GistComments               map[int]*GistComment    // id → gist comment
	StarredGists               map[int]map[string]bool // userID → gistID → starred
	SecretScanningAlerts       map[int]*SecretScanningAlert
	SecretScanningAlertsByRepo map[string]map[int]*SecretScanningAlert // repoKey → alertNumber → alert
	SecretScanningNextNumber   map[string]int                          // repoKey → next alert number
	CodeScanningAlerts         map[int]*CodeScanningAlert
	CodeScanningAlertsByRepo   map[string]map[int]*CodeScanningAlert // repoKey → alertNumber → alert
	CodeScanningNextNumber     map[string]int                        // repoKey → next alert number
	CodeScanningAnalyses       map[int]*CodeScanningAnalysis
	CodeScanningAnalysesByRepo map[string]map[int]*CodeScanningAnalysis // repoKey → analysisID → analysis
	SARIFUploads               map[string]*SARIFUpload                  // uploadID → upload
	DependabotAlerts           map[int]*DependabotAlert
	DependabotAlertsByRepo     map[string]map[int]*DependabotAlert        // repoKey → alertNumber → alert
	DependabotNextNumber       map[string]int                             // repoKey → next alert number
	DependabotSecrets          map[string]map[string]*DependabotSecret    // repoKey → name → secret
	DependabotOrgSecrets       map[string]map[string]*DependabotOrgSecret // orgLogin → name → secret
	Packages                   map[int]*Package
	PackageVersions            map[int]*PackageVersion
	PackageFiles               map[int]*PackageFile
	PackagesByOwnerKey         map[string]map[string]*Package  // ownerKey → packageKey → package
	PackageVersionsByPackage   map[int]map[int]*PackageVersion // packageID → versionID → version
	PackageFilesByVersion      map[int]map[int]*PackageFile    // versionID → fileID → file
	PackageDataDir             string                          // directory for package file bytes
	NextGistID                 int
	NextGistCommentID          int
	NextAgent                  int
	NextSecretScanningAlertID  int
	NextCodeScanningAlertID    int
	NextCodeScanningAnalysisID int
	NextDependabotAlertID      int
	NextPackageID              int
	NextPackageVersionID       int
	NextPackageFileID          int
	NextMsg                    int64
	NextLog                    int
	NextReqID                  int64
	NextUser                   int
	NextRepo                   int
	NextOrg                    int
	NextTeam                   int
	NextIssue                  int
	NextLabel                  int
	NextMilestone              int
	NextComment                int
	NextPR                     int
	NextPRReview               int
	NextRunID                  int
	NextHookID                 int
	NextDeliveryID             int
	NextAppID                  int
	NextInstallationID         int
	NextCheckRunID             int64
	NextCheckSuiteID           int64
	NextRulesetID              int
	NextProjectClassicID       int
	NextProjectColumnID        int
	NextProjectCardID          int
	NextUserMigrationID        int
	NextOrgMigrationID         int
	NextAutolinkID             int
	NextInvitationID           int
	Discussions                map[int]*Discussion
	DiscussionCategories       map[int]*DiscussionCategory
	DiscussionComments         map[int]*DiscussionComment
	NextDiscussionID           int
	NextDiscussionCategoryID   int
	NextDiscussionCommentID    int
	actionsKeyPair             *SecretsKeyPair // lazily generated sealed-box keypair (persisted)
	persist                    *Persistence
	mu                         sync.RWMutex
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
		Agents:                     make(map[int]*Agent),
		Sessions:                   make(map[string]*Session),
		Jobs:                       make(map[string]*Job),
		Users:                      make(map[int]*User),
		UsersByLogin:               make(map[string]*User),
		Tokens:                     make(map[string]*Token),
		DeviceCodes:                make(map[string]*DeviceCode),
		AuthCodes:                  make(map[string]*authCode),
		LoginSessions:              make(map[string]*LoginSession),
		Repos:                      make(map[int]*Repo),
		ReposByName:                make(map[string]*Repo),
		GitStorages:                make(map[string]gitStorage.Storer),
		Orgs:                       make(map[int]*Org),
		OrgsByLogin:                make(map[string]*Org),
		Teams:                      make(map[int]*Team),
		TeamsBySlug:                make(map[string]*Team),
		Memberships:                make(map[string]*Membership),
		Issues:                     make(map[int]*Issue),
		Labels:                     make(map[int]*IssueLabel),
		Milestones:                 make(map[int]*Milestone),
		Comments:                   make(map[int]*Comment),
		PullRequests:               make(map[int]*PullRequest),
		PRReviews:                  make(map[int]*PullRequestReview),
		Workflows:                  make(map[string]*Workflow),
		WorkflowFiles:              make(map[int64]*WorkflowFile),
		RepoSecrets:                make(map[string]map[string]*Secret),
		RepoVariables:              make(map[string]map[string]*ActionsVariable),
		RepoCollaborators:          make(map[string]map[string]string),
		RepoAutolinks:              make(map[string]map[int]*RepoAutolink),
		RepoInvitations:            make(map[string]map[int]*RepoInvitation),
		OrgSecrets:                 make(map[string]map[string]*OrgSecret),
		OrgVariables:               make(map[string]map[string]*ActionsVariable),
		EnvSecrets:                 make(map[string]map[string]*Secret),
		EnvVariables:               make(map[string]map[string]*ActionsVariable),
		TimelineRecords:            make(map[string][]*TimelineRecord),
		LogFiles:                   make(map[int][]byte),
		WorkflowAttempts:           make(map[int][]*Workflow),
		RunnerGroups:               make(map[int]*RunnerGroup),
		NextRunnerGroupID:          2,
		Hooks:                      make(map[string][]*Webhook),
		OrgHooks:                   make(map[string][]*Webhook),
		HookDeliveries:             make(map[int][]*WebhookDelivery),
		Apps:                       make(map[int]*App),
		AppsBySlug:                 make(map[string]*App),
		AppsByClientID:             make(map[string]*App),
		OAuthApps:                  make(map[string]*OAuthApp),
		Installations:              make(map[int]*Installation),
		InstallationTokens:         make(map[string]*InstallationToken),
		UserToServerTokens:         make(map[string]*UserToServerToken),
		RefreshTokens:              make(map[string]*RefreshToken),
		AppHookDeliveries:          make(map[int][]*WebhookDelivery),
		ManifestCodes:              make(map[string]int),
		CheckRuns:                  make(map[int64]*CheckRun),
		CheckSuites:                make(map[int64]*CheckSuite),
		CheckSuitePrefs:            make(map[string][]*CheckSuitePref),
		CommitStatuses:             newCommitStatusStore(nil),
		CommitComments:             newCommitCommentStore(nil),
		Reactions:                  newReactionStore(nil),
		Releases:                   newReleaseStore(nil),
		Deployments:                newDeploymentStore(nil),
		PRReviewComments:           newPRReviewCommentStore(nil),
		Misc:                       newMiscStore(),
		ProjectsV2:                 newProjectV2Store(nil),
		NotificationsState:         map[int]*UserNotificationsState{},
		Rulesets:                   map[int]*Ruleset{},
		ProjectClassic:             map[int]*ProjectClassic{},
		ProjectColumns:             map[int]*ProjectColumn{},
		ProjectCards:               map[int]*ProjectCard{},
		UserMigrations:             map[int]*UserMigration{},
		OrgMigrations:              map[int]*OrgMigration{},
		Codespaces:                 map[int]*Codespace{},
		CodespacesByName:           map[string]*Codespace{},
		CodespaceSecrets:           map[string]map[string]*CodespaceSecret{},
		LogLines:                   make(map[string][]string),
		Gists:                      make(map[string]*Gist),
		GistComments:               make(map[int]*GistComment),
		StarredGists:               make(map[int]map[string]bool),
		SecretScanningAlerts:       make(map[int]*SecretScanningAlert),
		SecretScanningAlertsByRepo: make(map[string]map[int]*SecretScanningAlert),
		SecretScanningNextNumber:   make(map[string]int),
		CodeScanningAlerts:         make(map[int]*CodeScanningAlert),
		CodeScanningAlertsByRepo:   make(map[string]map[int]*CodeScanningAlert),
		CodeScanningNextNumber:     make(map[string]int),
		CodeScanningAnalyses:       make(map[int]*CodeScanningAnalysis),
		CodeScanningAnalysesByRepo: make(map[string]map[int]*CodeScanningAnalysis),
		SARIFUploads:               make(map[string]*SARIFUpload),
		DependabotAlerts:           make(map[int]*DependabotAlert),
		DependabotAlertsByRepo:     make(map[string]map[int]*DependabotAlert),
		DependabotNextNumber:       make(map[string]int),
		DependabotSecrets:          make(map[string]map[string]*DependabotSecret),
		DependabotOrgSecrets:       make(map[string]map[string]*DependabotOrgSecret),
		Packages:                   map[int]*Package{},
		PackageVersions:            map[int]*PackageVersion{},
		PackageFiles:               map[int]*PackageFile{},
		PackagesByOwnerKey:         map[string]map[string]*Package{},
		PackageVersionsByPackage:   map[int]map[int]*PackageVersion{},
		PackageFilesByVersion:      map[int]map[int]*PackageFile{},
		Discussions:                map[int]*Discussion{},
		DiscussionCategories:       map[int]*DiscussionCategory{},
		DiscussionComments:         map[int]*DiscussionComment{},
		NextAgent:                  1,
		NextSecretScanningAlertID:  1,
		NextCodeScanningAlertID:    1,
		NextCodeScanningAnalysisID: 1,
		NextDependabotAlertID:      1,
		NextPackageID:              1,
		NextPackageVersionID:       1,
		NextPackageFileID:          1,
		NextMsg:                    1,
		NextLog:                    1,
		NextReqID:                  1,
		NextUser:                   1,
		NextRepo:                   1,
		NextOrg:                    1,
		NextTeam:                   1,
		NextIssue:                  1,
		NextLabel:                  1,
		NextMilestone:              1,
		NextComment:                1,
		NextPR:                     1,
		NextPRReview:               1,
		NextRunID:                  1,
		NextHookID:                 1,
		NextDeliveryID:             1,
		NextAppID:                  1,
		NextInstallationID:         1,
		NextCheckRunID:             1,
		NextCheckSuiteID:           1,
		NextRulesetID:              1,
		NextProjectClassicID:       1,
		NextProjectColumnID:        1,
		NextProjectCardID:          1,
		NextUserMigrationID:        1,
		NextOrgMigrationID:         1,
		NextCodespaceID:            1,
		NextCodespaceSecretID:      1,
		NextAutolinkID:             1,
		NextInvitationID:           1,
		NextDiscussionID:           1,
		NextDiscussionCategoryID:   1,
		NextDiscussionCommentID:    1,
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
	st.CommitStatuses.persist = p
	st.CommitComments.persist = p
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
//	pages_builds, branch_protection, audit_log, marketplace_plans,
//	notifications_state, repo_rulesets, projects_classic, project_columns,
//	project_cards, secret_scanning_alerts, code_scanning_alerts,
//	code_scanning_analyses, sarif_uploads, user_migrations, org_migrations,
//	discussions, discussion_categories, discussion_comments.
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
		{"commit_statuses", func(key string, raw []byte) error {
			var statuses []*CommitStatus
			if err := loadJSON(raw, &statuses); err != nil {
				return err
			}
			st.CommitStatuses.byKey[key] = statuses
			return nil
		}},
		{"commit_comments", func(_ string, raw []byte) error {
			var c CommitComment
			if err := loadJSON(raw, &c); err != nil {
				return err
			}
			st.CommitComments.byID[c.ID] = &c
			st.CommitComments.byRepo[c.RepoID] = append(st.CommitComments.byRepo[c.RepoID], &c)
			ck := commitKey(c.RepoID, c.CommitID)
			st.CommitComments.byCommit[ck] = append(st.CommitComments.byCommit[ck], &c)
			if c.ID >= st.CommitComments.nextID {
				st.CommitComments.nextID = c.ID + 1
			}
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
		{"repo_autolinks", func(key string, raw []byte) error {
			var autolinks map[int]*RepoAutolink
			if err := loadJSON(raw, &autolinks); err != nil {
				return err
			}
			for _, a := range autolinks {
				a.RepoKey = key
				if a.ID >= st.NextAutolinkID {
					st.NextAutolinkID = a.ID + 1
				}
			}
			st.RepoAutolinks[key] = autolinks
			return nil
		}},
		{"repo_invitations", func(key string, raw []byte) error {
			var invitations map[int]*RepoInvitation
			if err := loadJSON(raw, &invitations); err != nil {
				return err
			}
			for _, inv := range invitations {
				inv.RepoKey = key
				if inv.ID >= st.NextInvitationID {
					st.NextInvitationID = inv.ID + 1
				}
			}
			st.RepoInvitations[key] = invitations
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
			st.Misc.branchProtection[key] = &bp
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
		{"admin_audit_log", func(_ string, raw []byte) error {
			var e AuditLogEvent
			if err := loadJSON(raw, &e); err != nil {
				return err
			}
			if e.Timestamp != "" {
				if ts, err := time.Parse(time.RFC3339Nano, e.Timestamp); err == nil {
					e.createdAt = ts
				}
			}
			st.Misc.auditLogEvents = append(st.Misc.auditLogEvents, &e)
			if e.ID > st.Misc.nextAdminAuditID {
				st.Misc.nextAdminAuditID = e.ID
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
		{"notifications_state", func(key string, raw []byte) error {
			var state UserNotificationsState
			if err := loadJSON(raw, &state); err != nil {
				return err
			}
			userID, err := strconv.Atoi(key)
			if err != nil {
				return fmt.Errorf("notifications_state key %q: %w", key, err)
			}
			st.NotificationsState[userID] = &state
			return nil
		}},
		{"repo_rulesets", func(key string, raw []byte) error {
			var rs Ruleset
			if err := loadJSON(raw, &rs); err != nil {
				return err
			}
			if rs.Versions == nil {
				rs.Versions = map[int]RulesetVersion{}
			}
			st.Rulesets[rs.ID] = &rs
			if rs.ID >= st.NextRulesetID {
				st.NextRulesetID = rs.ID + 1
			}
			return nil
		}},
		{"projects_classic", func(_ string, raw []byte) error {
			var p ProjectClassic
			if err := loadJSON(raw, &p); err != nil {
				return err
			}
			st.ProjectClassic[p.ID] = &p
			if p.ID >= st.NextProjectClassicID {
				st.NextProjectClassicID = p.ID + 1
			}
			return nil
		}},
		{"project_columns", func(_ string, raw []byte) error {
			var c ProjectColumn
			if err := loadJSON(raw, &c); err != nil {
				return err
			}
			st.ProjectColumns[c.ID] = &c
			if c.ID >= st.NextProjectColumnID {
				st.NextProjectColumnID = c.ID + 1
			}
			return nil
		}},
		{"project_cards", func(_ string, raw []byte) error {
			var c ProjectCard
			if err := loadJSON(raw, &c); err != nil {
				return err
			}
			st.ProjectCards[c.ID] = &c
			if c.ID >= st.NextProjectCardID {
				st.NextProjectCardID = c.ID + 1
			}
			return nil
		}},
		{"secret_scanning_alerts", func(_ string, raw []byte) error {
			var a SecretScanningAlert
			if err := loadJSON(raw, &a); err != nil {
				return err
			}
			st.SecretScanningAlerts[a.ID] = &a
			if st.SecretScanningAlertsByRepo[a.RepoKey] == nil {
				st.SecretScanningAlertsByRepo[a.RepoKey] = make(map[int]*SecretScanningAlert)
			}
			st.SecretScanningAlertsByRepo[a.RepoKey][a.Number] = &a
			if a.Number >= st.SecretScanningNextNumber[a.RepoKey] {
				st.SecretScanningNextNumber[a.RepoKey] = a.Number + 1
			}
			if a.ID >= st.NextSecretScanningAlertID {
				st.NextSecretScanningAlertID = a.ID + 1
			}
			return nil
		}},
		{"code_scanning_alerts", func(_ string, raw []byte) error {
			var a CodeScanningAlert
			if err := loadJSON(raw, &a); err != nil {
				return err
			}
			st.CodeScanningAlerts[a.ID] = &a
			if st.CodeScanningAlertsByRepo[a.RepoKey] == nil {
				st.CodeScanningAlertsByRepo[a.RepoKey] = make(map[int]*CodeScanningAlert)
			}
			st.CodeScanningAlertsByRepo[a.RepoKey][a.Number] = &a
			if a.Number >= st.CodeScanningNextNumber[a.RepoKey] {
				st.CodeScanningNextNumber[a.RepoKey] = a.Number + 1
			}
			if a.ID >= st.NextCodeScanningAlertID {
				st.NextCodeScanningAlertID = a.ID + 1
			}
			return nil
		}},
		{"code_scanning_analyses", func(_ string, raw []byte) error {
			var a CodeScanningAnalysis
			if err := loadJSON(raw, &a); err != nil {
				return err
			}
			st.CodeScanningAnalyses[a.ID] = &a
			if st.CodeScanningAnalysesByRepo[a.RepoKey] == nil {
				st.CodeScanningAnalysesByRepo[a.RepoKey] = make(map[int]*CodeScanningAnalysis)
			}
			st.CodeScanningAnalysesByRepo[a.RepoKey][a.ID] = &a
			if a.ID >= st.NextCodeScanningAnalysisID {
				st.NextCodeScanningAnalysisID = a.ID + 1
			}
			return nil
		}},
		{"sarif_uploads", func(key string, raw []byte) error {
			var up SARIFUpload
			if err := loadJSON(raw, &up); err != nil {
				return err
			}
			st.SARIFUploads[key] = &up
			return nil
		}},
		{"dependabot_alerts", func(_ string, raw []byte) error {
			var a DependabotAlert
			if err := loadJSON(raw, &a); err != nil {
				return err
			}
			st.DependabotAlerts[a.ID] = &a
			if st.DependabotAlertsByRepo[a.RepoKey] == nil {
				st.DependabotAlertsByRepo[a.RepoKey] = make(map[int]*DependabotAlert)
			}
			st.DependabotAlertsByRepo[a.RepoKey][a.Number] = &a
			if a.Number >= st.DependabotNextNumber[a.RepoKey] {
				st.DependabotNextNumber[a.RepoKey] = a.Number + 1
			}
			if a.ID >= st.NextDependabotAlertID {
				st.NextDependabotAlertID = a.ID + 1
			}
			return nil
		}},
		{"dependabot_secrets", func(key string, raw []byte) error {
			var m map[string]*DependabotSecret
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.DependabotSecrets[key] = m
			return nil
		}},
		{"dependabot_org_secrets", func(key string, raw []byte) error {
			var m map[string]*DependabotOrgSecret
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.DependabotOrgSecrets[key] = m
			return nil
		}},
		{"user_migrations", func(_ string, raw []byte) error {
			var r userMigrationRecord
			if err := loadJSON(raw, &r); err != nil {
				return err
			}
			m := recordToUserMigration(&r)
			st.UserMigrations[m.ID] = m
			if m.ID >= st.NextUserMigrationID {
				st.NextUserMigrationID = m.ID + 1
			}
			return nil
		}},
		{"org_migrations", func(_ string, raw []byte) error {
			var r orgMigrationRecord
			if err := loadJSON(raw, &r); err != nil {
				return err
			}
			m := recordToOrgMigration(&r)
			st.OrgMigrations[m.ID] = m
			if m.ID >= st.NextOrgMigrationID {
				st.NextOrgMigrationID = m.ID + 1
			}
			return nil
		}},
		{"discussion_categories", func(_ string, raw []byte) error {
			var cat DiscussionCategory
			if err := loadJSON(raw, &cat); err != nil {
				return err
			}
			st.DiscussionCategories[cat.ID] = &cat
			if cat.ID >= st.NextDiscussionCategoryID {
				st.NextDiscussionCategoryID = cat.ID + 1
			}
			return nil
		}},
		{"discussions", func(_ string, raw []byte) error {
			var d Discussion
			if err := loadJSON(raw, &d); err != nil {
				return err
			}
			st.Discussions[d.ID] = &d
			if d.ID >= st.NextDiscussionID {
				st.NextDiscussionID = d.ID + 1
			}
			return nil
		}},
		{"discussion_comments", func(_ string, raw []byte) error {
			var c DiscussionComment
			if err := loadJSON(raw, &c); err != nil {
				return err
			}
			st.DiscussionComments[c.ID] = &c
			if c.ID >= st.NextDiscussionCommentID {
				st.NextDiscussionCommentID = c.ID + 1
			}
			return nil
		}},
		{"packages", func(_ string, raw []byte) error {
			var p Package
			if err := loadJSON(raw, &p); err != nil {
				return err
			}
			st.Packages[p.ID] = &p
			if st.PackagesByOwnerKey[p.OwnerKey] == nil {
				st.PackagesByOwnerKey[p.OwnerKey] = map[string]*Package{}
			}
			st.PackagesByOwnerKey[p.OwnerKey][packageKey(p.PackageType, p.Name)] = &p
			if p.ID >= st.NextPackageID {
				st.NextPackageID = p.ID + 1
			}
			return nil
		}},
		{"package_versions", func(_ string, raw []byte) error {
			var v PackageVersion
			if err := loadJSON(raw, &v); err != nil {
				return err
			}
			st.PackageVersions[v.ID] = &v
			if st.PackageVersionsByPackage[v.PackageID] == nil {
				st.PackageVersionsByPackage[v.PackageID] = map[int]*PackageVersion{}
			}
			st.PackageVersionsByPackage[v.PackageID][v.ID] = &v
			if v.ID >= st.NextPackageVersionID {
				st.NextPackageVersionID = v.ID + 1
			}
			return nil
		}},
		{"package_files", func(_ string, raw []byte) error {
			var f PackageFile
			if err := loadJSON(raw, &f); err != nil {
				return err
			}
			st.PackageFiles[f.ID] = &f
			if st.PackageFilesByVersion[f.VersionID] == nil {
				st.PackageFilesByVersion[f.VersionID] = map[int]*PackageFile{}
			}
			st.PackageFilesByVersion[f.VersionID][f.ID] = &f
			if f.ID >= st.NextPackageFileID {
				st.NextPackageFileID = f.ID + 1
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

	// Audit entries arrive in map-iteration order; the in-memory log is
	// newest-first (recordAuditEvent prepends), so sort by ID descending.
	sort.Slice(st.Misc.auditLog, func(i, j int) bool { return st.Misc.auditLog[i].ID > st.Misc.auditLog[j].ID })
	sort.Slice(st.Misc.auditLogEvents, func(i, j int) bool { return st.Misc.auditLogEvents[i].ID > st.Misc.auditLogEvents[j].ID })

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

// generateGistID creates a random 20-character hexadecimal gist ID.
func generateGistID() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateGist creates a new gist owned by the given user.
func (st *Store) CreateGist(owner *User, description string, public bool, files map[string]*GistFile) *Gist {
	st.mu.Lock()
	defer st.mu.Unlock()

	id := generateGistID()
	for st.Gists[id] != nil {
		id = generateGistID()
	}
	now := time.Now().UTC()
	g := &Gist{
		ID:          id,
		NodeID:      fmt.Sprintf("G_kwDOB%06d", st.NextGistID),
		Description: description,
		Public:      public,
		OwnerID:     owner.ID,
		Files:       files,
		CreatedAt:   now,
		UpdatedAt:   now,
		Comments:    0,
	}
	st.Gists[id] = g
	st.NextGistID++
	return g
}

// GetGist returns the gist with the given ID, or nil.
func (st *Store) GetGist(id string) *Gist {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Gists[id]
}

// UpdateGist replaces the gist fields and records a history entry.
func (st *Store) UpdateGist(id string, description *string, files map[string]*GistFile, deleteFiles []string) (*Gist, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	g := st.Gists[id]
	if g == nil {
		return nil, false
	}

	additions, deletions := 0, 0
	if description != nil {
		g.Description = *description
	}
	for name, f := range files {
		if _, existed := g.Files[name]; existed {
			deletions += len(g.Files[name].Content)
		} else {
			additions += len(f.Content)
		}
		g.Files[name] = f
	}
	for _, name := range deleteFiles {
		if f, ok := g.Files[name]; ok {
			deletions += len(f.Content)
			delete(g.Files, name)
		}
	}
	g.UpdatedAt = time.Now().UTC()

	version := generateGistID()
	g.History = append(g.History, &GistHistory{
		Version:     version,
		CommittedAt: g.UpdatedAt,
		ChangeStatus: map[string]int{
			"total":     additions + deletions,
			"additions": additions,
			"deletions": deletions,
		},
	})
	return g, true
}

// DeleteGist deletes a gist and all its comments.
func (st *Store) DeleteGist(id string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.Gists[id] == nil {
		return false
	}
	delete(st.Gists, id)
	for cid, c := range st.GistComments {
		if c.GistID == id {
			delete(st.GistComments, cid)
		}
	}
	for uid, stars := range st.StarredGists {
		if stars[id] {
			delete(stars, id)
			if len(stars) == 0 {
				delete(st.StarredGists, uid)
			}
		}
	}
	return true
}

// ListGistsForUser returns gists owned by the user, optionally filtered by since.
func (st *Store) ListGistsForUser(userID int, since time.Time) []*Gist {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*Gist
	for _, g := range st.Gists {
		if g.OwnerID == userID && !g.UpdatedAt.Before(since) {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

// ListPublicGists returns all public gists, newest first.
func (st *Store) ListPublicGists(since time.Time) []*Gist {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*Gist
	for _, g := range st.Gists {
		if g.Public && !g.UpdatedAt.Before(since) {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

// ListStarredGists returns gists starred by the user.
func (st *Store) ListStarredGists(userID int) []*Gist {
	st.mu.RLock()
	defer st.mu.RUnlock()
	stars, ok := st.StarredGists[userID]
	if !ok {
		return nil
	}
	var out []*Gist
	for id := range stars {
		if g := st.Gists[id]; g != nil {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

// StarGist stars a gist for the user.
func (st *Store) StarGist(userID int, gistID string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.Gists[gistID] == nil {
		return false
	}
	if st.StarredGists[userID] == nil {
		st.StarredGists[userID] = make(map[string]bool)
	}
	st.StarredGists[userID][gistID] = true
	return true
}

// UnstarGist unstars a gist for the user.
func (st *Store) UnstarGist(userID int, gistID string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.Gists[gistID] == nil {
		return false
	}
	if st.StarredGists[userID] != nil {
		delete(st.StarredGists[userID], gistID)
		if len(st.StarredGists[userID]) == 0 {
			delete(st.StarredGists, userID)
		}
	}
	return true
}

// IsGistStarred reports whether the user has starred the gist.
func (st *Store) IsGistStarred(userID int, gistID string) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	if st.Gists[gistID] == nil {
		return false
	}
	return st.StarredGists[userID][gistID]
}

// ForkGist forks a gist for the given user.
func (st *Store) ForkGist(user *User, gistID string) (*Gist, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	orig := st.Gists[gistID]
	if orig == nil {
		return nil, false
	}
	files := make(map[string]*GistFile, len(orig.Files))
	for name, f := range orig.Files {
		cp := *f
		files[name] = &cp
	}
	now := time.Now().UTC()
	id := generateGistID()
	for st.Gists[id] != nil {
		id = generateGistID()
	}
	fork := &Gist{
		ID:          id,
		NodeID:      fmt.Sprintf("G_kwDOB%06d", st.NextGistID),
		Description: orig.Description,
		Public:      orig.Public,
		OwnerID:     user.ID,
		Files:       files,
		CreatedAt:   now,
		UpdatedAt:   now,
		Comments:    0,
		ForkOfID:    orig.ID,
	}
	st.Gists[id] = fork
	orig.ForkIDs = append(orig.ForkIDs, id)
	st.NextGistID++
	return fork, true
}

// ListGistForks returns forks of a gist.
func (st *Store) ListGistForks(gistID string) []*Gist {
	st.mu.RLock()
	defer st.mu.RUnlock()
	orig := st.Gists[gistID]
	if orig == nil {
		return nil
	}
	var out []*Gist
	for _, fid := range orig.ForkIDs {
		if f := st.Gists[fid]; f != nil {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// CreateGistComment adds a comment to a gist.
func (st *Store) CreateGistComment(gistID string, user *User, body string) *GistComment {
	st.mu.Lock()
	defer st.mu.Unlock()
	g := st.Gists[gistID]
	if g == nil {
		return nil
	}
	now := time.Now().UTC()
	c := &GistComment{
		ID:                st.NextGistCommentID,
		NodeID:            fmt.Sprintf("GC_kwDOB%06d", st.NextGistCommentID),
		GistID:            gistID,
		UserID:            user.ID,
		Body:              body,
		CreatedAt:         now,
		UpdatedAt:         now,
		AuthorAssociation: "OWNER",
	}
	st.GistComments[c.ID] = c
	st.NextGistCommentID++
	g.Comments++
	return c
}

// GetGistComment returns a comment by ID.
func (st *Store) GetGistComment(id int) *GistComment {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.GistComments[id]
}

// UpdateGistComment updates a comment body.
func (st *Store) UpdateGistComment(id int, body string) (*GistComment, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	c := st.GistComments[id]
	if c == nil {
		return nil, false
	}
	c.Body = body
	c.UpdatedAt = time.Now().UTC()
	return c, true
}

// DeleteGistComment deletes a comment and decrements the gist comment count.
func (st *Store) DeleteGistComment(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	c := st.GistComments[id]
	if c == nil {
		return false
	}
	if g := st.Gists[c.GistID]; g != nil {
		g.Comments--
	}
	delete(st.GistComments, id)
	return true
}

// ListGistComments returns comments for a gist, oldest first.
func (st *Store) ListGistComments(gistID string) []*GistComment {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*GistComment
	for _, c := range st.GistComments {
		if c.GistID == gistID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}
