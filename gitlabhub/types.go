package gitlabhub

// RunnerRegistrationRequest is the body for POST /api/v4/runners.
type RunnerRegistrationRequest struct {
	Token       string   `json:"token"`
	Description string   `json:"description"`
	Info        RunnerInfo `json:"info,omitempty"`
	Active      bool     `json:"active"`
	Locked      bool     `json:"locked"`
	RunUntagged bool     `json:"run_untagged"`
	TagList     []string `json:"tag_list"`
	AccessLevel string   `json:"access_level"`
}

// RunnerInfo describes the runner's system.
type RunnerInfo struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Revision     string   `json:"revision"`
	Platform     string   `json:"platform"`
	Architecture string   `json:"architecture"`
	Executor     string   `json:"executor"`
	Features     Features `json:"features"`
}

// Features describes runner capabilities.
type Features struct {
	Variables       bool `json:"variables"`
	Image           bool `json:"image"`
	Services        bool `json:"services"`
	Artifacts       bool `json:"artifacts"`
	Cache           bool `json:"cache"`
	Shared          bool `json:"shared"`
	UploadMultipleArtifacts  bool `json:"upload_multiple_artifacts"`
	UploadRawArtifacts       bool `json:"upload_raw_artifacts"`
	Session                  bool `json:"session"`
	Terminal                 bool `json:"terminal"`
	Refspecs                 bool `json:"refspecs"`
	Masking                  bool `json:"masking"`
	Proxy                    bool `json:"proxy"`
	RawVariables             bool `json:"raw_variables"`
	ArtifactsExclude         bool `json:"artifacts_exclude"`
	MultiBuildSteps          bool `json:"multi_build_steps"`
	TraceChecksum            bool `json:"trace_checksum"`
	TraceSize                bool `json:"trace_size"`
	TraceReset               bool `json:"trace_reset"`
	CISecretsFile            bool `json:"ci_secrets_file"`
	ReturnExitCode           bool `json:"return_exit_code"`
}

// RunnerRegistrationResponse is returned by POST /api/v4/runners.
type RunnerRegistrationResponse struct {
	ID    int    `json:"id"`
	Token string `json:"token"`
}

// RunnerVerifyRequest is the body for POST /api/v4/runners/verify.
type RunnerVerifyRequest struct {
	Token string `json:"token"`
}

// JobRequestBody is the body sent by the runner for POST /api/v4/jobs/request.
type JobRequestBody struct {
	Token    string     `json:"token"`
	Info     RunnerInfo `json:"info"`
	LastUpdate string   `json:"last_update"`
}

// JobResponse is the full job payload returned to the runner.
type JobResponse struct {
	ID            int           `json:"id"`
	Token         string        `json:"token"`
	AllowGitFetch bool          `json:"allow_git_fetch"`
	JobInfo       JobInfo       `json:"job_info"`
	GitInfo       GitInfo       `json:"git_info"`
	RunnerInfo    RunnerInfoRef `json:"runner_info"`
	Variables     []VariableDef `json:"variables"`
	Steps         []StepDef     `json:"steps"`
	Image         ImageDef      `json:"image"`
	Services      []ServiceDef  `json:"services"`
	Artifacts     []ArtifactDef `json:"artifacts"`
	Cache         []CacheDef    `json:"cache"`
	Dependencies  []Dependency  `json:"dependencies"`
	Features      FeaturesDef   `json:"features"`
}

// JobInfo describes the job metadata.
type JobInfo struct {
	Name        string `json:"name"`
	Stage       string `json:"stage"`
	ProjectID   int    `json:"project_id"`
	ProjectName string `json:"project_name"`
}

// GitInfo describes the repo to clone.
type GitInfo struct {
	RepoURL   string   `json:"repo_url"`
	Ref       string   `json:"ref"`
	Sha       string   `json:"sha"`
	BeforeSha string   `json:"before_sha"`
	RefType   string   `json:"ref_type"`
	Refspecs  []string `json:"refspecs"`
	Depth     int      `json:"depth"`
}

// RunnerInfoRef is a reference to the runner.
type RunnerInfoRef struct {
	Timeout int `json:"timeout"`
}

// VariableDef is a CI variable in the job payload.
type VariableDef struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Public bool   `json:"public"`
	Masked bool   `json:"masked"`
	File   bool   `json:"file"`
}

// StepDef is a build step in the job payload.
type StepDef struct {
	Name         string   `json:"name"`
	Script       []string `json:"script"`
	Timeout      int      `json:"timeout"`
	When         string   `json:"when"`
	AllowFailure bool    `json:"allow_failure"`
}

// ImageDef describes a container image.
type ImageDef struct {
	Name       string   `json:"name"`
	Alias      string   `json:"alias,omitempty"`
	Command    []string `json:"command,omitempty"`
	Entrypoint []string `json:"entrypoint,omitempty"`
}

// ServiceDef describes a service container.
type ServiceDef struct {
	Name       string            `json:"name"`
	Alias      string            `json:"alias,omitempty"`
	Command    []string          `json:"command,omitempty"`
	Entrypoint []string          `json:"entrypoint,omitempty"`
	Variables  map[string]string `json:"variables,omitempty"`
}

// ArtifactDef describes an artifact declaration.
type ArtifactDef struct {
	Name      string   `json:"name"`
	Untracked bool     `json:"untracked"`
	Paths     []string `json:"paths"`
	When      string   `json:"when"`
	ExpireIn  string   `json:"expire_in"`
	Type      string   `json:"artifact_type"`
	Format    string   `json:"artifact_format"`
}

// CacheDef describes a cache declaration.
type CacheDef struct {
	Key       string   `json:"key"`
	Untracked bool     `json:"untracked"`
	Paths     []string `json:"paths"`
	Policy    string   `json:"policy"`
	When      string   `json:"when"`
}

// Dependency is a reference to a prior job's artifacts.
type Dependency struct {
	ID            int           `json:"id"`
	Name          string        `json:"name"`
	Token         string        `json:"token"`
	ArtifactsFile ArtifactsFile `json:"artifacts_file"`
}

// ArtifactsFile describes an artifact file reference.
type ArtifactsFile struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// FeaturesDef describes features enabled for the job.
type FeaturesDef struct {
	TraceSections    bool `json:"trace_sections"`
	TraceChecksum    bool `json:"trace_checksum"`
	TraceSize        bool `json:"trace_size"`
	TraceReset       bool `json:"trace_reset"`
	FailureReasons   bool `json:"failure_reasons"`
	CancelGracefully bool `json:"cancel_gracefully"`
}

// JobUpdateRequest is the body for PUT /api/v4/jobs/:id.
type JobUpdateRequest struct {
	Token        string `json:"token"`
	State        string `json:"state"`
	FailureReason string `json:"failure_reason,omitempty"`
	ExitCode     int    `json:"exit_code,omitempty"`
}

// PipelineSubmitRequest is the management API request.
type PipelineSubmitRequest struct {
	Pipeline string            `json:"pipeline"`
	Image    string            `json:"image"`
	Files    map[string]string `json:"files,omitempty"` // extra repo files (for include:local:)
}

// PipelineStatusResponse is the management API status response.
type PipelineStatusResponse struct {
	ID        int                          `json:"id"`
	Status    string                       `json:"status"`
	Result    string                       `json:"result"`
	Jobs      map[string]*PipelineJobView  `json:"jobs"`
	CreatedAt string                       `json:"created_at"`
}

// PipelineJobView is a simplified job view for the API.
type PipelineJobView struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Stage  string `json:"stage"`
	Status string `json:"status"`
	Result string `json:"result"`
}
