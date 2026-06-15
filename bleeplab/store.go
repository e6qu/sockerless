package bleeplab

import (
	"strings"
	"sync"
)

// Runner is a registered CI runner, keyed by its authentication token.
type Runner struct {
	ID    int
	Token string
}

// Project is a CI project with a stored .gitlab-ci.yml (set by a commit).
type Project struct {
	ID       int
	Name     string
	Path     string // path_with_namespace (e.g. "root/demo"); the git repo key
	CIConfig string // raw .gitlab-ci.yml content
	Ref      string // default branch
	SHA      string // HEAD commit SHA of the default branch (set by a commit)
}

// Pipeline is one run of a project's CI config; it owns an ordered set of
// jobs grouped by stage.
type Pipeline struct {
	ID        int
	ProjectID int
	Ref       string
	SHA       string
	Status    string // pending, running, success, failed
	JobIDs    []int
	Stages    []string // ordered stage names
	stageIdx  int      // index of the stage currently enqueued/running
}

// Job is one CI job. Trace is the accumulated build log the runner streams
// back via PATCH .../trace; the orchestrator reads it via the project job
// trace endpoint.
type Job struct {
	ID         int
	Token      string
	Name       string
	Stage      string
	Status     string // created, pending, running, success, failed
	Script     []string
	BeforeS    []string
	AfterS     []string
	Image      string
	Services   []JobService
	Variables  []JobVariable
	ProjectID  int
	PipelineID int
	Ref        string
	SHA        string

	// Artifacts is the job's upload spec (what to archive after it runs);
	// Dependencies names the jobs whose artifacts to download before it runs
	// (empty = GitLab's default: every job in earlier stages).
	Artifacts    []jobArtifactSpec
	Dependencies []string
	// ArtifactSize/ArtifactFilename are set when the runner uploads this
	// job's artifact archive; ArtifactSize == 0 means it produced none.
	ArtifactSize     int64
	ArtifactFilename string

	mu    sync.Mutex
	trace strings.Builder
}

// JobService is a `services:` entry (a sidecar container, e.g. postgres).
type JobService struct {
	Name  string
	Alias string
}

// JobVariable is a CI variable injected into the job environment.
type JobVariable struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Public bool   `json:"public"`
	Masked bool   `json:"masked"`
}

func (j *Job) appendTrace(s string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.trace.WriteString(s)
}

func (j *Job) setTrace(s string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.trace.Reset()
	j.trace.WriteString(s)
}

func (j *Job) traceLen() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.trace.Len()
}

func (j *Job) traceString() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.trace.String()
}

// — Runner-facing wire shapes (what gitlab-runner consumes) —

// jobRequest is the body gitlab-runner POSTs to /api/v4/jobs/request.
type jobRequest struct {
	Token string         `json:"token"`
	Info  map[string]any `json:"info"`
}

// jobResponse is the 201 payload returned when a job is available. Field
// names match the GitLab runner API exactly — the runner deserializes this
// into its common.JobResponse.
type jobResponse struct {
	ID            int               `json:"id"`
	Token         string            `json:"token"`
	AllowGitFetch bool              `json:"allow_git_fetch"`
	JobInfo       jobInfo           `json:"job_info"`
	GitInfo       gitInfo           `json:"git_info"`
	RunnerInfo    runnerInfo        `json:"runner_info"`
	Variables     []JobVariable     `json:"variables"`
	Steps         []jobStep         `json:"steps"`
	Image         *jobImage         `json:"image,omitempty"`
	Services      []jobImage        `json:"services"`
	Artifacts     []jobArtifactSpec `json:"artifacts"`
	Cache         []any             `json:"cache"`
	Credentials   []any             `json:"credentials"`
	Dependencies  []jobDependency   `json:"dependencies"`
	Features      map[string]any    `json:"features"`
}

type jobInfo struct {
	Name        string `json:"name"`
	Stage       string `json:"stage"`
	ProjectID   int    `json:"project_id"`
	ProjectName string `json:"project_name"`
}

type gitInfo struct {
	RepoURL   string   `json:"repo_url"`
	Ref       string   `json:"ref"`
	SHA       string   `json:"sha"`
	BeforeSHA string   `json:"before_sha"`
	RefType   string   `json:"ref_type"`
	Refspecs  []string `json:"refspecs"`
	Depth     int      `json:"depth"`
}

type runnerInfo struct {
	Timeout int `json:"timeout"`
}

type jobStep struct {
	Name         string   `json:"name"` // "script" or "after_script"
	Script       []string `json:"script"`
	Timeout      int      `json:"timeout"`
	When         string   `json:"when"`
	AllowFailure bool     `json:"allow_failure"`
}

type jobImage struct {
	Name       string   `json:"name"`
	Alias      string   `json:"alias,omitempty"`
	Entrypoint []string `json:"entrypoint,omitempty"`
	Command    []string `json:"command,omitempty"`
}

type jobArtifactSpec struct {
	Name      string   `json:"name,omitempty"`
	Paths     []string `json:"paths,omitempty"`
	When      string   `json:"when,omitempty"`
	ExpireIn  string   `json:"expire_in,omitempty"`
	Untracked bool     `json:"untracked,omitempty"`
}

// jobDependency is one entry in the runner-API `dependencies` list: a job in an
// earlier stage whose artifacts this job downloads before running. The runner
// fetches the archive from `GET /api/v4/jobs/:id/artifacts` using Token.
type jobDependency struct {
	ID            int           `json:"id"`
	Name          string        `json:"name"`
	Token         string        `json:"token"`
	ArtifactsFile *artifactFile `json:"artifacts_file,omitempty"`
}

// artifactFile is the metadata for a dependency's uploaded artifact archive.
type artifactFile struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}
