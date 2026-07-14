package bleephub

import (
	"bytes"
	"context"
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
	"golang.org/x/crypto/ssh"
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
	Suspended    bool            `json:"suspended,omitempty"`
	StarredRepos map[string]bool `json:"starred_repos,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	// user-surface profile fields (PATCH /user), email addresses, and
	// account-level interaction limits.
	Blog                   string      `json:"blog,omitempty"`
	Company                string      `json:"company,omitempty"`
	Location               string      `json:"location,omitempty"`
	TwitterUsername        string      `json:"twitter_username,omitempty"`
	Hireable               *bool       `json:"hireable,omitempty"`
	Emails                 []UserEmail `json:"emails,omitempty"`
	InteractionLimit       string      `json:"interaction_limit,omitempty"`
	InteractionLimitExpiry *time.Time  `json:"interaction_limit_expiry,omitempty"`
	PasswordHash           string      `json:"password_hash,omitempty"`
}

// UserEmail is one email address on a user account, matching GitHub's
// `email` schema (primary/verified/visibility).
type UserEmail struct {
	Email      string `json:"email"`
	Primary    bool   `json:"primary"`
	Verified   bool   `json:"verified"`
	Visibility string `json:"visibility,omitempty"` // "public", "private", or "" (null)
}

// Token represents a personal access token.
type Token struct {
	Value               string
	UserID              int
	Scopes              string
	CreatedAt           time.Time
	FineGrained         bool              `json:"fine_grained,omitempty"`
	FineGrainedID       int               `json:"fine_grained_id,omitempty"`
	Name                string            `json:"name,omitempty"`
	ResourceOwner       string            `json:"resource_owner,omitempty"`
	RepositorySelection string            `json:"repository_selection,omitempty"`
	RepositoryIDs       []int             `json:"repository_ids,omitempty"`
	Permissions         OrgPATPermissions `json:"permissions,omitempty"`
	ExpiresAt           *time.Time        `json:"expires_at,omitempty"`
}

// DeviceCode represents a pending device authorization flow.
type DeviceCode struct {
	Code          string
	UserCode      string
	ClientID      string
	Scopes        string
	Token         string
	UserID        int
	AppID         int
	OAuthClientID string
	ApprovedAt    time.Time
	ExpiresAt     time.Time
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
	OwnerID     int                  `json:"owner_id"`
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
	ForkOfID    string               `json:"fork_of_id,omitempty"`
	ForkIDs     []string             `json:"fork_ids,omitempty"`
}

// GistComment is a comment on a gist.
type GistComment struct {
	ID                int       `json:"id"`
	NodeID            string    `json:"node_id"`
	GistID            string    `json:"gist_id"`
	UserID            int       `json:"user_id"`
	Body              string    `json:"body"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	AuthorAssociation string    `json:"author_association"`
	URL               string    `json:"url"`
}

type persistedStarredGists struct {
	UserID int             `json:"user_id"`
	Stars  map[string]bool `json:"stars"`
}

// Store holds all in-memory state for bleephub.
type Store struct {
	Agents                       map[int]*Agent
	Sessions                     map[string]*Session
	Jobs                         map[string]*Job
	Users                        map[int]*User
	UsersByLogin                 map[string]*User
	Tokens                       map[string]*Token
	DeviceCodes                  map[string]*DeviceCode
	AuthCodes                    map[string]*authCode     // OAuth web-flow codes
	LoginSessions                map[string]*LoginSession // _gh_sess cookie value → session
	Repos                        map[int]*Repo
	ReposByName                  map[string]*Repo                       // "owner/name" → repo
	GitStorages                  map[string]gitStorage.Storer           // "owner/name" → go-git storage (memory or filesystem)
	Orgs                         map[int]*Org                           // id → org
	OrgsByLogin                  map[string]*Org                        // login → org
	Teams                        map[int]*Team                          // id → team
	TeamsBySlug                  map[string]*Team                       // "org/slug" → team
	Memberships                  map[string]*Membership                 // "org/user" → membership
	Issues                       map[int]*Issue                         // id → issue
	IssuesByRepo                 map[int]map[int]*Issue                 // repoID → number → issue (secondary index)
	Labels                       map[int]*IssueLabel                    // id → label
	Milestones                   map[int]*Milestone                     // id → milestone
	Comments                     map[int]*Comment                       // id → comment
	CommentCounts                map[string]int                         // "parentType\x1fparentID" → comment count (index)
	IssueEvents                  map[int]*IssueEvent                    // id → issue event
	PullRequests                 map[int]*PullRequest                   // id → PR
	PullsByRepo                  map[int]map[int]*PullRequest           // repoID → number → PR (secondary index)
	PRReviews                    map[int]*PullRequestReview             // id → review
	PRReviewsByPR                map[int][]*PullRequestReview           // PR id → reviews (secondary index)
	Workflows                    map[string]*Workflow                   // id → workflow (run-level)
	WorkflowFiles                map[int64]*WorkflowFile                // id → workflow file (file-level)
	PendingMessages              []*TaskAgentMessage                    // messages awaiting delivery
	RepoSecrets                  map[string]map[string]*Secret          // "owner/repo" → name → secret
	RepoVariables                map[string]map[string]*ActionsVariable // "owner/repo" → NAME → variable
	RepoCollaborators            map[string]map[string]string           // "owner/repo" → login → permission (pull/push/admin)
	RepoAutolinks                map[string]map[int]*RepoAutolink       // "owner/repo" → id → autolink
	RepoInvitations              map[string]map[int]*RepoInvitation     // "owner/repo" → id → invitation
	RepoDeployKeys               map[string]map[int]*RepoDeployKey      // "owner/repo" → id → deploy key
	RepoSubscriptions            map[string]*RepoSubscription           // "userID:repoID" → subscription
	OrgSecrets                   map[string]map[string]*OrgSecret       // org login → NAME → org secret
	OrgVariables                 map[string]map[string]*ActionsVariable // org login → NAME → org variable
	EnvSecrets                   map[string]map[string]*Secret          // envScopeKey(repo, env) → NAME → secret
	EnvVariables                 map[string]map[string]*ActionsVariable // envScopeKey(repo, env) → NAME → variable
	TimelineRecords              map[string][]*TimelineRecord           // planID → runner-uploaded timeline records
	LogFiles                     map[int][]byte                         // logID → uploaded runner log content
	WorkflowAttempts             map[int][]*Workflow                    // runID → prior attempts (oldest first)
	RunnerGroups                 map[int]*RunnerGroup                   // org runner groups (global pool overlay)
	NextRunnerGroupID            int
	Hooks                        map[string][]*Webhook         // "owner/repo" → hooks
	OrgHooks                     map[string][]*Webhook         // org login → org-level hooks
	HookDeliveries               map[int][]*WebhookDelivery    // hookID → deliveries
	Apps                         map[int]*App                  // id → app
	AppsBySlug                   map[string]*App               // slug → app
	AppsByClientID               map[string]*App               // OAuth client_id → app
	OAuthApps                    map[string]*OAuthApp          // OAuth client_id → OAuth app (distinct from GitHub App)
	Installations                map[int]*Installation         // id → installation
	InstallationTokens           map[string]*InstallationToken // token value → token
	UserToServerTokens           map[string]*UserToServerToken // gho_/ghu_ token value → token
	RefreshTokens                map[string]*RefreshToken      // ghr_ token value → refresh token
	AppHookDeliveries            map[int][]*WebhookDelivery    // appID → app-level webhook deliveries
	ManifestCodes                map[string]int                // code → appID (one-time-use)
	CheckRuns                    map[int64]*CheckRun           // id → check run
	CheckSuites                  map[int64]*CheckSuite         // id → check suite
	CheckSuitePrefs              map[string][]*CheckSuitePref  // repoKey → autoTrigger prefs
	CommitStatuses               *CommitStatusStore            // commit status contexts per repo+ref
	CommitComments               *CommitCommentStore           // commit comments per repo/commit
	Reactions                    *ReactionStore                // reactions across all parent types
	Releases                     *ReleaseStore                 // release CRUD
	Deployments                  *DeploymentStore              // deployments + statuses + environments
	PRReviewComments             *PRReviewCommentStore         // PR review comments (inline / threads)
	Misc                         *MiscStore                    // long-tail surfaces
	ProjectsV2                   *ProjectV2Store               // GitHub Projects v2
	NotificationsState           map[int]*UserNotificationsState
	Rulesets                     map[int]*Ruleset
	ProjectClassic               map[int]*ProjectClassic                // id → project
	ProjectColumns               map[int]*ProjectColumn                 // id → column
	ProjectCards                 map[int]*ProjectCard                   // id → card
	UserMigrations               map[int]*UserMigration                 // id → user migration
	OrgMigrations                map[int]*OrgMigration                  // id → org migration
	Codespaces                   map[int]*Codespace                     // id → codespace
	CodespacesByName             map[string]*Codespace                  // name → codespace
	CodespaceSecrets             map[string]map[string]*CodespaceSecret // scope\x1fname → secret
	NextCodespaceID              int
	NextCodespaceSecretID        int
	LogLines                     map[string][]string     // jobID → captured console log lines
	Gists                        map[string]*Gist        // id → gist
	GistComments                 map[int]*GistComment    // id → gist comment
	StarredGists                 map[int]map[string]bool // userID → gistID → starred
	SecretScanningAlerts         map[int]*SecretScanningAlert
	SecretScanningAlertsByRepo   map[string]map[int]*SecretScanningAlert // repoKey → alertNumber → alert
	SecretScanningNextNumber     map[string]int                          // repoKey → next alert number
	CodeScanningAlerts           map[int]*CodeScanningAlert
	CodeScanningAlertsByRepo     map[string]map[int]*CodeScanningAlert // repoKey → alertNumber → alert
	CodeScanningNextNumber       map[string]int                        // repoKey → next alert number
	CodeScanningAnalyses         map[int]*CodeScanningAnalysis
	CodeScanningAnalysesByRepo   map[string]map[int]*CodeScanningAnalysis // repoKey → analysisID → analysis
	CodeScanningDefaultSetups    map[string]*CodeScanningDefaultSetup     // repoKey → default setup
	SARIFUploads                 map[string]*SARIFUpload                  // uploadID → upload
	DependabotAlerts             map[int]*DependabotAlert
	DependabotAlertsByRepo       map[string]map[int]*DependabotAlert         // repoKey → alertNumber → alert
	DependabotNextNumber         map[string]int                              // repoKey → next alert number
	DependabotSecrets            map[string]map[string]*DependabotSecret     // repoKey → name → secret
	DependabotOrgSecrets         map[string]map[string]*DependabotOrgSecret  // orgLogin → name → secret
	DependabotUserSecrets        map[string]map[string]*DependabotUserSecret // userLogin → name → secret
	DependabotRepositoryAccess   map[string][]int                            // orgLogin → repo IDs
	SecurityAdvisories           map[int]*SecurityAdvisory
	SecurityAdvisoriesByRepo     map[string]map[string]*SecurityAdvisory // repoKey → GHSA ID → advisory
	SecurityAdvisoryReports      map[int]*SecurityAdvisoryReport
	Packages                     map[int]*Package
	PackageVersions              map[int]*PackageVersion
	PackageFiles                 map[int]*PackageFile
	PackagesByOwnerKey           map[string]map[string]*Package  // ownerKey → packageKey → package
	PackageVersionsByPackage     map[int]map[int]*PackageVersion // packageID → versionID → version
	PackageFilesByVersion        map[int]map[int]*PackageFile    // versionID → fileID → file
	PackageDataDir               string                          // directory for package file bytes
	ObjectByteStore              actionsByteStore                // object storage for durable service bytes
	NextGistID                   int
	NextGistCommentID            int
	NextAgent                    int
	NextSecretScanningAlertID    int
	NextCodeScanningAlertID      int
	NextCodeScanningAnalysisID   int
	NextDependabotAlertID        int
	NextPackageID                int
	NextPackageVersionID         int
	NextPackageFileID            int
	NextMsg                      int64
	NextLog                      int
	NextReqID                    int64
	NextUser                     int
	NextRepo                     int
	NextOrg                      int
	NextTeam                     int
	NextIssue                    int
	NextLabel                    int
	NextMilestone                int
	NextComment                  int
	NextIssueEventID             int
	NextPR                       int
	NextPRReview                 int
	NextRunID                    int
	NextHookID                   int
	NextDeliveryID               int
	NextAppID                    int
	NextInstallationID           int
	NextCheckRunID               int64
	NextCheckSuiteID             int64
	NextRulesetID                int
	NextProjectClassicID         int
	NextProjectColumnID          int
	NextProjectCardID            int
	NextUserMigrationID          int
	NextOrgMigrationID           int
	NextAutolinkID               int
	NextInvitationID             int
	NextDeployKeyID              int
	NextSecurityAdvisoryID       int
	NextSecurityAdvisoryReportID int
	Discussions                  map[int]*Discussion
	DiscussionCategories         map[int]*DiscussionCategory
	DiscussionComments           map[int]*DiscussionComment
	NextDiscussionID             int
	NextDiscussionCategoryID     int
	NextDiscussionCommentID      int
	OrgActionsPermissions        map[string]*OrgActionsPermissions
	RepoActionsPermissions       map[string]*RepoActionsPermissions
	actionsKeyPair               *SecretsKeyPair // lazily generated sealed-box keypair (persisted)
	persist                      *Persistence
	// mu guards the Store's maps and counters. sync.RWMutex read locks are
	// NOT reentrant: once a writer queues on Lock, new RLock calls block, so
	// a goroutine that re-acquires mu while already holding it deadlocks.
	// The invariants that keep that impossible:
	//   - Public Store methods and JSON serializers (repoToJSON, issueToJSON,
	//     canReadRepo, …) acquire mu themselves and must never be called with
	//     mu held.
	//   - Helpers named xxxLocked (and helpers documented "callers hold
	//     st.mu") never acquire mu; they run under the caller's lock.
	//   - Handlers and store methods that need both a coherent scan AND
	//     rendered JSON gather rows under one RLock, release, then render
	//     with the self-locking serializers (see deriveActivityEvents).
	//   - Lock order between mutexes: Store.mu is acquired before any
	//     sub-store mutex (Misc.mu, Reactions.mu, Releases.mu, persistence),
	//     never the reverse.
	mu sync.RWMutex
	// enterprises
	EnterpriseTeams                    map[int]*EnterpriseTeam
	EnterpriseTeamsBySlug              map[string]*EnterpriseTeam
	EnterpriseCodeSecurityConfigs      map[int]*EnterpriseCodeSecurityConfiguration
	EnterpriseCodeSecurityRepoConfigs  map[int]int // repoID → attached config ID
	EnterpriseSettings                 *EnterpriseSettings
	NextEnterpriseTeamID               int
	NextEnterpriseCodeSecurityConfigID int

	// attestations + org artifact metadata
	Attestations                   map[int]*Attestation // id → attestation
	NextAttestationID              int
	ArtifactStorageRecords         map[int]*ArtifactStorageRecord // id → storage record
	NextArtifactStorageRecordID    int
	ArtifactDeploymentRecords      map[int]*ArtifactDeploymentRecord // id → deployment record
	NextArtifactDeploymentRecordID int

	// copilot + code quality (gh_copilot.go, gh_copilot_spaces.go, gh_code_quality.go)
	CopilotSeats             map[string]map[int]*CopilotSeat           // org login → user ID → seat
	CopilotContentExclusions map[string]*CopilotContentExclusion       // org login → rules
	CopilotCodingAgentPerms  map[string]*CopilotCodingAgentPermissions // org login → policy
	CopilotSpaces            map[int64]*CopilotSpace                   // space ID → space
	NextCopilotSpaceID       int64
	CodeQualitySetups        map[string]*CodeQualitySetup // repo full name → setup

	// org governance surfaces (code security configurations, custom
	// properties, issue types, issue fields, security campaigns, private
	// registries, hosted compute network configurations, immutable releases)
	CodeSecurityConfigs         map[string]map[int]*CodeSecurityConfiguration // org login → id → configuration
	CodeSecurityRepoAttachments map[string]map[int]int                        // org login → repo ID → configuration ID
	NextCodeSecurityConfigID    int
	OrgCustomProperties         map[string]map[string]*CustomProperty // org login → property name → definition
	RepoCustomPropertyValues    map[string]map[string]interface{}     // "owner/repo" → property name → value
	OrgIssueTypes               map[string]map[int]*IssueType         // org login → id → issue type
	NextIssueTypeID             int
	OrgIssueFields              map[string]map[int]*IssueField // org login → id → issue field
	NextIssueFieldID            int
	NextIssueFieldOptionID      int
	IssueFieldValues            map[int]map[int]interface{}                         // issue ID → field ID → raw value
	OrgCampaigns                map[string]map[int]*Campaign                        // org login → campaign number → campaign
	OrgPrivateRegistries        map[string]map[string]*PrivateRegistryConfiguration // org login → name → configuration
	OrgNetworkConfigurations    map[string]map[string]*NetworkConfiguration         // org login → id → configuration
	OrgNetworkSettings          map[string]map[string]*NetworkSettingsResource      // org login → id → settings resource
	OrgImmutableReleases        map[string]*OrgImmutableReleasesSettings            // org login → enforcement policy
	RepoImmutableReleases       map[string]bool                                     // "owner/repo" → repo-level enablement

	// hosted-runners
	HostedRunners            map[int]*HostedRunner
	NextHostedRunnerID       int
	HostedRunnerCustomImages map[int]*HostedRunnerCustomImage
	NextHostedRunnerImageID  int
	// actions-oidc-properties
	OrgOIDCPropertyInclusions map[string][]string

	// agents-codescan: GitHub Copilot coding agent secrets/variables/tasks
	// and CodeQL databases/variant analyses.
	AgentsRepoSecrets           map[string]map[string]*Secret          // "owner/repo" → NAME → secret
	AgentsOrgSecrets            map[string]map[string]*OrgSecret       // org login → NAME → org secret
	AgentsRepoVariables         map[string]map[string]*ActionsVariable // "owner/repo" → NAME → variable
	AgentsOrgVariables          map[string]map[string]*ActionsVariable // org login → NAME → org variable
	AgentTasks                  map[string]*AgentTask                  // task ID (UUID) → task
	CodeScanningAutofixes       map[string]*CodeScanningAutofix        // autofixKey(repoKey, number) → autofix
	CodeQLDatabases             map[int]*CodeQLDatabase                // id → database
	CodeQLDatabasesByRepo       map[string]map[string]*CodeQLDatabase  // repoKey → language → database
	CodeQLVariantAnalyses       map[int]*CodeQLVariantAnalysis         // id → variant analysis
	NextCodeQLDatabaseID        int
	NextCodeQLVariantAnalysisID int
	// teams-people
	OrgInvitations         map[int]*OrgInvitation          // id → org invitation
	NextOrgInvitationID    int                             //
	OrgBlocks              map[string]map[int]time.Time    // orgLogin → blocked userID → blocked-at
	OrgInteractionLimits   map[string]*OrgInteractionLimit // orgLogin → active interaction limit
	OrgRoleTeamAssignments map[string]map[int][]int        // orgLogin → roleID → team IDs
	OrgRoleUserAssignments map[string]map[int][]int        // orgLogin → roleID → user IDs
	// org billing budgets (gh_org_billing.go)
	OrgBudgets map[string]map[string]*OrgBudget // org login → budget ID → budget
	// API insights (gh_api_insights.go)
	APIRequestRecords []*APIRequestRecord // ordered by ID (oldest first)
	NextAPIRequestID  int64
	// fine-grained personal access token administration (gh_org_pat_admin.go)
	OrgPATGrantRequests map[string]map[int]*OrgPATGrantRequest // org login → request ID → request
	OrgPATGrants        map[string]map[int]*OrgPATGrant        // org login → grant ID → grant
	NextPATRequestID    int
	NextPATGrantID      int
	NextPATTokenID      int
	// org codespaces access settings (gh_codespaces.go)
	OrgCodespacesAccess map[string]*OrgCodespacesAccess // org login → access settings
	// Dependabot repository access default level (gh_dependabot.go)
	DependabotRepoAccessDefaultLevel map[string]string // org login → "public" | "internal"
	// secret scanning pattern configurations + push protection (gh_secret_scanning.go)
	SecretScanningPatternConfigs   map[string]*OrgSecretScanningPatternConfig                     // org login → config
	SecretScanningPushPlaceholders map[string]map[string]*SecretScanningPushProtectionPlaceholder // repoKey → placeholder ID → placeholder
	SecretScanningPushBypasses     map[string][]*SecretScanningPushProtectionBypass               // repoKey → bypasses

	// repo-write surfaces
	PagesDeployments         map[int]map[int]*PagesDeploymentRecord // repoID → deployment ID → record
	NextPagesDeploymentID    int
	EnvBranchPolicies        map[int][]*DeploymentBranchPolicyRule // environment ID → ordered branch/tag policies
	NextEnvBranchPolicyID    int
	EnvProtectionRules       map[int][]*EnvCustomProtectionRule // environment ID → enabled custom protection rules
	NextEnvProtectionRuleID  int
	SubIssueLists            map[int][]int                 // parent issue ID → ordered sub-issue IDs
	SubIssueParent           map[int]int                   // sub-issue ID → parent issue ID
	IssueBlockedBy           map[int][]int                 // issue ID → IDs of the issues blocking it
	RepoImports              map[int]*RepoImport           // repoID → source import
	DependencySnapshots      map[int][]*DependencySnapshot // repoID → submitted snapshots (oldest first)
	NextDependencySnapshotID int
	SBOMExports              map[string]*SBOMExport // export uuid → SBOM report export

	// GitHub Classroom
	Classrooms                   map[int]*Classroom
	ClassroomAssignments         map[int]*ClassroomAssignment
	ClassroomAcceptedAssignments map[int]*ClassroomAcceptedAssignment
	NextClassroomID              int
	NextClassroomAssignmentID    int
	NextClassroomAcceptedID      int

	// repo-reads
	RepoActivities   map[int]*RepoActivity         // id → recorded ref update (push activity)
	NextRepoActivity int                           // next RepoActivity ID
	RepoCloneTraffic map[string]*RepoTrafficBucket // "repoID:YYYY-MM-DD" → clone counters
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
		Agents:                       make(map[int]*Agent),
		Sessions:                     make(map[string]*Session),
		Jobs:                         make(map[string]*Job),
		Users:                        make(map[int]*User),
		UsersByLogin:                 make(map[string]*User),
		Tokens:                       make(map[string]*Token),
		DeviceCodes:                  make(map[string]*DeviceCode),
		AuthCodes:                    make(map[string]*authCode),
		LoginSessions:                make(map[string]*LoginSession),
		Repos:                        make(map[int]*Repo),
		ReposByName:                  make(map[string]*Repo),
		GitStorages:                  make(map[string]gitStorage.Storer),
		Orgs:                         make(map[int]*Org),
		OrgsByLogin:                  make(map[string]*Org),
		Teams:                        make(map[int]*Team),
		TeamsBySlug:                  make(map[string]*Team),
		Memberships:                  make(map[string]*Membership),
		Issues:                       make(map[int]*Issue),
		IssuesByRepo:                 make(map[int]map[int]*Issue),
		Labels:                       make(map[int]*IssueLabel),
		Milestones:                   make(map[int]*Milestone),
		Comments:                     make(map[int]*Comment),
		CommentCounts:                make(map[string]int),
		IssueEvents:                  make(map[int]*IssueEvent),
		PullRequests:                 make(map[int]*PullRequest),
		PullsByRepo:                  make(map[int]map[int]*PullRequest),
		PRReviews:                    make(map[int]*PullRequestReview),
		PRReviewsByPR:                make(map[int][]*PullRequestReview),
		Workflows:                    make(map[string]*Workflow),
		WorkflowFiles:                make(map[int64]*WorkflowFile),
		RepoSecrets:                  make(map[string]map[string]*Secret),
		RepoVariables:                make(map[string]map[string]*ActionsVariable),
		RepoCollaborators:            make(map[string]map[string]string),
		RepoAutolinks:                make(map[string]map[int]*RepoAutolink),
		RepoInvitations:              make(map[string]map[int]*RepoInvitation),
		RepoDeployKeys:               make(map[string]map[int]*RepoDeployKey),
		RepoSubscriptions:            map[string]*RepoSubscription{},
		OrgSecrets:                   make(map[string]map[string]*OrgSecret),
		OrgVariables:                 make(map[string]map[string]*ActionsVariable),
		EnvSecrets:                   make(map[string]map[string]*Secret),
		EnvVariables:                 make(map[string]map[string]*ActionsVariable),
		TimelineRecords:              make(map[string][]*TimelineRecord),
		LogFiles:                     make(map[int][]byte),
		WorkflowAttempts:             make(map[int][]*Workflow),
		RunnerGroups:                 make(map[int]*RunnerGroup),
		NextRunnerGroupID:            2,
		Hooks:                        make(map[string][]*Webhook),
		OrgHooks:                     make(map[string][]*Webhook),
		HookDeliveries:               make(map[int][]*WebhookDelivery),
		Apps:                         make(map[int]*App),
		AppsBySlug:                   make(map[string]*App),
		AppsByClientID:               make(map[string]*App),
		OAuthApps:                    make(map[string]*OAuthApp),
		Installations:                make(map[int]*Installation),
		InstallationTokens:           make(map[string]*InstallationToken),
		UserToServerTokens:           make(map[string]*UserToServerToken),
		RefreshTokens:                make(map[string]*RefreshToken),
		AppHookDeliveries:            make(map[int][]*WebhookDelivery),
		ManifestCodes:                make(map[string]int),
		CheckRuns:                    make(map[int64]*CheckRun),
		CheckSuites:                  make(map[int64]*CheckSuite),
		CheckSuitePrefs:              make(map[string][]*CheckSuitePref),
		CommitStatuses:               newCommitStatusStore(nil),
		CommitComments:               newCommitCommentStore(nil),
		Reactions:                    newReactionStore(nil),
		Releases:                     newReleaseStore(nil),
		Deployments:                  newDeploymentStore(nil),
		PRReviewComments:             newPRReviewCommentStore(nil),
		Misc:                         newMiscStore(),
		ProjectsV2:                   newProjectV2Store(nil),
		NotificationsState:           map[int]*UserNotificationsState{},
		Rulesets:                     map[int]*Ruleset{},
		ProjectClassic:               map[int]*ProjectClassic{},
		ProjectColumns:               map[int]*ProjectColumn{},
		ProjectCards:                 map[int]*ProjectCard{},
		UserMigrations:               map[int]*UserMigration{},
		OrgMigrations:                map[int]*OrgMigration{},
		Codespaces:                   map[int]*Codespace{},
		CodespacesByName:             map[string]*Codespace{},
		CodespaceSecrets:             map[string]map[string]*CodespaceSecret{},
		LogLines:                     make(map[string][]string),
		Gists:                        make(map[string]*Gist),
		GistComments:                 make(map[int]*GistComment),
		StarredGists:                 make(map[int]map[string]bool),
		SecretScanningAlerts:         make(map[int]*SecretScanningAlert),
		SecretScanningAlertsByRepo:   make(map[string]map[int]*SecretScanningAlert),
		SecretScanningNextNumber:     make(map[string]int),
		CodeScanningAlerts:           make(map[int]*CodeScanningAlert),
		CodeScanningAlertsByRepo:     make(map[string]map[int]*CodeScanningAlert),
		CodeScanningNextNumber:       make(map[string]int),
		CodeScanningAnalyses:         make(map[int]*CodeScanningAnalysis),
		CodeScanningAnalysesByRepo:   make(map[string]map[int]*CodeScanningAnalysis),
		CodeScanningDefaultSetups:    make(map[string]*CodeScanningDefaultSetup),
		SARIFUploads:                 make(map[string]*SARIFUpload),
		DependabotAlerts:             make(map[int]*DependabotAlert),
		DependabotAlertsByRepo:       make(map[string]map[int]*DependabotAlert),
		DependabotNextNumber:         make(map[string]int),
		DependabotSecrets:            make(map[string]map[string]*DependabotSecret),
		DependabotOrgSecrets:         make(map[string]map[string]*DependabotOrgSecret),
		DependabotUserSecrets:        make(map[string]map[string]*DependabotUserSecret),
		DependabotRepositoryAccess:   map[string][]int{},
		SecurityAdvisories:           map[int]*SecurityAdvisory{},
		SecurityAdvisoriesByRepo:     map[string]map[string]*SecurityAdvisory{},
		SecurityAdvisoryReports:      map[int]*SecurityAdvisoryReport{},
		Packages:                     map[int]*Package{},
		PackageVersions:              map[int]*PackageVersion{},
		PackageFiles:                 map[int]*PackageFile{},
		PackagesByOwnerKey:           map[string]map[string]*Package{},
		PackageVersionsByPackage:     map[int]map[int]*PackageVersion{},
		PackageFilesByVersion:        map[int]map[int]*PackageFile{},
		Discussions:                  map[int]*Discussion{},
		DiscussionCategories:         map[int]*DiscussionCategory{},
		DiscussionComments:           map[int]*DiscussionComment{},
		OrgActionsPermissions:        map[string]*OrgActionsPermissions{},
		RepoActionsPermissions:       map[string]*RepoActionsPermissions{},
		NextAgent:                    1,
		NextSecretScanningAlertID:    1,
		NextCodeScanningAlertID:      1,
		NextCodeScanningAnalysisID:   1,
		NextDependabotAlertID:        1,
		NextPackageID:                1,
		NextPackageVersionID:         1,
		NextPackageFileID:            1,
		NextMsg:                      1,
		NextLog:                      1,
		NextReqID:                    1,
		NextUser:                     1,
		NextRepo:                     1,
		NextOrg:                      1,
		NextTeam:                     1,
		NextIssue:                    1,
		NextLabel:                    1,
		NextMilestone:                1,
		NextComment:                  1,
		NextPR:                       1,
		NextPRReview:                 1,
		NextRunID:                    1,
		NextHookID:                   1,
		NextDeliveryID:               1,
		NextAppID:                    1,
		NextInstallationID:           1,
		NextCheckRunID:               1,
		NextCheckSuiteID:             1,
		NextRulesetID:                1,
		NextProjectClassicID:         1,
		NextProjectColumnID:          1,
		NextProjectCardID:            1,
		NextUserMigrationID:          1,
		NextOrgMigrationID:           1,
		NextCodespaceID:              1,
		NextCodespaceSecretID:        1,
		NextAutolinkID:               1,
		NextInvitationID:             1,
		NextIssueEventID:             1,
		NextDeployKeyID:              1,
		NextSecurityAdvisoryID:       1,
		NextSecurityAdvisoryReportID: 1,
		NextDiscussionID:             1,
		NextDiscussionCategoryID:     1,
		NextDiscussionCommentID:      1,
		// enterprises
		EnterpriseTeams:                    map[int]*EnterpriseTeam{},
		EnterpriseTeamsBySlug:              map[string]*EnterpriseTeam{},
		EnterpriseCodeSecurityConfigs:      map[int]*EnterpriseCodeSecurityConfiguration{},
		EnterpriseCodeSecurityRepoConfigs:  map[int]int{},
		EnterpriseSettings:                 defaultEnterpriseSettings(),
		NextEnterpriseTeamID:               1,
		NextEnterpriseCodeSecurityConfigID: 1,

		// attestations + org artifact metadata
		Attestations:                   map[int]*Attestation{},
		NextAttestationID:              1,
		ArtifactStorageRecords:         map[int]*ArtifactStorageRecord{},
		NextArtifactStorageRecordID:    1,
		ArtifactDeploymentRecords:      map[int]*ArtifactDeploymentRecord{},
		NextArtifactDeploymentRecordID: 1,

		// copilot + code quality
		CopilotSeats:             make(map[string]map[int]*CopilotSeat),
		CopilotContentExclusions: make(map[string]*CopilotContentExclusion),
		CopilotCodingAgentPerms:  make(map[string]*CopilotCodingAgentPermissions),
		CopilotSpaces:            make(map[int64]*CopilotSpace),
		NextCopilotSpaceID:       1,
		CodeQualitySetups:        make(map[string]*CodeQualitySetup),

		// org governance surfaces
		CodeSecurityConfigs:         map[string]map[int]*CodeSecurityConfiguration{},
		CodeSecurityRepoAttachments: map[string]map[int]int{},
		NextCodeSecurityConfigID:    1,
		OrgCustomProperties:         map[string]map[string]*CustomProperty{},
		RepoCustomPropertyValues:    map[string]map[string]interface{}{},
		OrgIssueTypes:               map[string]map[int]*IssueType{},
		NextIssueTypeID:             1,
		OrgIssueFields:              map[string]map[int]*IssueField{},
		NextIssueFieldID:            1,
		NextIssueFieldOptionID:      1,
		IssueFieldValues:            map[int]map[int]interface{}{},
		OrgCampaigns:                map[string]map[int]*Campaign{},
		OrgPrivateRegistries:        map[string]map[string]*PrivateRegistryConfiguration{},
		OrgNetworkConfigurations:    map[string]map[string]*NetworkConfiguration{},
		OrgNetworkSettings:          map[string]map[string]*NetworkSettingsResource{},
		OrgImmutableReleases:        map[string]*OrgImmutableReleasesSettings{},
		RepoImmutableReleases:       map[string]bool{},

		// hosted-runners
		HostedRunners:            map[int]*HostedRunner{},
		NextHostedRunnerID:       1,
		HostedRunnerCustomImages: map[int]*HostedRunnerCustomImage{},
		NextHostedRunnerImageID:  1,
		// actions-oidc-properties
		OrgOIDCPropertyInclusions: map[string][]string{},

		// agents-codescan
		AgentsRepoSecrets:           make(map[string]map[string]*Secret),
		AgentsOrgSecrets:            make(map[string]map[string]*OrgSecret),
		AgentsRepoVariables:         make(map[string]map[string]*ActionsVariable),
		AgentsOrgVariables:          make(map[string]map[string]*ActionsVariable),
		AgentTasks:                  make(map[string]*AgentTask),
		CodeScanningAutofixes:       make(map[string]*CodeScanningAutofix),
		CodeQLDatabases:             make(map[int]*CodeQLDatabase),
		CodeQLDatabasesByRepo:       make(map[string]map[string]*CodeQLDatabase),
		CodeQLVariantAnalyses:       make(map[int]*CodeQLVariantAnalysis),
		NextCodeQLDatabaseID:        1,
		NextCodeQLVariantAnalysisID: 1,
		// teams-people
		OrgInvitations:         map[int]*OrgInvitation{},
		NextOrgInvitationID:    1,
		OrgBlocks:              map[string]map[int]time.Time{},
		OrgInteractionLimits:   map[string]*OrgInteractionLimit{},
		OrgRoleTeamAssignments: map[string]map[int][]int{},
		OrgRoleUserAssignments: map[string]map[int][]int{},
		// org billing budgets
		OrgBudgets: map[string]map[string]*OrgBudget{},
		// API insights
		NextAPIRequestID: 1,
		// fine-grained personal access token administration
		OrgPATGrantRequests: map[string]map[int]*OrgPATGrantRequest{},
		OrgPATGrants:        map[string]map[int]*OrgPATGrant{},
		NextPATRequestID:    1,
		NextPATGrantID:      1,
		NextPATTokenID:      1,
		// org codespaces access settings
		OrgCodespacesAccess: map[string]*OrgCodespacesAccess{},
		// Dependabot repository access default level
		DependabotRepoAccessDefaultLevel: map[string]string{},
		// secret scanning pattern configurations + push protection
		SecretScanningPatternConfigs:   map[string]*OrgSecretScanningPatternConfig{},
		SecretScanningPushPlaceholders: map[string]map[string]*SecretScanningPushProtectionPlaceholder{},
		SecretScanningPushBypasses:     map[string][]*SecretScanningPushProtectionBypass{},
		// repo-write surfaces
		PagesDeployments:         map[int]map[int]*PagesDeploymentRecord{},
		NextPagesDeploymentID:    1,
		EnvBranchPolicies:        map[int][]*DeploymentBranchPolicyRule{},
		NextEnvBranchPolicyID:    1,
		EnvProtectionRules:       map[int][]*EnvCustomProtectionRule{},
		NextEnvProtectionRuleID:  1,
		SubIssueLists:            map[int][]int{},
		SubIssueParent:           map[int]int{},
		IssueBlockedBy:           map[int][]int{},
		RepoImports:              map[int]*RepoImport{},
		DependencySnapshots:      map[int][]*DependencySnapshot{},
		NextDependencySnapshotID: 1,
		SBOMExports:              map[string]*SBOMExport{},
		// GitHub Classroom
		Classrooms:                   map[int]*Classroom{},
		ClassroomAssignments:         map[int]*ClassroomAssignment{},
		ClassroomAcceptedAssignments: map[int]*ClassroomAcceptedAssignment{},
		NextClassroomID:              1,
		NextClassroomAssignmentID:    1,
		NextClassroomAcceptedID:      1,

		// repo-reads
		RepoActivities:   make(map[int]*RepoActivity),
		NextRepoActivity: 1,
		RepoCloneTraffic: make(map[string]*RepoTrafficBucket),
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
// The loadBucket registrations below are the authoritative durable-state
// inventory.
//
// Other state (sessions, agents, ephemeral codes) deliberately stays
// in-memory only.
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
	if err := st.loadBucket("gists", func(raw []byte) error {
		var g Gist
		if err := loadJSON(raw, &g); err != nil {
			return err
		}
		if st.Users[g.OwnerID] == nil {
			return fmt.Errorf("gist %s: owner id %d not found in loaded users", g.ID, g.OwnerID)
		}
		if g.Files == nil {
			g.Files = map[string]*GistFile{}
		}
		if g.History == nil {
			g.History = []*GistHistory{}
		}
		st.Gists[g.ID] = &g
		if n, err := strconv.Atoi(strings.TrimPrefix(g.NodeID, "G_kwDOB")); err == nil && n >= st.NextGistID {
			st.NextGistID = n + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("gist_comments", func(raw []byte) error {
		var c GistComment
		if err := loadJSON(raw, &c); err != nil {
			return err
		}
		if st.Gists[c.GistID] == nil {
			return fmt.Errorf("gist comment %d: gist %s not found in loaded gists", c.ID, c.GistID)
		}
		if st.Users[c.UserID] == nil {
			return fmt.Errorf("gist comment %d: user id %d not found in loaded users", c.ID, c.UserID)
		}
		st.GistComments[c.ID] = &c
		if c.ID >= st.NextGistCommentID {
			st.NextGistCommentID = c.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("starred_gists", func(raw []byte) error {
		var row persistedStarredGists
		if err := loadJSON(raw, &row); err != nil {
			return err
		}
		if st.Users[row.UserID] == nil {
			return fmt.Errorf("starred_gists user id %d not found in loaded users", row.UserID)
		}
		for gistID, starred := range row.Stars {
			if !starred {
				delete(row.Stars, gistID)
				continue
			}
			if st.Gists[gistID] == nil {
				return fmt.Errorf("starred_gists user %d: gist %s not found in loaded gists", row.UserID, gistID)
			}
		}
		if len(row.Stars) > 0 {
			st.StarredGists[row.UserID] = row.Stars
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("apps", func(raw []byte) error {
		var a App
		if err := loadJSON(raw, &a); err != nil {
			return err
		}
		if st.Users[a.OwnerID] == nil {
			return fmt.Errorf("app %d (%s): owner id %d not found in loaded users", a.ID, a.Slug, a.OwnerID)
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
	// Load organizations before repositories so persisted repository ownership
	// is validated against the real owner table.
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
	if err := st.loadBucket("repos", func(raw []byte) error {
		var r Repo
		if err := loadJSON(raw, &r); err != nil {
			return err
		}
		ownerLogin, _, ok := strings.Cut(r.FullName, "/")
		if !ok || ownerLogin == "" || r.Name == "" {
			return fmt.Errorf("repo %q: invalid full_name/name", r.FullName)
		}
		if r.OwnerID == 0 {
			return fmt.Errorf("repo %s: missing owner_id", r.FullName)
		}
		switch r.OwnerType {
		case "User":
			owner := st.Users[r.OwnerID]
			if owner == nil {
				return fmt.Errorf("repo %s: user owner id=%d not found in loaded users", r.FullName, r.OwnerID)
			}
			if !strings.EqualFold(ownerLogin, owner.Login) {
				return fmt.Errorf("repo %s: user owner id=%d login %q does not match full_name owner %q", r.FullName, r.OwnerID, owner.Login, ownerLogin)
			}
			r.Owner = owner
		case "Organization":
			org := st.Orgs[r.OwnerID]
			if org == nil {
				return fmt.Errorf("repo %s: organization owner id=%d not found in loaded organizations", r.FullName, r.OwnerID)
			}
			if !strings.EqualFold(ownerLogin, org.Login) {
				return fmt.Errorf("repo %s: organization owner id=%d login %q does not match full_name owner %q", r.FullName, r.OwnerID, org.Login, ownerLogin)
			}
			r.Owner = nil
		default:
			return fmt.Errorf("repo %s: invalid owner_type %q", r.FullName, r.OwnerType)
		}
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
	// Load teams and memberships.
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
		st.indexIssueLocked(&i)
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
		st.CommentCounts[commentCountKey(c.ParentType, c.IssueID)]++
		if c.ID >= st.NextComment {
			st.NextComment = c.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("issue_events", func(raw []byte) error {
		var e IssueEvent
		if err := loadJSON(raw, &e); err != nil {
			return err
		}
		st.IssueEvents[e.ID] = &e
		if e.ID >= st.NextIssueEventID {
			st.NextIssueEventID = e.ID + 1
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
		st.indexPullLocked(&pr)
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
			for _, cs := range statuses {
				if cs.ID >= st.CommitStatuses.nextID {
					st.CommitStatuses.nextID = cs.ID + 1
				}
			}
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
		{"repo_deploy_keys", func(key string, raw []byte) error {
			var keys map[int]*RepoDeployKey
			if err := loadJSON(raw, &keys); err != nil {
				return err
			}
			for _, k := range keys {
				if k.ID >= st.NextDeployKeyID {
					st.NextDeployKeyID = k.ID + 1
				}
			}
			st.RepoDeployKeys[key] = keys
			return nil
		}},
		{"repo_subscriptions", func(key string, raw []byte) error {
			var sub RepoSubscription
			if err := loadJSON(raw, &sub); err != nil {
				return err
			}
			st.RepoSubscriptions[key] = &sub
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
		{"workflows", func(_ string, raw []byte) error {
			var wf Workflow
			if err := loadJSON(raw, &wf); err != nil {
				return err
			}
			normalizeReloadedWorkflow(&wf)
			st.Workflows[wf.ID] = &wf
			if wf.RunID >= st.NextRunID {
				st.NextRunID = wf.RunID + 1
			}
			if st.persist != nil {
				st.persist.MustPut("workflows", wf.ID, &wf)
			}
			return nil
		}},
		{"workflow_attempts", func(key string, raw []byte) error {
			runID, err := strconv.Atoi(key)
			if err != nil {
				return fmt.Errorf("workflow_attempts key %q: %w", key, err)
			}
			var attempts []*Workflow
			if err := loadJSON(raw, &attempts); err != nil {
				return err
			}
			for _, wf := range attempts {
				normalizeReloadedWorkflow(wf)
				if wf.RunID >= st.NextRunID {
					st.NextRunID = wf.RunID + 1
				}
			}
			st.WorkflowAttempts[runID] = attempts
			if st.persist != nil {
				st.persist.MustPut("workflow_attempts", key, attempts)
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
			st.PRReviewsByPR[r.PRID] = append(st.PRReviewsByPR[r.PRID], &r)
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
		{"release_assets", func(_ string, raw []byte) error {
			var a ReleaseAsset
			if err := loadJSON(raw, &a); err != nil {
				return err
			}
			st.Releases.assetByID[a.ID] = &a
			if rel := st.Releases.byID[a.ReleaseID]; rel != nil {
				rel.Assets = append(rel.Assets, &a)
			}
			if a.ID >= st.Releases.nextAsset {
				st.Releases.nextAsset = a.ID + 1
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
			if it.ContentID != 0 {
				st.ProjectsV2.itemsByOwner[it.ContentID] = append(st.ProjectsV2.itemsByOwner[it.ContentID], &it)
			}
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
			case "blocked_users":
				var blocked map[int]map[int]bool
				if err := loadJSON(raw, &blocked); err != nil {
					return err
				}
				st.Misc.blockedUsers = blocked
			case "social_accounts":
				var accounts map[int][]map[string]interface{}
				if err := loadJSON(raw, &accounts); err != nil {
					return err
				}
				st.Misc.socialAccounts = accounts
			case "ssh_signing_keys":
				var keys map[int][]map[string]interface{}
				if err := loadJSON(raw, &keys); err != nil {
					return err
				}
				st.Misc.sshSigningKeys = keys
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
			for _, b := range builds {
				if b.ID == 0 {
					b.ID = pagesBuildIDFromURL(b.URL)
				}
				if b.ID >= st.Misc.nextPagesBuildID {
					st.Misc.nextPagesBuildID = b.ID + 1
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
			if p.ID >= st.Misc.nextMarketplacePlanID {
				st.Misc.nextMarketplacePlanID = p.ID + 1
			}
			return nil
		}},
		{"marketplace_listings", func(_ string, raw []byte) error {
			var listing MarketplaceListing
			if err := loadJSON(raw, &listing); err != nil {
				return err
			}
			st.Misc.marketplaceListings[strings.ToLower(listing.Slug)] = &listing
			if listing.WebhookID >= st.NextHookID {
				st.NextHookID = listing.WebhookID + 1
			}
			return nil
		}},
		{"marketplace_deliveries", func(key string, raw []byte) error {
			var deliveries []*WebhookDelivery
			if err := loadJSON(raw, &deliveries); err != nil {
				return err
			}
			st.Misc.marketplaceDeliveries[strings.ToLower(key)] = deliveries
			for _, delivery := range deliveries {
				if delivery.ID >= st.Misc.nextMarketplaceDeliveryID {
					st.Misc.nextMarketplaceDeliveryID = delivery.ID + 1
				}
			}
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
		{"code_scanning_default_setups", func(_ string, raw []byte) error {
			var setup CodeScanningDefaultSetup
			if err := loadJSON(raw, &setup); err != nil {
				return err
			}
			st.CodeScanningDefaultSetups[setup.RepoKey] = &setup
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
		{"dependabot_user_secrets", func(key string, raw []byte) error {
			var m map[string]*DependabotUserSecret
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.DependabotUserSecrets[key] = m
			return nil
		}},
		{"dependabot_repo_access", func(key string, raw []byte) error {
			var ids []int
			if err := loadJSON(raw, &ids); err != nil {
				return err
			}
			st.DependabotRepositoryAccess[key] = ids
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
		{"org_actions_permissions", func(key string, raw []byte) error {
			var p OrgActionsPermissions
			if err := loadJSON(raw, &p); err != nil {
				return err
			}
			st.OrgActionsPermissions[key] = &p
			return nil
		}},
		{"repo_actions_permissions", func(key string, raw []byte) error {
			var p RepoActionsPermissions
			if err := loadJSON(raw, &p); err != nil {
				return err
			}
			st.RepoActionsPermissions[key] = &p
			return nil
		}},
		{"packages", func(_ string, raw []byte) error {
			var p Package
			if err := loadJSON(raw, &p); err != nil {
				return err
			}
			st.Packages[p.ID] = &p
			// Soft-deleted packages stay out of the by-owner key map so
			// lists/gets skip them while restore can still find them.
			if !p.Deleted {
				if st.PackagesByOwnerKey[p.OwnerKey] == nil {
					st.PackagesByOwnerKey[p.OwnerKey] = map[string]*Package{}
				}
				st.PackagesByOwnerKey[p.OwnerKey][packageKey(p.PackageType, p.Name)] = &p
			}
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
		{"security_advisories", func(_ string, raw []byte) error {
			var a SecurityAdvisory
			if err := loadJSON(raw, &a); err != nil {
				return err
			}
			st.SecurityAdvisories[a.ID] = &a
			if repo := st.Repos[a.RepoID]; repo != nil {
				if st.SecurityAdvisoriesByRepo[repo.FullName] == nil {
					st.SecurityAdvisoriesByRepo[repo.FullName] = map[string]*SecurityAdvisory{}
				}
				st.SecurityAdvisoriesByRepo[repo.FullName][a.GHSAID] = &a
			}
			if a.ID >= st.NextSecurityAdvisoryID {
				st.NextSecurityAdvisoryID = a.ID + 1
			}
			return nil
		}},
		{"security_advisory_reports", func(_ string, raw []byte) error {
			var r SecurityAdvisoryReport
			if err := loadJSON(raw, &r); err != nil {
				return err
			}
			st.SecurityAdvisoryReports[r.ID] = &r
			if r.ID >= st.NextSecurityAdvisoryReportID {
				st.NextSecurityAdvisoryReportID = r.ID + 1
			}
			return nil
		}},
		// org billing budgets
		{"org_budgets", func(key string, raw []byte) error {
			var m map[string]*OrgBudget
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.OrgBudgets[key] = m
			return nil
		}},
		// API insights
		{"api_insights_requests", func(_ string, raw []byte) error {
			var rec APIRequestRecord
			if err := loadJSON(raw, &rec); err != nil {
				return err
			}
			st.APIRequestRecords = append(st.APIRequestRecords, &rec)
			if rec.ID >= st.NextAPIRequestID {
				st.NextAPIRequestID = rec.ID + 1
			}
			return nil
		}},
		// fine-grained personal access token administration
		{"org_pat_grant_requests", func(key string, raw []byte) error {
			var m map[int]*OrgPATGrantRequest
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.OrgPATGrantRequests[key] = m
			for _, req := range m {
				if req.ID >= st.NextPATRequestID {
					st.NextPATRequestID = req.ID + 1
				}
				if req.TokenID >= st.NextPATTokenID {
					st.NextPATTokenID = req.TokenID + 1
				}
			}
			return nil
		}},
		{"org_pat_grants", func(key string, raw []byte) error {
			var m map[int]*OrgPATGrant
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.OrgPATGrants[key] = m
			for _, g := range m {
				if g.ID >= st.NextPATGrantID {
					st.NextPATGrantID = g.ID + 1
				}
				if g.TokenID >= st.NextPATTokenID {
					st.NextPATTokenID = g.TokenID + 1
				}
			}
			return nil
		}},
		// org codespaces access settings
		{"org_codespaces_access", func(key string, raw []byte) error {
			var a OrgCodespacesAccess
			if err := loadJSON(raw, &a); err != nil {
				return err
			}
			st.OrgCodespacesAccess[key] = &a
			return nil
		}},
		// Dependabot repository access default level
		{"dependabot_repo_access_default_level", func(key string, raw []byte) error {
			var level string
			if err := loadJSON(raw, &level); err != nil {
				return err
			}
			st.DependabotRepoAccessDefaultLevel[key] = level
			return nil
		}},
		// secret scanning pattern configurations + push protection
		{"secret_scanning_pattern_configs", func(key string, raw []byte) error {
			var cfg OrgSecretScanningPatternConfig
			if err := loadJSON(raw, &cfg); err != nil {
				return err
			}
			st.SecretScanningPatternConfigs[key] = &cfg
			return nil
		}},
		{"secret_scanning_push_placeholders", func(key string, raw []byte) error {
			var m map[string]*SecretScanningPushProtectionPlaceholder
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.SecretScanningPushPlaceholders[key] = m
			return nil
		}},
		{"secret_scanning_push_bypasses", func(key string, raw []byte) error {
			var list []*SecretScanningPushProtectionBypass
			if err := loadJSON(raw, &list); err != nil {
				return err
			}
			st.SecretScanningPushBypasses[key] = list
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

	// API request records arrive in map-iteration order; the in-memory log
	// is oldest-first (RecordAPIRequest appends), so sort by ID ascending.
	sort.Slice(st.APIRequestRecords, func(i, j int) bool { return st.APIRequestRecords[i].ID < st.APIRequestRecords[j].ID })

	if v, err := st.persist.GetCounter("next_run_id"); err != nil {
		return fmt.Errorf("load counter next_run_id: %w", err)
	} else if int(v) > st.NextRunID {
		st.NextRunID = int(v)
	}

	// enterprises
	if err := st.loadBucket("enterprise_teams", func(raw []byte) error {
		var t EnterpriseTeam
		if err := loadJSON(raw, &t); err != nil {
			return err
		}
		st.EnterpriseTeams[t.ID] = &t
		st.EnterpriseTeamsBySlug[t.Slug] = &t
		if t.ID >= st.NextEnterpriseTeamID {
			st.NextEnterpriseTeamID = t.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	// teams-people
	if err := st.loadBucket("org_invitations", func(raw []byte) error {
		var inv OrgInvitation
		if err := loadJSON(raw, &inv); err != nil {
			return err
		}
		st.OrgInvitations[inv.ID] = &inv
		if inv.ID >= st.NextOrgInvitationID {
			st.NextOrgInvitationID = inv.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	// hosted-runners
	if err := st.loadBucket("hosted_runners", func(raw []byte) error {
		var hr HostedRunner
		if err := loadJSON(raw, &hr); err != nil {
			return err
		}
		st.HostedRunners[hr.ID] = &hr
		if hr.ID >= st.NextHostedRunnerID {
			st.NextHostedRunnerID = hr.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("enterprise_code_security_configs", func(raw []byte) error {
		var c EnterpriseCodeSecurityConfiguration
		if err := loadJSON(raw, &c); err != nil {
			return err
		}
		st.EnterpriseCodeSecurityConfigs[c.ID] = &c
		if c.ID >= st.NextEnterpriseCodeSecurityConfigID {
			st.NextEnterpriseCodeSecurityConfigID = c.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("hosted_runner_custom_images", func(raw []byte) error {
		var img HostedRunnerCustomImage
		if err := loadJSON(raw, &img); err != nil {
			return err
		}
		st.HostedRunnerCustomImages[img.ID] = &img
		if img.ID >= st.NextHostedRunnerImageID {
			st.NextHostedRunnerImageID = img.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("enterprise_code_security_attachments", func(raw []byte) error {
		var a EnterpriseCodeSecurityAttachment
		if err := loadJSON(raw, &a); err != nil {
			return err
		}
		st.EnterpriseCodeSecurityRepoConfigs[a.RepoID] = a.ConfigID
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("enterprise_settings", func(raw []byte) error {
		var s EnterpriseSettings
		if err := loadJSON(raw, &s); err != nil {
			return err
		}
		st.EnterpriseSettings = &s
		return nil
	}); err != nil {
		return err
	}

	// projects-v2 views
	if err := st.loadBucket("project_v2_views", func(raw []byte) error {
		var v ProjectV2View
		if err := loadJSON(raw, &v); err != nil {
			return err
		}
		st.ProjectsV2.views[v.ID] = &v
		st.ProjectsV2.viewsByProj[v.ProjectID] = append(st.ProjectsV2.viewsByProj[v.ProjectID], &v)
		if v.ID >= st.ProjectsV2.nextViewID {
			st.ProjectsV2.nextViewID = v.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	// Rows arrive in map-iteration order; restore per-project creation
	// (ID) order for fields, views, and per-content item slices. Iteration
	// IDs share the option-seed space with single-select option IDs, so
	// resume the seed past them too.
	for _, views := range st.ProjectsV2.viewsByProj {
		sort.Slice(views, func(i, j int) bool { return views[i].ID < views[j].ID })
	}
	for _, fields := range st.ProjectsV2.fieldsByProj {
		sort.Slice(fields, func(i, j int) bool { return fields[i].ID < fields[j].ID })
	}
	for _, items := range st.ProjectsV2.itemsByOwner {
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	}
	for _, f := range st.ProjectsV2.fields {
		if f.Iteration == nil {
			continue
		}
		for _, iter := range f.Iteration.Iterations {
			if n, err := strconv.ParseInt(iter.ID, 16, 64); err == nil && int(n) >= st.ProjectsV2.nextOptionSeed {
				st.ProjectsV2.nextOptionSeed = int(n) + 1
			}
		}
	}

	// org governance surfaces
	// agents-codescan: GitHub Copilot coding agent secrets/variables/tasks
	// and CodeQL databases/variant analyses.
	for _, loadFn := range []struct {
		name string
		fn   func(key string, raw []byte) error
	}{
		{"code_security_configurations", func(key string, raw []byte) error {
			var m map[int]*CodeSecurityConfiguration
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.CodeSecurityConfigs[key] = m
			for id := range m {
				if id >= st.NextCodeSecurityConfigID {
					st.NextCodeSecurityConfigID = id + 1
				}
			}
			return nil
		}},
		{"code_security_repo_attachments", func(key string, raw []byte) error {
			var m map[int]int
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.CodeSecurityRepoAttachments[key] = m
			return nil
		}},
		{"org_custom_properties", func(key string, raw []byte) error {
			var m map[string]*CustomProperty
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.OrgCustomProperties[key] = m
			return nil
		}},
		{"repo_custom_property_values", func(key string, raw []byte) error {
			var m map[string]interface{}
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.RepoCustomPropertyValues[key] = m
			return nil
		}},
		{"org_issue_types", func(key string, raw []byte) error {
			var m map[int]*IssueType
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.OrgIssueTypes[key] = m
			for id := range m {
				if id >= st.NextIssueTypeID {
					st.NextIssueTypeID = id + 1
				}
			}
			return nil
		}},
		{"org_issue_fields", func(key string, raw []byte) error {
			var m map[int]*IssueField
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.OrgIssueFields[key] = m
			for id, f := range m {
				if id >= st.NextIssueFieldID {
					st.NextIssueFieldID = id + 1
				}
				for _, opt := range f.Options {
					if opt.ID >= st.NextIssueFieldOptionID {
						st.NextIssueFieldOptionID = opt.ID + 1
					}
				}
			}
			return nil
		}},
		{"issue_field_values", func(key string, raw []byte) error {
			issueID, err := strconv.Atoi(key)
			if err != nil {
				return fmt.Errorf("issue_field_values key %q: %w", key, err)
			}
			var m map[int]interface{}
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.IssueFieldValues[issueID] = m
			return nil
		}},
		{"org_campaigns", func(key string, raw []byte) error {
			var m map[int]*Campaign
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.OrgCampaigns[key] = m
			return nil
		}},
		{"org_private_registries", func(key string, raw []byte) error {
			var m map[string]*PrivateRegistryConfiguration
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.OrgPrivateRegistries[key] = m
			return nil
		}},
		{"org_network_configurations", func(key string, raw []byte) error {
			var m map[string]*NetworkConfiguration
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.OrgNetworkConfigurations[key] = m
			return nil
		}},
		{"org_network_settings", func(key string, raw []byte) error {
			var m map[string]*NetworkSettingsResource
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.OrgNetworkSettings[key] = m
			return nil
		}},
		{"org_immutable_releases", func(key string, raw []byte) error {
			var s OrgImmutableReleasesSettings
			if err := loadJSON(raw, &s); err != nil {
				return err
			}
			st.OrgImmutableReleases[key] = &s
			return nil
		}},
		{"repo_immutable_releases", func(key string, raw []byte) error {
			var enabled bool
			if err := loadJSON(raw, &enabled); err != nil {
				return err
			}
			st.RepoImmutableReleases[key] = enabled
			return nil
		}},
		{"agents_repo_secrets", func(key string, raw []byte) error {
			var m map[string]*Secret
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.AgentsRepoSecrets[key] = m
			return nil
		}},
		{"agents_org_secrets", func(key string, raw []byte) error {
			var m map[string]*OrgSecret
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.AgentsOrgSecrets[key] = m
			return nil
		}},
		{"agents_repo_variables", func(key string, raw []byte) error {
			var m map[string]*ActionsVariable
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.AgentsRepoVariables[key] = m
			return nil
		}},
		{"agents_org_variables", func(key string, raw []byte) error {
			var m map[string]*ActionsVariable
			if err := loadJSON(raw, &m); err != nil {
				return err
			}
			st.AgentsOrgVariables[key] = m
			return nil
		}},
		{"agent_tasks", func(key string, raw []byte) error {
			var task AgentTask
			if err := loadJSON(raw, &task); err != nil {
				return err
			}
			st.AgentTasks[key] = &task
			return nil
		}},
		{"code_scanning_autofixes", func(key string, raw []byte) error {
			var a CodeScanningAutofix
			if err := loadJSON(raw, &a); err != nil {
				return err
			}
			st.CodeScanningAutofixes[key] = &a
			return nil
		}},
		{"codeql_databases", func(_ string, raw []byte) error {
			var db CodeQLDatabase
			if err := loadJSON(raw, &db); err != nil {
				return err
			}
			st.CodeQLDatabases[db.ID] = &db
			if st.CodeQLDatabasesByRepo[db.RepoKey] == nil {
				st.CodeQLDatabasesByRepo[db.RepoKey] = make(map[string]*CodeQLDatabase)
			}
			st.CodeQLDatabasesByRepo[db.RepoKey][db.Language] = &db
			if db.ID >= st.NextCodeQLDatabaseID {
				st.NextCodeQLDatabaseID = db.ID + 1
			}
			return nil
		}},
		{"codeql_variant_analyses", func(_ string, raw []byte) error {
			var va CodeQLVariantAnalysis
			if err := loadJSON(raw, &va); err != nil {
				return err
			}
			st.CodeQLVariantAnalyses[va.ID] = &va
			if va.ID >= st.NextCodeQLVariantAnalysisID {
				st.NextCodeQLVariantAnalysisID = va.ID + 1
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

	// attestations
	if err := st.loadBucket("attestations", func(raw []byte) error {
		var a Attestation
		if err := loadJSON(raw, &a); err != nil {
			return err
		}
		st.Attestations[a.ID] = &a
		if a.ID >= st.NextAttestationID {
			st.NextAttestationID = a.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}

	// org artifact metadata records
	if err := st.loadBucket("artifact_storage_records", func(raw []byte) error {
		var rec ArtifactStorageRecord
		if err := loadJSON(raw, &rec); err != nil {
			return err
		}
		st.ArtifactStorageRecords[rec.ID] = &rec
		if rec.ID >= st.NextArtifactStorageRecordID {
			st.NextArtifactStorageRecordID = rec.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("artifact_deployment_records", func(raw []byte) error {
		var rec ArtifactDeploymentRecord
		if err := loadJSON(raw, &rec); err != nil {
			return err
		}
		st.ArtifactDeploymentRecords[rec.ID] = &rec
		if rec.ID >= st.NextArtifactDeploymentRecordID {
			st.NextArtifactDeploymentRecordID = rec.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}

	// copilot + code quality
	if err := st.loadBucket("copilot_seats", func(raw []byte) error {
		var seat CopilotSeat
		if err := loadJSON(raw, &seat); err != nil {
			return err
		}
		if st.CopilotSeats[seat.OrgLogin] == nil {
			st.CopilotSeats[seat.OrgLogin] = map[int]*CopilotSeat{}
		}
		st.CopilotSeats[seat.OrgLogin][seat.UserID] = &seat
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("copilot_content_exclusions", func(raw []byte) error {
		var ce CopilotContentExclusion
		if err := loadJSON(raw, &ce); err != nil {
			return err
		}
		st.CopilotContentExclusions[ce.OrgLogin] = &ce
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("copilot_coding_agent_permissions", func(raw []byte) error {
		var p CopilotCodingAgentPermissions
		if err := loadJSON(raw, &p); err != nil {
			return err
		}
		st.CopilotCodingAgentPerms[p.OrgLogin] = &p
		return nil
	}); err != nil {
		return err
	}
	// User-surface: GitHub Marketplace subscriptions are independent for
	// every listing and purchasing account.
	if err := st.loadBucket("marketplace_purchases", func(raw []byte) error {
		var p MarketplacePurchase
		if err := loadJSON(raw, &p); err != nil {
			return err
		}
		if p.ListingSlug == "" || p.AccountType == "" {
			return fmt.Errorf("marketplace purchase is missing listing or account identity")
		}
		st.Misc.marketplacePurchases[marketplacePurchaseKey(p.ListingSlug, p.AccountType, p.AccountID)] = &p
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("copilot_spaces", func(raw []byte) error {
		var space CopilotSpace
		if err := loadJSON(raw, &space); err != nil {
			return err
		}
		st.CopilotSpaces[space.ID] = &space
		if space.ID >= st.NextCopilotSpaceID {
			st.NextCopilotSpaceID = space.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("code_quality_setups", func(raw []byte) error {
		var setup CodeQualitySetup
		if err := loadJSON(raw, &setup); err != nil {
			return err
		}
		st.CodeQualitySetups[setup.RepoFullName] = &setup
		return nil
	}); err != nil {
		return err
	}
	// actions-oidc-properties (keyed by org login, so List directly)
	if rows, err := st.persist.List("org_oidc_property_inclusions"); err != nil {
		return fmt.Errorf("load org_oidc_property_inclusions: %w", err)
	} else {
		for org, raw := range rows {
			var names []string
			if err := loadJSON(raw, &names); err != nil {
				return fmt.Errorf("decode org_oidc_property_inclusions row: %w", err)
			}
			st.OrgOIDCPropertyInclusions[org] = names
		}
	}
	if rows, err := st.persist.List("org_blocks"); err != nil {
		return fmt.Errorf("load org_blocks: %w", err)
	} else {
		for orgLogin, raw := range rows {
			blocks := map[int]time.Time{}
			if err := loadJSON(raw, &blocks); err != nil {
				return fmt.Errorf("decode org_blocks row: %w", err)
			}
			st.OrgBlocks[orgLogin] = blocks
		}
	}
	if rows, err := st.persist.List("org_interaction_limits"); err != nil {
		return fmt.Errorf("load org_interaction_limits: %w", err)
	} else {
		for orgLogin, raw := range rows {
			var lim OrgInteractionLimit
			if err := loadJSON(raw, &lim); err != nil {
				return fmt.Errorf("decode org_interaction_limits row: %w", err)
			}
			st.OrgInteractionLimits[orgLogin] = &lim
		}
	}
	for bucket, dst := range map[string]map[string]map[int][]int{
		"org_role_team_assignments": st.OrgRoleTeamAssignments,
		"org_role_user_assignments": st.OrgRoleUserAssignments,
	} {
		rows, err := st.persist.List(bucket)
		if err != nil {
			return fmt.Errorf("load %s: %w", bucket, err)
		}
		for orgLogin, raw := range rows {
			assignments := map[int][]int{}
			if err := loadJSON(raw, &assignments); err != nil {
				return fmt.Errorf("decode %s row: %w", bucket, err)
			}
			dst[orgLogin] = assignments
		}
	}

	// repo-write surfaces
	for _, loadFn := range []struct {
		name string
		fn   func(string, []byte) error
	}{
		{"pages_deployments", func(key string, raw []byte) error {
			repoID, err := strconv.Atoi(key)
			if err != nil {
				return fmt.Errorf("pages_deployments key %q: %w", key, err)
			}
			var byID map[int]*PagesDeploymentRecord
			if err := loadJSON(raw, &byID); err != nil {
				return err
			}
			st.PagesDeployments[repoID] = byID
			for id := range byID {
				if id >= st.NextPagesDeploymentID {
					st.NextPagesDeploymentID = id + 1
				}
			}
			return nil
		}},
		{"env_branch_policies", func(key string, raw []byte) error {
			envID, err := strconv.Atoi(key)
			if err != nil {
				return fmt.Errorf("env_branch_policies key %q: %w", key, err)
			}
			var policies []*DeploymentBranchPolicyRule
			if err := loadJSON(raw, &policies); err != nil {
				return err
			}
			st.EnvBranchPolicies[envID] = policies
			for _, p := range policies {
				if p.ID >= st.NextEnvBranchPolicyID {
					st.NextEnvBranchPolicyID = p.ID + 1
				}
			}
			return nil
		}},
		{"env_protection_rules", func(key string, raw []byte) error {
			envID, err := strconv.Atoi(key)
			if err != nil {
				return fmt.Errorf("env_protection_rules key %q: %w", key, err)
			}
			var rules []*EnvCustomProtectionRule
			if err := loadJSON(raw, &rules); err != nil {
				return err
			}
			st.EnvProtectionRules[envID] = rules
			for _, rule := range rules {
				if rule.ID >= st.NextEnvProtectionRuleID {
					st.NextEnvProtectionRuleID = rule.ID + 1
				}
			}
			return nil
		}},
		{"sub_issues", func(key string, raw []byte) error {
			parentID, err := strconv.Atoi(key)
			if err != nil {
				return fmt.Errorf("sub_issues key %q: %w", key, err)
			}
			var children []int
			if err := loadJSON(raw, &children); err != nil {
				return err
			}
			st.SubIssueLists[parentID] = children
			for _, childID := range children {
				st.SubIssueParent[childID] = parentID
			}
			return nil
		}},
		{"issue_blocked_by", func(key string, raw []byte) error {
			issueID, err := strconv.Atoi(key)
			if err != nil {
				return fmt.Errorf("issue_blocked_by key %q: %w", key, err)
			}
			var blockers []int
			if err := loadJSON(raw, &blockers); err != nil {
				return err
			}
			st.IssueBlockedBy[issueID] = blockers
			return nil
		}},
		{"repo_imports", func(key string, raw []byte) error {
			repoID, err := strconv.Atoi(key)
			if err != nil {
				return fmt.Errorf("repo_imports key %q: %w", key, err)
			}
			var imp RepoImport
			if err := loadJSON(raw, &imp); err != nil {
				return err
			}
			st.RepoImports[repoID] = &imp
			return nil
		}},
		{"dependency_snapshots", func(key string, raw []byte) error {
			repoID, err := strconv.Atoi(key)
			if err != nil {
				return fmt.Errorf("dependency_snapshots key %q: %w", key, err)
			}
			var snapshots []*DependencySnapshot
			if err := loadJSON(raw, &snapshots); err != nil {
				return err
			}
			st.DependencySnapshots[repoID] = snapshots
			for _, snap := range snapshots {
				if snap.ID >= st.NextDependencySnapshotID {
					st.NextDependencySnapshotID = snap.ID + 1
				}
			}
			return nil
		}},
		{"sbom_exports", func(key string, raw []byte) error {
			var exp SBOMExport
			if err := loadJSON(raw, &exp); err != nil {
				return err
			}
			st.SBOMExports[key] = &exp
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

	// GitHub Classroom
	if err := st.loadBucket("classrooms", func(raw []byte) error {
		var c Classroom
		if err := loadJSON(raw, &c); err != nil {
			return err
		}
		st.Classrooms[c.ID] = &c
		if c.ID >= st.NextClassroomID {
			st.NextClassroomID = c.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("classroom_assignments", func(raw []byte) error {
		var a ClassroomAssignment
		if err := loadJSON(raw, &a); err != nil {
			return err
		}
		st.ClassroomAssignments[a.ID] = &a
		if a.ID >= st.NextClassroomAssignmentID {
			st.NextClassroomAssignmentID = a.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("classroom_accepted_assignments", func(raw []byte) error {
		var a ClassroomAcceptedAssignment
		if err := loadJSON(raw, &a); err != nil {
			return err
		}
		st.ClassroomAcceptedAssignments[a.ID] = &a
		if a.ID >= st.NextClassroomAcceptedID {
			st.NextClassroomAcceptedID = a.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	// repo-reads
	if err := st.loadBucket("repo_activity", func(raw []byte) error {
		var a RepoActivity
		if err := loadJSON(raw, &a); err != nil {
			return err
		}
		st.RepoActivities[a.ID] = &a
		if a.ID >= st.NextRepoActivity {
			st.NextRepoActivity = a.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	if err := st.loadBucket("repo_traffic_clones", func(raw []byte) error {
		var b RepoTrafficBucket
		if err := loadJSON(raw, &b); err != nil {
			return err
		}
		st.RepoCloneTraffic[repoTrafficKey(b.RepoID, b.Day)] = &b
		return nil
	}); err != nil {
		return err
	}

	// codespaces
	if err := st.loadBucket("codespaces", func(raw []byte) error {
		var cs Codespace
		if err := loadJSON(raw, &cs); err != nil {
			return err
		}
		st.Codespaces[cs.ID] = &cs
		st.CodespacesByName[cs.Name] = &cs
		if cs.ID >= st.NextCodespaceID {
			st.NextCodespaceID = cs.ID + 1
		}
		return nil
	}); err != nil {
		return err
	}
	codespaceSecretRows, err := st.persist.List("codespace_secrets")
	if err != nil {
		return fmt.Errorf("load codespace_secrets: %w", err)
	}
	for scope, raw := range codespaceSecretRows {
		var m map[string]*CodespaceSecret
		if err := loadJSON(raw, &m); err != nil {
			return fmt.Errorf("decode codespace_secrets row: %w", err)
		}
		st.CodespaceSecrets[scope] = m
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
	if t.ExpiresAt != nil && !t.ExpiresAt.After(time.Now()) {
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

// LookupUserBySSHKey resolves a registered account SSH authentication key.
// It compares parsed SSH wire encodings so comments and spacing differences in
// authorized-key text cannot create a different credential identity.
func (st *Store) LookupUserBySSHKey(key ssh.PublicKey) *User {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	for userID, keys := range st.Misc.keysByUser {
		for _, registered := range keys {
			parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(registered.Key))
			if err == nil && bytes.Equal(parsed.Marshal(), key.Marshal()) {
				return st.GetUserByID(userID)
			}
		}
	}
	return nil
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
	for _, issue := range st.IssuesByRepo[repoID] {
		if issue.State == "OPEN" {
			n++
		}
	}
	for _, pr := range st.PullsByRepo[repoID] {
		if pr.State == "OPEN" {
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
func generateTokenValue() (string, error) {
	h, err := randomHex(20)
	if err != nil {
		return "", fmt.Errorf("generate personal access token: %w", err)
	}
	return fmt.Sprintf("ghp_%s", h), nil
}

// generateGistID creates a random 20-character hexadecimal gist ID.
func generateGistID() (string, error) {
	h, err := randomHex(10)
	if err != nil {
		return "", fmt.Errorf("generate gist id: %w", err)
	}
	return h, nil
}

func (st *Store) persistGistLocked(g *Gist) {
	if st.persist != nil {
		st.persist.MustPut("gists", g.ID, g)
	}
}

func (st *Store) deleteGistPersistenceLocked(id string) {
	if st.persist != nil {
		st.persist.MustDelete("gists", id)
	}
}

func (st *Store) persistGistCommentLocked(c *GistComment) {
	if st.persist != nil {
		st.persist.MustPut("gist_comments", strconv.Itoa(c.ID), c)
	}
}

func (st *Store) deleteGistCommentPersistenceLocked(id int) {
	if st.persist != nil {
		st.persist.MustDelete("gist_comments", strconv.Itoa(id))
	}
}

func (st *Store) persistStarredGistsLocked(userID int) {
	if st.persist == nil {
		return
	}
	key := strconv.Itoa(userID)
	if stars := st.StarredGists[userID]; len(stars) > 0 {
		st.persist.MustPut("starred_gists", key, persistedStarredGists{UserID: userID, Stars: stars})
	} else {
		st.persist.MustDelete("starred_gists", key)
	}
}

// CreateGist creates a new gist owned by the given user.
func (st *Store) CreateGist(owner *User, description string, public bool, files map[string]*GistFile) *Gist {
	g, err := st.CreateGistE(owner, description, public, files)
	if err != nil {
		panic(err)
	}
	return g
}

func (st *Store) CreateGistE(owner *User, description string, public bool, files map[string]*GistFile) (*Gist, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	id, err := generateGistID()
	if err != nil {
		return nil, err
	}
	for st.Gists[id] != nil {
		id, err = generateGistID()
		if err != nil {
			return nil, err
		}
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
		History: []*GistHistory{{
			Version:     id,
			CommittedAt: now,
			ChangeStatus: map[string]int{
				"total":     0,
				"additions": 0,
				"deletions": 0,
			},
		}},
	}
	st.Gists[id] = g
	st.NextGistID++
	st.persistGistLocked(g)
	return g, nil
}

// GetGist returns the gist with the given ID, or nil.
func (st *Store) GetGist(id string) *Gist {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Gists[id]
}

// UpdateGist replaces the gist fields and records a history entry.
func (st *Store) UpdateGist(id string, description *string, files map[string]*GistFile, deleteFiles []string) (*Gist, bool) {
	g, ok, err := st.UpdateGistE(id, description, files, deleteFiles)
	if err != nil {
		panic(err)
	}
	return g, ok
}

func (st *Store) UpdateGistE(id string, description *string, files map[string]*GistFile, deleteFiles []string) (*Gist, bool, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	g := st.Gists[id]
	if g == nil {
		return nil, false, nil
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

	version, err := generateGistID()
	if err != nil {
		return nil, false, err
	}
	g.History = append(g.History, &GistHistory{
		Version:     version,
		CommittedAt: g.UpdatedAt,
		ChangeStatus: map[string]int{
			"total":     additions + deletions,
			"additions": additions,
			"deletions": deletions,
		},
	})
	st.persistGistLocked(g)
	return g, true, nil
}

// DeleteGist deletes a gist and all its comments.
func (st *Store) DeleteGist(id string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.Gists[id] == nil {
		return false
	}
	delete(st.Gists, id)
	st.deleteGistPersistenceLocked(id)
	for cid, c := range st.GistComments {
		if c.GistID == id {
			delete(st.GistComments, cid)
			st.deleteGistCommentPersistenceLocked(cid)
		}
	}
	for uid, stars := range st.StarredGists {
		if stars[id] {
			delete(stars, id)
			if len(stars) == 0 {
				delete(st.StarredGists, uid)
			}
			st.persistStarredGistsLocked(uid)
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

// ListGistCommits returns the revision history for a gist.
func (st *Store) ListGistCommits(gistID string) []*GistHistory {
	st.mu.RLock()
	defer st.mu.RUnlock()
	g := st.Gists[gistID]
	if g == nil {
		return nil
	}
	out := make([]*GistHistory, len(g.History))
	copy(out, g.History)
	sortHistory(out)
	return out
}

// GetGistAtRevision returns the gist state at a specific revision.
func (st *Store) GetGistAtRevision(gistID, sha string) *Gist {
	st.mu.RLock()
	defer st.mu.RUnlock()
	g := st.Gists[gistID]
	if g == nil {
		return nil
	}
	for _, h := range g.History {
		if h.Version == sha {
			cp := *g
			cp.UpdatedAt = h.CommittedAt
			return &cp
		}
	}
	return nil
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
	st.persistStarredGistsLocked(userID)
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
	st.persistStarredGistsLocked(userID)
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
	fork, ok, err := st.ForkGistE(user, gistID)
	if err != nil {
		panic(err)
	}
	return fork, ok
}

func (st *Store) ForkGistE(user *User, gistID string) (*Gist, bool, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	orig := st.Gists[gistID]
	if orig == nil {
		return nil, false, nil
	}
	files := make(map[string]*GistFile, len(orig.Files))
	for name, f := range orig.Files {
		cp := *f
		files[name] = &cp
	}
	now := time.Now().UTC()
	id, err := generateGistID()
	if err != nil {
		return nil, false, err
	}
	for st.Gists[id] != nil {
		id, err = generateGistID()
		if err != nil {
			return nil, false, err
		}
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
	st.persistGistLocked(orig)
	st.persistGistLocked(fork)
	return fork, true, nil
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
	st.persistGistCommentLocked(c)
	st.persistGistLocked(g)
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
	st.persistGistCommentLocked(c)
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
		st.persistGistLocked(g)
	}
	delete(st.GistComments, id)
	st.deleteGistCommentPersistenceLocked(id)
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

// ListUsers returns all users.
func (st *Store) ListUsers() []*User {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]*User, 0, len(st.Users))
	for _, u := range st.Users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ListBlockedUsers returns the logins of users blocked by userID.
func (st *Store) ListBlockedUsers(userID int) []string {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	blocked := st.Misc.blockedUsers[userID]
	out := make([]string, 0, len(blocked))
	for id := range blocked {
		if u := st.Users[id]; u != nil {
			out = append(out, u.Login)
		}
	}
	sort.Strings(out)
	return out
}

// IsUserBlocked reports whether userID has blocked targetID.
func (st *Store) IsUserBlocked(userID, targetID int) bool {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	return st.Misc.blockedUsers[userID][targetID]
}

// BlockUser blocks targetID for userID.
func (st *Store) BlockUser(userID, targetID int) bool {
	st.Misc.mu.Lock()
	defer st.Misc.mu.Unlock()
	if st.Misc.blockedUsers[userID] == nil {
		st.Misc.blockedUsers[userID] = map[int]bool{}
	}
	st.Misc.blockedUsers[userID][targetID] = true
	if st.Misc.persist != nil {
		st.Misc.persist.MustPut("misc", "blocked_users", st.Misc.blockedUsers)
	}
	return true
}

// UnblockUser unblocks targetID for userID.
func (st *Store) UnblockUser(userID, targetID int) bool {
	st.Misc.mu.Lock()
	defer st.Misc.mu.Unlock()
	if st.Misc.blockedUsers[userID] == nil {
		return false
	}
	if !st.Misc.blockedUsers[userID][targetID] {
		return false
	}
	delete(st.Misc.blockedUsers[userID], targetID)
	if st.Misc.persist != nil {
		st.Misc.persist.MustPut("misc", "blocked_users", st.Misc.blockedUsers)
	}
	return true
}

// ListUserBlocks returns users blocked by userID.
func (st *Store) ListUserBlocks(userID int) []*User {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	var out []*User
	for targetID := range st.Misc.blockedUsers[userID] {
		if u := st.Users[targetID]; u != nil {
			out = append(out, u)
		}
	}
	return out
}

// IsUserFollowing reports whether userID follows targetID.
func (st *Store) IsUserFollowing(userID, targetID int) bool {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	user := st.Users[userID]
	target := st.Users[targetID]
	if user == nil || target == nil {
		return false
	}
	return st.Misc.follows[user.Login][target.Login]
}

// ListUserSocialAccounts returns social accounts for a user.
func (st *Store) ListUserSocialAccounts(userID int) []map[string]interface{} {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	out := make([]map[string]interface{}, len(st.Misc.socialAccounts[userID]))
	copy(out, st.Misc.socialAccounts[userID])
	return out
}

// SetUserSocialAccounts replaces a user's social accounts.
func (st *Store) SetUserSocialAccounts(userID int, accounts []string) bool {
	st.Misc.mu.Lock()
	defer st.Misc.mu.Unlock()
	var out []map[string]interface{}
	for _, a := range accounts {
		if a == "" {
			continue
		}
		out = append(out, map[string]interface{}{
			"provider": "generic",
			"url":      a,
		})
	}
	st.Misc.socialAccounts[userID] = out
	if st.Misc.persist != nil {
		st.Misc.persist.MustPut("misc", "social_accounts", st.Misc.socialAccounts)
	}
	return true
}

// ListUserSSHSigningKeys returns SSH signing keys for a user.
func (st *Store) ListUserSSHSigningKeys(userID int) []map[string]interface{} {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	out := make([]map[string]interface{}, len(st.Misc.sshSigningKeys[userID]))
	copy(out, st.Misc.sshSigningKeys[userID])
	return out
}

// AddUserSSHSigningKey adds an SSH signing key for a user.
func (st *Store) AddUserSSHSigningKey(userID int, key string) map[string]interface{} {
	st.Misc.mu.Lock()
	defer st.Misc.mu.Unlock()
	id := st.Misc.nextSSHSigningKeyID
	st.Misc.nextSSHSigningKeyID++
	entry := map[string]interface{}{
		"id":         id,
		"key":        key,
		"title":      "",
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}
	st.Misc.sshSigningKeys[userID] = append(st.Misc.sshSigningKeys[userID], entry)
	if st.Misc.persist != nil {
		st.Misc.persist.MustPut("misc", "ssh_signing_keys", st.Misc.sshSigningKeys)
	}
	return entry
}

// sshSigningKeyEntryID extracts the numeric key ID from an SSH signing key
// entry. Freshly created entries store an int; entries reloaded from
// persistence decode JSON numbers as float64, so both shapes must resolve.
func sshSigningKeyEntryID(entry map[string]interface{}) int {
	switch v := entry["id"].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

// DeleteUserSSHSigningKey deletes an SSH signing key for a user.
func (st *Store) DeleteUserSSHSigningKey(userID, keyID int) bool {
	st.Misc.mu.Lock()
	defer st.Misc.mu.Unlock()
	keys := st.Misc.sshSigningKeys[userID]
	for i, k := range keys {
		if sshSigningKeyEntryID(k) == keyID {
			st.Misc.sshSigningKeys[userID] = append(keys[:i], keys[i+1:]...)
			if st.Misc.persist != nil {
				st.Misc.persist.MustPut("misc", "ssh_signing_keys", st.Misc.sshSigningKeys)
			}
			return true
		}
	}
	return false
}
