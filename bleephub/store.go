package bleephub

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/storage/memory"
)

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
	Agents       map[int]*Agent
	Sessions     map[string]*Session
	Jobs         map[string]*Job
	Users        map[int]*User
	UsersByLogin map[string]*User
	Tokens       map[string]*Token
	DeviceCodes  map[string]*DeviceCode
	Repos        map[int]*Repo
	ReposByName  map[string]*Repo            // "owner/name" → repo
	GitStorages  map[string]*memory.Storage   // "owner/name" → go-git memory storage
	Orgs         map[int]*Org                // id → org
	OrgsByLogin  map[string]*Org             // login → org
	Teams        map[int]*Team               // id → team
	TeamsBySlug  map[string]*Team            // "org/slug" → team
	Memberships  map[string]*Membership      // "org/user" → membership
	Issues       map[int]*Issue              // id → issue
	Labels       map[int]*IssueLabel         // id → label
	Milestones   map[int]*Milestone          // id → milestone
	Comments     map[int]*Comment            // id → comment
	PullRequests map[int]*PullRequest       // id → PR
	PRReviews    map[int]*PullRequestReview // id → review
	Workflows    map[string]*Workflow       // id → workflow
	NextAgent    int
	NextMsg      int64
	NextLog      int
	NextReqID    int64
	NextUser     int
	NextRepo     int
	NextOrg      int
	NextTeam     int
	NextIssue    int
	NextLabel    int
	NextMilestone int
	NextComment  int
	NextPR       int
	NextPRReview int
	NextRunID    int
	mu           sync.RWMutex
}

// Agent represents a registered runner agent.
type Agent struct {
	ID              int                    `json:"id"`
	Name            string                 `json:"name"`
	Version         string                 `json:"version"`
	Enabled         bool                   `json:"enabled"`
	Status          string                 `json:"status"`
	OSDescription   string                 `json:"osDescription"`
	Labels          []Label                `json:"labels"`
	Authorization   *AgentAuthorization    `json:"authorization,omitempty"`
	Ephemeral       bool                   `json:"ephemeral,omitempty"`
	MaxParallelism  int                    `json:"maxParallelism,omitempty"`
	ProvisionState  string                 `json:"provisioningState,omitempty"`
	CreatedOn       time.Time              `json:"createdOn"`
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
	SessionID string `json:"sessionId"`
	OwnerName string `json:"ownerName"`
	Agent     *Agent `json:"agent"`
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
		Agents:       make(map[int]*Agent),
		Sessions:     make(map[string]*Session),
		Jobs:         make(map[string]*Job),
		Users:        make(map[int]*User),
		UsersByLogin: make(map[string]*User),
		Tokens:       make(map[string]*Token),
		DeviceCodes:  make(map[string]*DeviceCode),
		Repos:        make(map[int]*Repo),
		ReposByName:  make(map[string]*Repo),
		GitStorages:  make(map[string]*memory.Storage),
		Orgs:         make(map[int]*Org),
		OrgsByLogin:  make(map[string]*Org),
		Teams:        make(map[int]*Team),
		TeamsBySlug:  make(map[string]*Team),
		Memberships:  make(map[string]*Membership),
		Issues:       make(map[int]*Issue),
		Labels:       make(map[int]*IssueLabel),
		Milestones:   make(map[int]*Milestone),
		Comments:     make(map[int]*Comment),
		PullRequests: make(map[int]*PullRequest),
		PRReviews:    make(map[int]*PullRequestReview),
		Workflows:    make(map[string]*Workflow),
		NextAgent:    1,
		NextMsg:      1,
		NextLog:      1,
		NextReqID:    1,
		NextUser:     1,
		NextRepo:     1,
		NextOrg:      1,
		NextTeam:     1,
		NextIssue:    1,
		NextLabel:    1,
		NextMilestone: 1,
		NextComment:  1,
		NextPR:       1,
		NextPRReview: 1,
		NextRunID:    1,
	}
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

	t := &Token{
		Value:     "bph_0000000000000000000000000000000000000000",
		UserID:    u.ID,
		Scopes:    "repo, read:org, gist",
		CreatedAt: now,
	}
	st.Tokens[t.Value] = t
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

// generateTokenValue creates a bph_-prefixed random token string.
func generateTokenValue() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return fmt.Sprintf("bph_%s", hex.EncodeToString(b))
}
