// Package bleeplab is a lightweight reimplementation of the slice of the
// GitLab API that a real `gitlab-runner` (docker executor) and a CI
// orchestrator exercise: the runner API (`POST /api/v4/jobs/request`,
// `PATCH /api/v4/jobs/:id/trace`, `PUT /api/v4/jobs/:id`, runner verify)
// and the project/pipeline API (projects, commits, pipeline trigger,
// pipeline/job status, job trace). It is the GitLab analog of the
// `bleephub` GitHub control-plane simulator: a real gitlab-runner registers
// against it and runs jobs through a `--docker-host` pointed at a sockerless
// backend, exactly as it would against gitlab.com.
//
// Fidelity, not fakery: the runner authenticates and polls exactly as it
// does against real GitLab; bleeplab differs only in coordinates (base URL
// + tokens). It implements the real wire shapes the runner consumes — it
// does not special-case sockerless.
package bleeplab

import (
	"net/http"
	"os"
	"sync"
	"time"

	gitStorage "github.com/go-git/go-git/v5/storage"
	"github.com/rs/zerolog"
)

// Server holds the in-memory GitLab control-plane state and the HTTP mux.
type Server struct {
	addr   string
	logger zerolog.Logger
	mux    *http.ServeMux

	// externalURL is the base URL a runner uses to clone project repos
	// (git_info.repo_url). It must be reachable from the job/helper
	// container, which is a different network vantage point than the runner
	// process — hence a distinct coordinate from the control-plane API URL.
	// Set via BLEEPLAB_EXTERNAL_URL; empty falls back to the request Host.
	externalURL string
	// started is the process start time, surfaced as uptime on /internal/status.
	started time.Time

	mu        sync.Mutex
	nextID    int
	runners   map[string]*Runner // by auth token
	projects  map[int]*Project
	pipelines map[int]*Pipeline
	jobs      map[int]*Job
	// gitStorages holds each project's go-git Storer, keyed by path
	// (namespace/project). The repos are served over smart-HTTP (git.go).
	gitStorages map[string]gitStorage.Storer
	// artifactsStore persists CI job artifact archives, object-store-backed
	// like the git storage (built lazily, see getArtifactStore).
	artifactsOnce  sync.Once
	artifactsStore artifactStore
	artifactsErr   error
	// queue holds pending job IDs in FIFO order, ready for a runner to
	// claim via POST /api/v4/jobs/request. A job enters the queue when its
	// pipeline reaches it (stage ordering) and leaves when claimed.
	queue []int
}

// NewServer constructs an empty bleeplab control plane listening on addr.
func NewServer(addr string, logger zerolog.Logger) *Server {
	s := &Server{
		addr:        addr,
		logger:      logger,
		mux:         http.NewServeMux(),
		externalURL: os.Getenv("BLEEPLAB_EXTERNAL_URL"),
		started:     time.Now(),
		nextID:      1,
		runners:     map[string]*Runner{},
		projects:    map[int]*Project{},
		pipelines:   map[int]*Pipeline{},
		jobs:        map[int]*Job{},
		gitStorages: map[string]gitStorage.Storer{},
	}
	s.routes()
	return s
}

// Handler exposes the mux for in-process tests (httptest.NewServer).
func (s *Server) Handler() http.Handler { return s.logMiddleware(s.mux) }

// ListenAndServe blocks serving the control-plane API.
func (s *Server) ListenAndServe() error {
	s.logger.Info().Str("addr", s.addr).Msg("bleeplab listening")
	return http.ListenAndServe(s.addr, s.Handler())
}

func (s *Server) routes() {
	// Runner-facing API (gitlab-runner polls these).
	s.mux.HandleFunc("POST /api/v4/runners/verify", s.handleRunnerVerify)
	s.mux.HandleFunc("POST /api/v4/runners", s.handleRunnerRegister)
	s.mux.HandleFunc("DELETE /api/v4/runners", s.handleRunnerUnregister)
	s.mux.HandleFunc("POST /api/v4/jobs/request", s.handleJobRequest)
	s.mux.HandleFunc("PUT /api/v4/jobs/{id}", s.handleJobUpdate)
	s.mux.HandleFunc("PATCH /api/v4/jobs/{id}/trace", s.handleJobTrace)
	s.mux.HandleFunc("POST /api/v4/jobs/{id}/artifacts", s.handleArtifactUpload)
	s.mux.HandleFunc("GET /api/v4/jobs/{id}/artifacts", s.handleArtifactDownload)

	// Control-plane API (the orchestrator / harness drives these).
	s.mux.HandleFunc("POST /api/v4/user/runners", s.handleUserRunnerCreate)
	s.mux.HandleFunc("POST /api/v4/projects", s.handleProjectCreate)
	s.mux.HandleFunc("POST /api/v4/projects/{id}/repository/commits", s.handleCommitCreate)
	s.mux.HandleFunc("POST /api/v4/projects/{id}/pipeline", s.handlePipelineCreate)
	s.mux.HandleFunc("GET /api/v4/projects/{id}/pipelines", s.handlePipelineList)
	s.mux.HandleFunc("GET /api/v4/projects/{id}/pipelines/{pid}", s.handlePipelineGet)
	s.mux.HandleFunc("GET /api/v4/projects/{id}/pipelines/{pid}/jobs", s.handlePipelineJobs)
	s.mux.HandleFunc("GET /api/v4/projects/{id}/jobs/{jid}/trace", s.handleProjectJobTrace)

	// Health + operator surface.
	s.mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Internal read-only aggregation surface the embedded UI consumes for its
	// dashboard (projections over control-plane state with no clean public-API
	// equivalent). Resource detail still comes from the public /api/v4 surface.
	s.mux.HandleFunc("GET /internal/status", s.handleInternalStatus)
	s.mux.HandleFunc("GET /internal/projects", s.handleInternalProjects)
	s.mux.HandleFunc("GET /internal/pipelines", s.handleInternalPipelines)
	s.mux.HandleFunc("GET /internal/pipelines/{id}", s.handleInternalPipeline)
	s.mux.HandleFunc("GET /internal/jobs/{id}", s.handleInternalJob)
	s.mux.HandleFunc("GET /internal/jobs/{id}/artifact", s.handleInternalArtifactDownload)
	s.mux.HandleFunc("GET /internal/runners", s.handleInternalRunners)
	s.mux.HandleFunc("GET /internal/storage", s.handleInternalStorage)

	// Embedded SPA at /ui/ (no-op when built with -tags noui). Registered
	// before the catch-all so the mux's longest-prefix match routes /ui/...
	// to the SPA while git paths fall through.
	s.registerUI()
	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
	})

	// Catch-all: git smart-HTTP (clone/fetch/push) lives on dynamic
	// /{namespace}/{project}.git/... paths the ServeMux can't pattern-match,
	// so it falls through here.
	s.mux.HandleFunc("/", s.handleCatchAll)
}

// handleCatchAll routes git smart-HTTP requests; anything else is 404.
func (s *Server) handleCatchAll(w http.ResponseWriter, r *http.Request) {
	if s.tryHandleGitRequest(w, r) {
		return
	}
	http.NotFound(w, r)
}

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.logger.Debug().Str("method", r.Method).Str("path", r.URL.Path).Msg("request")
		next.ServeHTTP(w, r)
	})
}
