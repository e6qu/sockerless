package gitlabhub

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/storage/memory"
)

// Runner represents a registered GitLab runner.
type Runner struct {
	ID          int      `json:"id"`
	Token       string   `json:"token"`
	Description string   `json:"description"`
	Active      bool     `json:"active"`
	Tags        []string `json:"tag_list"`
}

// Project represents a GitLab project.
type Project struct {
	ID        int                   `json:"id"`
	Name      string                `json:"name"`
	Variables map[string]*Variable  `json:"variables,omitempty"`
}

// Variable represents a CI/CD variable.
type Variable struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Protected bool   `json:"protected"`
	Masked    bool   `json:"masked"`
}

// Pipeline represents a CI pipeline.
type Pipeline struct {
	ID        int                       `json:"id"`
	ProjectID int                       `json:"project_id"`
	Status    string                    `json:"status"` // created, pending, running, success, failed, canceled
	Result    string                    `json:"result"`
	Ref       string                    `json:"ref"`
	Sha       string                    `json:"sha"`
	Jobs      map[string]*PipelineJob   `json:"jobs"`
	Stages    []string                  `json:"stages"`
	Def       *PipelineDef              `json:"-"`
	CreatedAt time.Time                 `json:"created_at"`
	ServerURL string                    `json:"-"`
	Image     string                    `json:"-"` // default image override
}

// PipelineJob represents a job within a pipeline.
type PipelineJob struct {
	ID           int       `json:"id"`
	PipelineID   int       `json:"pipeline_id"`
	ProjectID    int       `json:"project_id"`
	Name         string    `json:"name"`
	Stage        string    `json:"stage"`
	Status       string    `json:"status"` // created, pending, running, success, failed, canceled, skipped
	Result       string    `json:"result"`
	AllowFailure bool     `json:"allow_failure"`
	When         string    `json:"when"` // on_success, always, never, manual
	Needs        []string  `json:"needs,omitempty"`
	Token        string    `json:"token"`
	TraceData    []byte    `json:"-"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	RetryCount   int       `json:"retry_count,omitempty"`
	MaxRetries   int       `json:"max_retries,omitempty"`
	Timeout       int               `json:"timeout,omitempty"` // seconds
	MatrixGroup   string            `json:"matrix_group,omitempty"`
	ResourceGroup string            `json:"resource_group,omitempty"`
	DotenvVars    map[string]string `json:"dotenv_vars,omitempty"`
}

// Store holds all in-memory state for gitlabhub.
type Store struct {
	Runners      map[int]*Runner
	RunnersByTok map[string]*Runner
	Projects     map[int]*Project
	Pipelines    map[int]*Pipeline
	Jobs         map[int]*PipelineJob
	GitStorages  map[string]*memory.Storage // "project-name" → go-git storage
	Artifacts    map[int][]byte             // job ID → zip data
	Cache        map[string][]byte          // cache key → data

	NextRunner   int
	NextProject  int
	NextPipeline int
	NextJob      int

	// PendingJobs is a queue of job IDs ready for dispatch to runners.
	PendingJobs []int

	mu sync.RWMutex
}

// NewStore creates an initialized store.
func NewStore() *Store {
	return &Store{
		Runners:      make(map[int]*Runner),
		RunnersByTok: make(map[string]*Runner),
		Projects:     make(map[int]*Project),
		Pipelines:    make(map[int]*Pipeline),
		Jobs:         make(map[int]*PipelineJob),
		GitStorages:  make(map[string]*memory.Storage),
		Artifacts:    make(map[int][]byte),
		Cache:        make(map[string][]byte),
		NextRunner:   1,
		NextProject:  1,
		NextPipeline: 1,
		NextJob:      1,
	}
}

// RegisterRunner creates a new runner and returns it.
func (st *Store) RegisterRunner(desc string, tags []string) *Runner {
	st.mu.Lock()
	defer st.mu.Unlock()

	token := generateRunnerToken()
	r := &Runner{
		ID:          st.NextRunner,
		Token:       token,
		Description: desc,
		Active:      true,
		Tags:        tags,
	}
	st.NextRunner++
	st.Runners[r.ID] = r
	st.RunnersByTok[token] = r
	return r
}

// LookupRunnerByToken returns the runner with the given token, or nil.
func (st *Store) LookupRunnerByToken(token string) *Runner {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.RunnersByTok[token]
}

// UnregisterRunner removes a runner by token. Returns true if found.
func (st *Store) UnregisterRunner(token string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	r, ok := st.RunnersByTok[token]
	if !ok {
		return false
	}
	delete(st.Runners, r.ID)
	delete(st.RunnersByTok, token)
	return true
}

// CreateProject creates a new project and returns it.
func (st *Store) CreateProject(name string) *Project {
	st.mu.Lock()
	defer st.mu.Unlock()

	p := &Project{
		ID:        st.NextProject,
		Name:      name,
		Variables: make(map[string]*Variable),
	}
	st.NextProject++
	st.Projects[p.ID] = p
	return p
}

// GetProject returns a project by ID, or nil.
func (st *Store) GetProject(id int) *Project {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Projects[id]
}

// GetGitStorage returns the go-git memory storage for a project.
func (st *Store) GetGitStorage(name string) *memory.Storage {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.GitStorages[name]
}

// SetGitStorage sets the go-git memory storage for a project.
func (st *Store) SetGitStorage(name string, stor *memory.Storage) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.GitStorages[name] = stor
}

// EnqueueJob adds a job ID to the pending queue.
func (st *Store) EnqueueJob(jobID int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.PendingJobs = append(st.PendingJobs, jobID)
}

// DequeueJob removes and returns the next pending job ID, or 0 if empty.
func (st *Store) DequeueJob() int {
	st.mu.Lock()
	defer st.mu.Unlock()

	if len(st.PendingJobs) == 0 {
		return 0
	}
	id := st.PendingJobs[0]
	st.PendingJobs = st.PendingJobs[1:]
	return id
}

// GetJob returns a job by ID, or nil.
func (st *Store) GetJob(id int) *PipelineJob {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Jobs[id]
}

// GetPipeline returns a pipeline by ID, or nil.
func (st *Store) GetPipeline(id int) *Pipeline {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Pipelines[id]
}

// StoreArtifact stores artifact data for a job.
func (st *Store) StoreArtifact(jobID int, data []byte) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.Artifacts[jobID] = data
}

// GetArtifact returns artifact data for a job, or nil.
func (st *Store) GetArtifact(jobID int) []byte {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Artifacts[jobID]
}

// SetCache stores cache data by key.
func (st *Store) SetCache(key string, data []byte) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.Cache[key] = data
}

// GetCache returns cache data by key, or nil.
func (st *Store) GetCache(key string) []byte {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Cache[key]
}

// generateRunnerToken creates a glrt-prefixed random token string.
func generateRunnerToken() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return fmt.Sprintf("glrt-%s", hex.EncodeToString(b))
}
